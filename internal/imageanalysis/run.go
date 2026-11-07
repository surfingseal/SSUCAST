package imageanalysis

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/mvp"
	"example.com/crawlserver/internal/noticepolicy"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/schedulebatch"
	"example.com/crawlserver/internal/scheduleexperiment"
	"example.com/crawlserver/internal/scheduleflash"
	"fmt"
	"google.golang.org/genai"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type Result struct {
	Review    *VisualReview                               `json:"visual_review,omitempty"`
	Format    string                                      `json:"response_format,omitempty"`
	ID        string                                      `json:"id"`
	InputHash string                                      `json:"input_hash"`
	Original  model.Content                               `json:"original"`
	Images    []ImageInput                                `json:"images"`
	Model     string                                      `json:"model_version"`
	Raw       string                                      `json:"raw_response"`
	Output    *Output                                     `json:"validated_model_output,omitempty"`
	Schedules []model.Schedule                            `json:"image_schedules"`
	Issues    []string                                    `json:"validation_issues"`
	Usage     *genai.GenerateContentResponseUsageMetadata `json:"token_usage,omitempty"`
	At        time.Time                                   `json:"analyzed_at"`
	Error     string                                      `json:"error,omitempty"`
}

func InputHash(c Candidate) string {
	return schedulebatch.Hash([]any{Version, Model, scheduleapply.InputHash(c.Original), c.Original.ContentHash, c.Original.ScheduleAnalysisID, noticepolicy.Effective(c.Original), c.Images, Instruction})
}
func InputHashFor(c Candidate, format string) string {
	if format == CompactVersion {
		return schedulebatch.Hash([]any{CompactVersion, InputHash(c), CompactInstruction, schedulebatch.Hash(CompactConfig())})
	}
	return InputHash(c)
}
func configFor(format string) *genai.GenerateContentConfig {
	if format == CompactVersion {
		return CompactConfig()
	}
	return Config()
}
func recoverResult(dir string, b *schedulebatch.Budget, c Candidate, format string) (*Result, error) {
	var prior Result
	if e := mvp.Read(filepath.Join(dir, "checkpoints", c.Original.ID.Hex()+".json"), &prior); e == nil {
		if prior.InputHash != InputHashFor(c, format) {
			return nil, fmt.Errorf("checkpoint image/input mismatch")
		}
		return &prior, nil
	}
	var last *schedulebatch.Attempt
	for i := range b.Attempts {
		if b.Attempts[i].ID == c.Original.ID.Hex() {
			last = &b.Attempts[i]
		}
	}
	if last == nil || last.HTTPStatus != 200 {
		return nil, nil
	}
	if !last.Finished {
		return nil, fmt.Errorf("ambiguous previous request")
	}
	var saved schedulebatch.SavedHTTP
	var resp genai.GenerateContentResponse
	if mvp.Read(filepath.Join(dir, "responses", fmt.Sprintf("%03d.json", last.Number)), &saved) != nil || json.Unmarshal([]byte(saved.Body), &resp) != nil {
		return nil, fmt.Errorf("paid response cannot be recovered; no replay")
	}
	r := Result{Format: format, ID: c.Original.ID.Hex(), InputHash: InputHashFor(c, format), Original: c.Original, Images: c.Images, Model: resp.ModelVersion, Raw: resp.Text(), Usage: resp.UsageMetadata, At: last.Started.UTC(), Schedules: []model.Schedule{}, Issues: []string{}}
	if r.Model != Model {
		r.Error = "model_mismatch"
	} else if len(resp.Candidates) != 1 || resp.Candidates[0].FinishReason != genai.FinishReasonStop {
		r.Error = "incomplete_or_missing_candidate"
	} else {
		out, e := resultOutput(r)
		if e != nil {
			r.Error = "schema_invalid: " + e.Error()
		} else {
			r.Output = &out
			r.Schedules, r.Issues = resultSchedules(r, out)
		}
	}
	if e := mvp.Write(filepath.Join(dir, "checkpoints"), r.ID+".json", r); e != nil {
		return nil, e
	}
	return &r, nil
}
func guard(dir string, b *schedulebatch.Budget, r *http.Request, priorDir string) error {
	// Reservation is deliberately an upper bound: UTF-8 body bytes, image token
	// allowance for three high-resolution posters, and doubled output/thought cap.
	raw, e := io.ReadAll(r.Body)
	if e != nil {
		return e
	}
	r.Body.Close()
	if r.GetBody == nil {
		return fmt.Errorf("replayable SDK body required")
	}
	r.Body, e = r.GetBody()
	if e != nil {
		return e
	}
	used := scheduleflash.KnownCost(dir, b)
	if priorDir != "" {
		var attempts []schedulebatch.Attempt
		if mvp.Read(filepath.Join(priorDir, "attempts.json"), &attempts) != nil {
			return fmt.Errorf("prior additional budget missing")
		}
		for _, a := range attempts {
			if !a.Finished {
				return fmt.Errorf("prior request still active or ambiguous")
			}
		}
		if len(attempts)+len(b.Attempts) >= 25 {
			return fmt.Errorf("aggregate additional request cap25")
		}
		used += scheduleflash.KnownCost(priorDir, &schedulebatch.Budget{Attempts: attempts})
	}
	for _, a := range b.Attempts {
		if a.HTTPStatus == 200 {
			var saved schedulebatch.SavedHTTP
			var v genai.GenerateContentResponse
			if mvp.Read(filepath.Join(dir, "responses", fmt.Sprintf("%03d.json", a.Number)), &saved) != nil || json.Unmarshal([]byte(saved.Body), &v) != nil || v.UsageMetadata == nil {
				return fmt.Errorf("missing paid token accounting")
			}
		}
	}
	var wire map[string]any
	if json.Unmarshal(raw, &wire) != nil {
		return fmt.Errorf("invalid SDK body")
	}
	images := 0
	var strip func(any)
	strip = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if inline, ok := x["inlineData"].(map[string]any); ok {
				images++
				inline["data"] = ""
			}
			for _, y := range x {
				strip(y)
			}
		case []any:
			for _, y := range x {
				strip(y)
			}
		}
	}
	strip(wire)
	if images < 1 || images > 3 {
		return fmt.Errorf("inline image count rejected")
	}
	textBody, _ := json.Marshal(wire)
	// Official high resolution is1120 tokens/image; reserve32768/image plus
	// another32768 envelope allowance, independently of compressed image bytes.
	reserve := (float64(len(textBody)+images*32768+32768)*.75 + float64(2*16384)*3.75) / 1e6
	if used+reserve > 3 {
		return fmt.Errorf("US$3 cap cannot cover next worst-case request")
	}
	return mvp.Write(dir, "budget.json", map[string]any{"actual_estimate_usd": used, "next_reservation_usd": reserve, "cap_usd": 3, "request_cap": 25})
}
func Analyze(ctx context.Context, dir string, candidates []Candidate, cfg classification.Config) error {
	return AnalyzeWithRetry(ctx, dir, candidates, cfg, 3)
}
func AnalyzeWithRetry(ctx context.Context, dir string, candidates []Candidate, cfg classification.Config, perNoticeCap int) error {
	return analyze(ctx, dir, candidates, cfg, perNoticeCap, "", "")
}
func AnalyzeCompact(ctx context.Context, dir string, candidates []Candidate, cfg classification.Config) error {
	return analyze(ctx, dir, candidates, cfg, 3, CompactVersion, "")
}
func AnalyzeCompactWithPrior(ctx context.Context, dir string, candidates []Candidate, cfg classification.Config, prior string) error {
	return analyze(ctx, dir, candidates, cfg, 3, CompactVersion, prior)
}
func analyze(ctx context.Context, dir string, candidates []Candidate, cfg classification.Config, perNoticeCap int, format string, priorDir string) error {
	if perNoticeCap < 3 || perNoticeCap > 4 {
		return fmt.Errorf("explicit retry cap outside3..4")
	}
	if len(candidates) == 0 || len(candidates) > 20 {
		return fmt.Errorf("one to20 notices required")
	}
	lock, e := os.OpenFile(filepath.Join(dir, "running.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return fmt.Errorf("image worker already active")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	frozen := map[string]any{"version": Version, "model": Model, "configuration_hash": schedulebatch.Hash(configFor(format)), "inputs": []string{}, "max_requests": 25, "max_notices": 20, "max_images": 3, "cost_cap_usd": 3}
	hashes := []string{}
	ids := []string{}
	for _, c := range candidates {
		ids = append(ids, c.Original.ID.Hex())
		hashes = append(hashes, InputHashFor(c, format))
		if len(c.Images) < 1 || len(c.Images) > 3 {
			return fmt.Errorf("image count cap")
		}
		for _, im := range c.Images {
			raw, e := os.ReadFile(im.File)
			if e != nil || digest(raw) != im.SHA256 {
				return fmt.Errorf("image bytes changed")
			}
		}
	}
	if priorDir != "" {
		frozen["prior_additional_budget_dir"] = priorDir
	}
	frozen["inputs"] = hashes
	if _, e = os.Stat(filepath.Join(dir, "generation-config.json")); e == nil {
		var old map[string]any
		if mvp.Read(filepath.Join(dir, "generation-config.json"), &old) != nil || schedulebatch.Hash(old) != schedulebatch.Hash(frozen) {
			return fmt.Errorf("frozen multimodal inputs/config changed")
		}
	} else if e = mvp.Write(dir, "generation-config.json", frozen); e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Join(dir, "checkpoints"), 0700); e != nil {
		return e
	}
	b, e := schedulebatch.LoadBoundedBudget(dir, ids, Model, 25)
	if e != nil {
		return e
	}
	b.MaxNoticeAttempts = perNoticeCap
	b.BeforeSend = func(r *http.Request) error { return guard(dir, b, r, priorDir) }
	client, e := schedulebatch.NewClient(ctx, cfg, b)
	if e != nil {
		return e
	}
	meta, e := client.SDK.Models.Get(ctx, Model, nil)
	if e != nil {
		return fmt.Errorf("model preflight failed: %s", classification.ErrorCode(e))
	}
	mvp.Write(dir, "vertex-preflight.json", map[string]any{"model": meta.Name, "region": cfg.Location, "model_generation_requests": 0})
	results := []Result{}
	failures := []map[string]any{}
	for i, c := range candidates {
		for {
			r, e := recoverResult(dir, b, c, format)
			if e != nil {
				return e
			}
			if r != nil {
				results = append(results, *r)
				break
			}
			attempts := 0
			for _, a := range b.Attempts {
				if a.ID == c.Original.ID.Hex() {
					attempts++
				}
			}
			if attempts >= perNoticeCap || len(b.Attempts) >= 25 {
				failures = append(failures, map[string]any{"id": c.Original.ID.Hex(), "reason": "budget_or_retry_cap"})
				break
			}
			if attempts > 0 {
				timer := time.NewTimer(time.Duration(1<<attempts) * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
			input := map[string]any{"title": c.Original.Title, "published_at": c.Original.PublishedAt, "content": c.Original.Content, "existing_text_schedules": noticepolicy.Effective(c.Original), "image_inputs": c.Images}
			// Local filenames are implementation details, never model input.
			ims := []map[string]any{}
			for _, im := range c.Images {
				ims = append(ims, map[string]any{"image_index": im.Index, "location": im.Location, "alt": im.Alt})
			}
			input["image_inputs"] = ims
			raw, _ := json.Marshal(input)
			parts := []*genai.Part{genai.NewPartFromText(string(raw))}
			for _, im := range c.Images {
				body, _ := os.ReadFile(im.File)
				parts = append(parts, genai.NewPartFromText(fmt.Sprintf("image_index=%d", im.Index)), genai.NewPartFromBytes(body, im.MIME))
			}
			b.ID = c.Original.ID.Hex()
			_, err := client.SDK.Models.GenerateContent(ctx, Model, []*genai.Content{{Role: "user", Parts: parts}}, configFor(format))
			if err != nil {
				status := scheduleexperiment.Status(err)
				failures = append(failures, map[string]any{"id": c.Original.ID.Hex(), "status": status, "reason": classification.ErrorCode(err)})
				mvp.Write(dir, "model-failures.json", failures)
				if status == 429 || status >= 500 && status <= 599 {
					continue
				}
				if status == 401 || status == 403 || status == 404 || status == 0 {
					return fmt.Errorf("access/budget/ambiguous model request stopped: %s", classification.ErrorCode(err))
				}
				break
			}
		}
		if e = mvp.Write(dir, "results.json", results); e != nil {
			return e
		}
		tokens := map[string]int64{"input": 0, "output": 0, "thinking": 0, "total": 0}
		for _, r := range results {
			if r.Usage != nil {
				tokens["input"] += int64(r.Usage.PromptTokenCount)
				tokens["output"] += int64(r.Usage.CandidatesTokenCount)
				tokens["thinking"] += int64(r.Usage.ThoughtsTokenCount)
				tokens["total"] += int64(r.Usage.TotalTokenCount)
			}
		}
		mvp.Write(dir, "usage.json", map[string]any{"requests": len(b.Attempts), "checkpoint_notices": len(results), "tokens": tokens, "cost_estimate_usd": scheduleflash.KnownCost(dir, b), "cost_cap_usd": 3, "not_final_invoice": true})
		fmt.Printf("Image model %d/%d checkpoints=%d calls=%d/25 cost=%.6f\n", i+1, len(candidates), len(results), len(b.Attempts), scheduleflash.KnownCost(dir, b))
	}
	return mvp.Write(dir, "model-failures.json", failures)
}
