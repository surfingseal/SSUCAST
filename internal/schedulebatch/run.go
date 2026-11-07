package schedulebatch

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/scheduleexperiment"
	"example.com/crawlserver/internal/schedules"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/genai"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type Checkpoint struct {
	ID           string                                      `json:"id"`
	InputHash    string                                      `json:"input_hash"`
	CompletedAt  time.Time                                   `json:"completed_at"`
	Error        string                                      `json:"error,omitempty"`
	HTTPStatus   int                                         `json:"http_status"`
	RawResponse  string                                      `json:"raw_response"`
	ModelVersion string                                      `json:"model_version"`
	Usage        *genai.GenerateContentResponseUsageMetadata `json:"usage,omitempty"`
	FinishReason string                                      `json:"finish_reason,omitempty"`
}

func checkpointPath(dir, id string) string { return filepath.Join(dir, "checkpoints", id+".json") }
func fromResponse(v model.Content, resp *genai.GenerateContentResponse, code int) Checkpoint {
	cp := Checkpoint{ID: v.ID.Hex(), InputHash: scheduleapply.InputHash(v), CompletedAt: time.Now().UTC(), HTTPStatus: code}
	if resp == nil {
		cp.Error = "missing_response"
		return cp
	}
	cp.RawResponse = resp.Text()
	cp.ModelVersion = resp.ModelVersion
	cp.Usage = resp.UsageMetadata
	if len(resp.Candidates) != 1 {
		cp.Error = "missing_or_multiple_candidates"
	} else {
		cp.FinishReason = string(resp.Candidates[0].FinishReason)
		if resp.Candidates[0].FinishReason != genai.FinishReasonStop {
			cp.Error = "non_stop_response"
		}
	}
	return cp
}
func lastAttempt(b *Budget, id string) (Attempt, bool) {
	for i := len(b.Attempts) - 1; i >= 0; i-- {
		if b.Attempts[i].ID == id {
			return b.Attempts[i], true
		}
	}
	return Attempt{}, false
}
func Recover(dir string, b *Budget, v model.Content) (*Checkpoint, error) {
	path := checkpointPath(dir, v.ID.Hex())
	raw, e := os.ReadFile(path)
	if e == nil {
		var cp Checkpoint
		if json.Unmarshal(raw, &cp) != nil || cp.ID != v.ID.Hex() || cp.InputHash != scheduleapply.InputHash(v) {
			return nil, fmt.Errorf("checkpoint input mismatch")
		}
		return &cp, nil
	}
	if !os.IsNotExist(e) {
		return nil, e
	}
	a, ok := lastAttempt(b, v.ID.Hex())
	if !ok {
		return nil, nil
	}
	if !a.Finished {
		return nil, fmt.Errorf("ambiguous request: no duplicate paid call allowed")
	}
	if a.HTTPStatus != 200 {
		return nil, nil
	}
	raw, e = os.ReadFile(b.responsePath(a.Number))
	if e != nil {
		return nil, fmt.Errorf("paid response missing: refusing replay")
	}
	var saved SavedHTTP
	if json.Unmarshal(raw, &saved) != nil {
		return nil, fmt.Errorf("saved response corrupt")
	}
	var resp genai.GenerateContentResponse
	if json.Unmarshal([]byte(saved.Body), &resp) != nil {
		return nil, fmt.Errorf("saved paid response cannot be decoded; no replay")
	}
	cp := fromResponse(v, &resp, 200)
	if e = scheduleexperiment.Write(path, cp); e != nil {
		return nil, e
	}
	return &cp, nil
}
func Execute(ctx context.Context, dir string, selection Selection, cfg classification.Config) error {
	if selection.SelectionHash != SelectionHash(selection) || len(selection.Items) != 76 {
		return fmt.Errorf("frozen scope invalid")
	}
	// OS advisory lock is released after a crash; a persistent file alone
	// must not prevent checkpoint recovery or allow concurrent paid workers.
	lock, e := os.OpenFile(filepath.Join(dir, "running.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return fmt.Errorf("another batch process holds the execution lock")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	lock.Truncate(0)
	fmt.Fprintf(lock, "pid=%d started=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	lock.Sync()
	spec := map[string]any{"model": Model, "instruction_sha256": Hash(Instruction), "schema_sha256": Hash(Schema()), "max_output_tokens": 16384, "sdk": "google.golang.org/genai@v1.77.0", "selection_hash": selection.SelectionHash}
	if raw, e := os.ReadFile(filepath.Join(dir, "generation-config.json")); e == nil {
		var old map[string]any
		if json.Unmarshal(raw, &old) != nil || Hash(old) != Hash(spec) {
			return fmt.Errorf("generation configuration changed; no automatic paid continuation")
		}
	} else if os.IsNotExist(e) {
		if e = scheduleexperiment.Write(filepath.Join(dir, "generation-config.json"), spec); e != nil {
			return e
		}
	} else {
		return e
	}
	if e = os.MkdirAll(filepath.Join(dir, "checkpoints"), 0700); e != nil {
		return e
	}
	budget, e := LoadBudget(dir, selection.IDs)
	if e != nil {
		return e
	}
	client, e := NewClient(ctx, cfg, budget)
	if e != nil {
		return e
	}
	model, e := client.SDK.Models.Get(ctx, Model, nil)
	if e != nil {
		return fmt.Errorf("requested model metadata unavailable; no model fallback")
	}
	if e = scheduleexperiment.Write(filepath.Join(dir, "vertex-preflight.json"), map[string]any{"checked_at": time.Now().UTC(), "project": cfg.Project, "location": cfg.Location, "requested_model": Model, "verified_model": model.Name, "sdk": "google.golang.org/genai@v1.77.0", "max_notices": 76, "max_generation_requests": 90, "model_fallback": false}); e != nil {
		return e
	}
	for index, v := range selection.Items {
		cp, e := Recover(dir, budget, v)
		if e != nil {
			return e
		}
		if cp != nil {
			fmt.Printf("checkpoint %d/76 id=%s reused; no model call\n", index+1, v.ID.Hex())
			continue
		}
		for {
			a, has := lastAttempt(budget, v.ID.Hex())
			if has && (!a.Finished || (a.HTTPStatus != 429 && (a.HTTPStatus < 500 || a.HTTPStatus > 599))) {
				return fmt.Errorf("unretryable/ambiguous request retained; no automatic replay")
			}
			n := 0
			for _, x := range budget.Attempts {
				if x.ID == v.ID.Hex() {
					n++
				}
			}
			if n >= 3 || len(budget.Attempts) >= 90 {
				failed := Checkpoint{ID: v.ID.Hex(), InputHash: scheduleapply.InputHash(v), CompletedAt: time.Now().UTC(), Error: "retry_or_global_budget_exhausted", HTTPStatus: a.HTTPStatus}
				scheduleexperiment.Write(checkpointPath(dir, v.ID.Hex()), failed)
				break
			}
			if n > 0 {
				delay := time.Duration(1<<uint(n)) * time.Second
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
				fmt.Printf("explicit retry id=%s retry=%d delay=%s global_calls=%d/90\n", v.ID.Hex(), n, delay, len(budget.Attempts))
			}
			callCtx, stop := context.WithTimeout(ctx, cfg.Timeout)
			resp, err := client.Generate(callCtx, v.ID.Hex(), map[string]any{"title": v.Title, "content": v.Content, "published_at": v.PublishedAt, "original_status": v.Status})
			stop()
			status := scheduleexperiment.Status(err)
			if err == nil {
				cp := fromResponse(v, resp, 200)
				if e = scheduleexperiment.Write(checkpointPath(dir, v.ID.Hex()), cp); e != nil {
					return e
				}
				break
			}
			if status == 429 || status >= 500 && status <= 599 {
				continue
			}
			failed := Checkpoint{ID: v.ID.Hex(), InputHash: scheduleapply.InputHash(v), CompletedAt: time.Now().UTC(), Error: classification.ErrorCode(err), HTTPStatus: status}
			if e = scheduleexperiment.Write(checkpointPath(dir, v.ID.Hex()), failed); e != nil {
				return e
			}
			return fmt.Errorf("permanent API/transport failure; checkpoint retained, no fallback")
		}
		if e = Aggregate(dir, selection); e != nil {
			return e
		}
		fmt.Printf("notice %d/76 done id=%s total_requests=%d/90\n", index+1, v.ID.Hex(), len(budget.Attempts))
	}
	return Aggregate(dir, selection)
}
func Aggregate(dir string, selection Selection) error {
	rawResults := []Checkpoint{}
	validated := []schedules.NoticeResult{}
	reviews := []map[string]any{}
	items := []scheduleapply.Item{}
	successful := []string{}
	errors := []map[string]string{}
	totals := map[string]int64{"promptTokenCount": 0, "candidatesTokenCount": 0, "thoughtsTokenCount": 0, "cachedContentTokenCount": 0, "totalTokenCount": 0}
	for _, v := range selection.Items {
		b, e := os.ReadFile(checkpointPath(dir, v.ID.Hex()))
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		var cp Checkpoint
		if json.Unmarshal(b, &cp) != nil || cp.InputHash != scheduleapply.InputHash(v) {
			return fmt.Errorf("checkpoint corrupt or changed input")
		}
		rawResults = append(rawResults, cp)
		if cp.Usage != nil {
			totals["promptTokenCount"] += int64(cp.Usage.PromptTokenCount)
			totals["candidatesTokenCount"] += int64(cp.Usage.CandidatesTokenCount)
			totals["thoughtsTokenCount"] += int64(cp.Usage.ThoughtsTokenCount)
			totals["cachedContentTokenCount"] += int64(cp.Usage.CachedContentTokenCount)
			totals["totalTokenCount"] += int64(cp.Usage.TotalTokenCount)
		}
		if cp.Error != "" {
			errors = append(errors, map[string]string{"id": cp.ID, "error": cp.Error})
			continue
		}
		n, e := schedules.RevalidateV2(v.ID.Hex(), cp.RawResponse, schedules.Document{Title: v.Title, Content: v.Content})
		if e != nil {
			errors = append(errors, map[string]string{"id": cp.ID, "error": "json_schema_or_validation_failure"})
			continue
		}
		usage := bson.M{}
		b, _ = json.Marshal(cp.Usage)
		json.Unmarshal(b, &usage)
		item, e := scheduleapply.PrepareV2(v, n, cp.RawResponse, cp.ModelVersion, usage, cp.CompletedAt)
		if e != nil {
			errors = append(errors, map[string]string{"id": cp.ID, "error": "apply_projection_validation_failure"})
			validated = append(validated, n)
			continue
		}
		validated = append(validated, n)
		items = append(items, item)
		successful = append(successful, v.ID.Hex())
		if item.Analysis.ValidatedOutput.ScheduleReviewRequired {
			reviews = append(reviews, map[string]any{"id": v.ID.Hex(), "title": v.Title, "schedule_review_required": true, "validation_issues": item.Analysis.ValidationIssues, "audience": n.Audience})
		}
	}
	var attempts []Attempt
	if b, e := os.ReadFile(filepath.Join(dir, "attempts.json")); e == nil {
		json.Unmarshal(b, &attempts)
	}
	actualSuccess := 0
	for _, a := range attempts {
		if a.HTTPStatus == 200 {
			actualSuccess++
		}
	}
	usage := map[string]any{"model": Model, "notices_selected": 76, "actual_generation_requests": len(attempts), "http_success_requests": actualSuccess, "retry_requests": max(0, len(attempts)-distinctRequests(attempts)), "completed_checkpoints": len(rawResults), "validated_apply_items": len(items), "errors": errors, "tokens": totals, "attempts": attempts, "cost_estimate_usd": (float64(totals["promptTokenCount"]-totals["cachedContentTokenCount"])*.30 + float64(totals["cachedContentTokenCount"])*.03 + float64(totals["candidatesTokenCount"]+totals["thoughtsTokenCount"])*2.50) / 1e6, "pricing_source": "https://cloud.google.com/model-garden/pricing", "pricing_mode": "Standard Global", "usage_caveat": "failed requests without usage metadata are not assigned invented tokens"}
	for name, value := range map[string]any{"results.json": rawResults, "validated-results.json": map[string]any{"version": schedules.Version, "notices": validated, "errors": errors}, "review-report.json": reviews, "usage.json": usage} {
		if e := scheduleexperiment.Write(filepath.Join(dir, name), value); e != nil {
			return e
		}
	}
	if len(items) > 0 {
		bundle := scheduleapply.Bundle{Version: scheduleapply.BatchBundleVersion, PreparedAt: selection.SelectedAt, Items: items, Batch: &scheduleapply.BatchScope{Database: "ssu_crawler", SourceName: "ssu_catch", SnapshotHash: selection.SnapshotHash, SelectionHash: selection.SelectionHash, SelectedIDs: selection.IDs, ExcludedIDs: selection.Excluded, SuccessfulIDs: successful, ExpectedItems: len(items)}}
		if e := scheduleapply.ValidateBundle(bundle); e != nil {
			return e
		}
		if e := scheduleexperiment.Write(filepath.Join(dir, "apply-input.json"), bundle); e != nil {
			return e
		}
	}
	return nil
}
func distinctRequests(as []Attempt) int {
	seen := map[string]bool{}
	for _, a := range as {
		seen[a.ID] = true
	}
	return len(seen)
}

// ConfirmUnchanged is a read-only guard both after generation and offline resume.
func ConfirmUnchanged(ctx context.Context, store *repository.Store, database string, existing scheduleapply.Bundle, selection Selection) error {
	now, e := Discover(ctx, store, database, existing)
	if e != nil {
		return e
	}
	if now.SelectionHash != selection.SelectionHash || Hash(now.Items) != Hash(selection.Items) {
		return fmt.Errorf("Atlas/source state changed during experiment")
	}
	return nil
}
