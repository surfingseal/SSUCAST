// Package scheduleflash is an explicit, bounded comparative experiment. It is
// never registered with Runner. It reuses the frozen Lite scope and wire schema.
package scheduleflash

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/schedulebatch"
	"example.com/crawlserver/internal/scheduleexperiment"
	"example.com/crawlserver/internal/schedules"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/genai"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const Model = "gemini-3.8-flash"
const LimitUSD = 3.0

func Config() *genai.GenerateContentConfig {
	return &genai.GenerateContentConfig{SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: schedulebatch.Instruction}}}, ResponseMIMEType: "application/json", ResponseSchema: schedulebatch.Schema(), MaxOutputTokens: 16384, CandidateCount: 1, AutomaticContinuation: genai.Ptr(false), ThinkingConfig: &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevelMedium}, HTTPOptions: &genai.HTTPOptions{RetryOptions: &genai.HTTPRetryOptions{Attempts: genai.Ptr[int32](1)}}}
}
func Cost(u *genai.GenerateContentResponseUsageMetadata) float64 {
	if u == nil {
		return 0
	}
	return (float64(u.PromptTokenCount-u.CachedContentTokenCount)*.75 + float64(u.CachedContentTokenCount)*.075 + float64(u.CandidatesTokenCount+u.ThoughtsTokenCount)*3.75) / 1e6
}
func read(p string, v any) error {
	b, e := os.ReadFile(p)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}

// Guard is checked before each journaled network transmission. Previous 200s
// without usage conservatively retain a full reservation; ambiguous transport
// is already blocked by the shared durable journal, including after a restart.
func Guard(dir string, b *schedulebatch.Budget, r *http.Request) error {
	return GuardLimit(dir, b, r, LimitUSD)
}
func GuardLimit(dir string, b *schedulebatch.Budget, r *http.Request, limit float64) error {
	body, e := io.ReadAll(r.Body)
	if e != nil {
		return fmt.Errorf("request body unavailable")
	}
	r.Body.Close()
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	if r.GetBody != nil {
		r.Body, e = r.GetBody()
		if e != nil {
			return e
		}
	}
	var req struct {
		GenerationConfig struct {
			MaxOutputTokens int `json:"maxOutputTokens"`
			ThinkingConfig  struct {
				ThinkingLevel string `json:"thinkingLevel"`
			} `json:"thinkingConfig"`
		} `json:"generationConfig"`
	}
	if json.Unmarshal(body, &req) != nil || req.GenerationConfig.MaxOutputTokens != 16384 || req.GenerationConfig.ThinkingConfig.ThinkingLevel != "MEDIUM" {
		return fmt.Errorf("fixed generation configuration rejected")
	}
	// UTF-8 byte length plus generous envelope allowance, and twice the output
	// cap (extra safety for output/thought accounting), rather than an average.
	reserve := (float64(len(body)+32768)*.75 + float64(2*16384)*3.75) / 1e6
	used := 0.0
	for _, a := range b.Attempts {
		if !a.Finished {
			return fmt.Errorf("ambiguous prior request blocks generation")
		}
		if a.HTTPStatus != 200 {
			continue
		}
		var saved schedulebatch.SavedHTTP
		var resp genai.GenerateContentResponse
		if read(filepath.Join(dir, "responses", fmt.Sprintf("%03d.json", a.Number)), &saved) != nil || json.Unmarshal([]byte(saved.Body), &resp) != nil || resp.UsageMetadata == nil {
			return fmt.Errorf("missing paid usage; manual review required")
		}
		used += Cost(resp.UsageMetadata)
	}
	if used+reserve > limit {
		return fmt.Errorf("cost cap: next worst-case reservation exceeds US$3")
	}
	return scheduleexperiment.Write(filepath.Join(dir, "cost-budget.json"), map[string]any{"actual_known_estimate_usd": used, "next_reservation_usd": reserve, "cap_usd": limit, "next_attempt": len(b.Attempts) + 1, "input_price_per_million": .75, "output_including_thought_price_per_million": 3.75, "source": "https://cloud.google.com/model-garden/pricing", "not_final_invoice": true})
}
func VerifyLite(dir string, s schedulebatch.Selection) error {
	if s.Model != schedulebatch.Model || s.SelectionHash != schedulebatch.SelectionHash(s) || len(s.IDs) != 76 || len(s.Items) != 76 {
		return fmt.Errorf("invalid original Lite scope")
	}
	var selected schedulebatch.Selection
	if read(filepath.Join(dir, "selected.json"), &selected) != nil || schedulebatch.Hash(selected.Items) != schedulebatch.Hash(s.Items) || selected.SelectionHash != s.SelectionHash {
		return fmt.Errorf("Lite candidates/selected mismatch")
	}
	var cps []schedulebatch.Checkpoint
	if read(filepath.Join(dir, "results.json"), &cps) != nil || len(cps) != 76 {
		return fmt.Errorf("all 76 Lite responses required")
	}
	by := map[string]schedulebatch.Checkpoint{}
	for _, c := range cps {
		if _, ok := by[c.ID]; ok {
			return fmt.Errorf("duplicate Lite response")
		}
		by[c.ID] = c
	}
	var old scheduleapply.Bundle
	if read(filepath.Join(dir, "apply-input.json"), &old) != nil || scheduleapply.ValidateBundle(old) != nil || len(old.Items) != 76 {
		return fmt.Errorf("Lite validated projections required")
	}
	projections := map[string]scheduleapply.Item{}
	for _, v := range old.Items {
		projections[v.Original.ID.Hex()] = v
	}
	seen, excluded := map[string]bool{}, map[string]bool{}
	for _, id := range s.Excluded {
		excluded[id] = true
	}
	for i, v := range s.Items {
		id := v.ID.Hex()
		c, ok := by[id]
		item, pok := projections[id]
		if s.IDs[i] != id || seen[id] || excluded[id] || !ok || !pok || c.Error != "" || c.HTTPStatus != 200 || c.ModelVersion != schedulebatch.Model || c.RawResponse == "" || c.InputHash != scheduleapply.InputHash(v) || item.Analysis.InputHash != c.InputHash || item.Analysis.ContentHash != v.ContentHash || item.Analysis.RawResponse != c.RawResponse || schedulebatch.Hash(item.Original) != schedulebatch.Hash(v) {
			return fmt.Errorf("Lite input/result conflict: %s", id)
		}
		seen[id] = true
	}
	return nil
}
func Execute(ctx context.Context, dir string, s schedulebatch.Selection, cfg classification.Config) error {
	lock, e := os.OpenFile(filepath.Join(dir, "running.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return fmt.Errorf("experiment already running")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	for _, sub := range []string{"checkpoints", "responses"} {
		if e = os.MkdirAll(filepath.Join(dir, sub), 0700); e != nil {
			return e
		}
	}
	b, e := schedulebatch.LoadBudget(dir, s.IDs)
	if e != nil {
		return e
	}
	b.ExpectedModel = Model
	b.BeforeSend = func(r *http.Request) error { return Guard(dir, b, r) }
	client, e := schedulebatch.NewClient(ctx, cfg, b)
	if e != nil {
		return e
	}
	metadata, e := client.SDK.Models.Get(ctx, Model, nil)
	if e != nil {
		return fmt.Errorf("Flash metadata unavailable; no fallback; error=%s", classification.ErrorCode(e))
	}
	if e = scheduleexperiment.Write(filepath.Join(dir, "vertex-preflight.json"), map[string]any{"requested_model": Model, "verified_model": metadata.Name, "project": cfg.Project, "location": cfg.Location, "thinking_level": "MEDIUM", "model_fallback": false, "checked_at": time.Now().UTC()}); e != nil {
		return e
	}
	for i, v := range s.Items {
		cp, e := schedulebatch.Recover(dir, b, v)
		if e != nil {
			return e
		}
		if cp != nil {
			if cp.ModelVersion != Model || cp.Error != "" {
				return fmt.Errorf("checkpoint failed or wrong model; no replay")
			}
			fmt.Printf("checkpoint %d/76 reused id=%s\n", i+1, v.ID.Hex())
			continue
		}
		for {
			tries := 0
			for _, a := range b.Attempts {
				if a.ID == v.ID.Hex() {
					tries++
				}
			}
			if tries >= 3 || len(b.Attempts) >= 90 {
				return fmt.Errorf("generation/retry cap reached")
			}
			if tries > 0 {
				timer := time.NewTimer(time.Duration(1<<uint(tries)) * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
			b.ID = v.ID.Hex()
			input, _ := json.Marshal(map[string]any{"title": v.Title, "content": v.Content, "published_at": v.PublishedAt, "original_status": v.Status})
			started := time.Now()
			call, stop := context.WithTimeout(ctx, cfg.Timeout)
			resp, err := client.SDK.Models.GenerateContent(call, Model, genai.Text(string(input)), Config())
			stop()
			if err != nil {
				status := scheduleexperiment.Status(err)
				fmt.Printf("request failed id=%s status=%d retry=%d\n", v.ID.Hex(), status, tries)
				if status == 429 || (status >= 500 && status <= 599) {
					continue
				}
				return fmt.Errorf("generation stopped: %s; no fallback or ambiguous replay", classification.ErrorCode(err))
			}
			// Recover reconstructs a durable SDK checkpoint from the saved HTTP body.
			cp, e = schedulebatch.Recover(dir, b, v)
			if e != nil || cp == nil {
				return fmt.Errorf("paid checkpoint recovery failed; no replay")
			}
			if cp.ModelVersion != Model {
				return fmt.Errorf("unexpected returned model version; stop")
			}
			scheduleexperiment.Write(filepath.Join(dir, "checkpoints", v.ID.Hex()+"-latency.json"), map[string]any{"id": v.ID.Hex(), "milliseconds": time.Since(started).Milliseconds()})
			if resp.UsageMetadata == nil {
				return fmt.Errorf("missing usage blocks further generation")
			}
			if e = Aggregate(dir, s); e != nil {
				return e
			}
			fmt.Printf("Flash %d/76 id=%s requests=%d/90 cost_usd=%.6f\n", i+1, v.ID.Hex(), len(b.Attempts), KnownCost(dir, b))
			break
		}
	}
	return Aggregate(dir, s)
}
func KnownCost(dir string, b *schedulebatch.Budget) float64 {
	total := 0.0
	for _, a := range b.Attempts {
		var h schedulebatch.SavedHTTP
		var r genai.GenerateContentResponse
		if a.HTTPStatus == 200 && read(filepath.Join(dir, "responses", fmt.Sprintf("%03d.json", a.Number)), &h) == nil && json.Unmarshal([]byte(h.Body), &r) == nil {
			total += Cost(r.UsageMetadata)
		}
	}
	return total
}
func Aggregate(dir string, s schedulebatch.Selection) error {
	raw, validated, errors := []schedulebatch.Checkpoint{}, []schedules.NoticeResult{}, []map[string]string{}
	items := []scheduleapply.Item{}
	success := []string{}
	tokens := map[string]int64{"input": 0, "output": 0, "thinking": 0, "cached": 0, "total": 0}
	cost := 0.0
	for _, v := range s.Items {
		var cp schedulebatch.Checkpoint
		e := read(filepath.Join(dir, "checkpoints", v.ID.Hex()+".json"), &cp)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil || cp.InputHash != scheduleapply.InputHash(v) || cp.ModelVersion != Model {
			return fmt.Errorf("Flash checkpoint provenance conflict")
		}
		raw = append(raw, cp)
		u := cp.Usage
		if u != nil {
			tokens["input"] += int64(u.PromptTokenCount)
			tokens["output"] += int64(u.CandidatesTokenCount)
			tokens["thinking"] += int64(u.ThoughtsTokenCount)
			tokens["cached"] += int64(u.CachedContentTokenCount)
			tokens["total"] += int64(u.TotalTokenCount)
			cost += Cost(u)
		}
		if cp.Error != "" {
			errors = append(errors, map[string]string{"id": cp.ID, "error": cp.Error})
			continue
		}
		n, e := schedules.RevalidateV2(cp.ID, cp.RawResponse, schedules.Document{Title: v.Title, Content: v.Content})
		if e != nil {
			errors = append(errors, map[string]string{"id": cp.ID, "error": "json_schema"})
			continue
		}
		validated = append(validated, n)
		usage := bson.M{}
		b, _ := json.Marshal(cp.Usage)
		json.Unmarshal(b, &usage)
		item, e := scheduleapply.PrepareFlash(v, n, cp.RawResponse, cp.ModelVersion, usage, cp.CompletedAt)
		if e != nil {
			errors = append(errors, map[string]string{"id": cp.ID, "error": "projection_validation"})
			continue
		}
		items = append(items, item)
		success = append(success, cp.ID)
	}
	var attempts []schedulebatch.Attempt
	read(filepath.Join(dir, "attempts.json"), &attempts)
	for name, value := range map[string]any{"flash-raw-results.json": raw, "flash-validated-results.json": map[string]any{"version": scheduleapply.FlashResponseFormat, "notices": validated, "errors": errors}, "flash-usage.json": map[string]any{"model": Model, "thinking_level": "MEDIUM", "checkpoints": len(raw), "validated_items": len(items), "generation_requests": len(attempts), "tokens": tokens, "cost_estimate_usd": cost, "cap_usd": LimitUSD, "errors": errors, "not_final_invoice": true}} {
		if e := scheduleexperiment.Write(filepath.Join(dir, name), value); e != nil {
			return e
		}
	}
	if len(items) > 0 {
		b := scheduleapply.Bundle{Version: scheduleapply.BatchBundleVersion, PreparedAt: time.Now().UTC(), Batch: &scheduleapply.BatchScope{ModelVersion: Model, Database: s.Database, SourceName: "ssu_catch", SnapshotHash: s.SnapshotHash, SelectionHash: s.SelectionHash, SelectedIDs: s.IDs, ExcludedIDs: s.Excluded, SuccessfulIDs: success, ExpectedItems: len(items)}, Items: items}
		if e := scheduleapply.ValidateBundle(b); e != nil {
			return e
		}
		return scheduleexperiment.Write(filepath.Join(dir, "apply-input-flash.json"), b)
	}
	return nil
}
