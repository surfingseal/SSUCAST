// Manual experiment only: no writes to MongoDB, school HTTP requests or model fallback.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/lifecycle"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	ex "example.com/crawlserver/internal/scheduleexperiment"
	"flag"
	"fmt"
	"google.golang.org/genai"
	"os"
	"path/filepath"
	"time"
)

type selection struct {
	Model    string `json:"model"`
	Snapshot string `json:"source_snapshot_sha256"`
	Count    int    `json:"candidate_count"`
	Items    []struct {
		model.Content
		Reason string `json:"selection_reason"`
	} `json:"items"`
}
type result struct {
	ID              string                                      `json:"id"`
	Title           string                                      `json:"title"`
	Status          string                                      `json:"status"`
	Output          *ex.Output                                  `json:"output"`
	ValidatedOutput *ex.Output                                  `json:"validated_output,omitempty"`
	Validation      *ex.Validation                              `json:"validation"`
	Error           string                                      `json:"error,omitempty"`
	HTTPStatus      int                                         `json:"http_status,omitempty"`
	Raw             string                                      `json:"raw_response,omitempty"`
	ModelVersion    string                                      `json:"model_version,omitempty"`
	Usage           *genai.GenerateContentResponseUsageMetadata `json:"usage,omitempty"`
}

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	dir := flag.String("dir", "", "existing experiment directory containing selected.json")
	validateOnly := flag.Bool("validate-only", false, "revalidate saved responses locally; no credentials, Mongo or Vertex access")
	execute := flag.Bool("execute", false, "explicit paid experiment, maximum 10 notices / 12 requests")
	flag.Parse()
	if *dir == "" {
		return fmt.Errorf("-dir required")
	}
	*dir = filepath.Clean(*dir)
	raw, e := os.ReadFile(filepath.Join(*dir, "selected.json"))
	if e != nil {
		return fmt.Errorf("selection unavailable")
	}
	var selected selection
	if json.Unmarshal(raw, &selected) != nil || selected.Model != ex.Model || len(selected.Items) != 10 || selected.Count != 86 {
		return fmt.Errorf("frozen selection must contain exactly 10 of 86")
	}
	ids := map[string]bool{}
	for _, v := range selected.Items {
		if ids[v.ID.Hex()] || v.SourceName != "ssu_catch" || (v.Status != "진행" && v.Status != "unknown") {
			return fmt.Errorf("invalid or duplicate selected notice")
		}
		ids[v.ID.Hex()] = true
	}
	if *validateOnly {
		return revalidate(*dir, selected)
	}
	mongoCfg, e := config.LoadMongoConnection("", "")
	if e != nil {
		return fmt.Errorf("Mongo configuration invalid")
	}
	if mongoCfg.Database != "ssu_crawler" {
		return fmt.Errorf("expected ssu_crawler database")
	}
	cfg, e := classification.LoadConfig()
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	store, e := repository.OpenReadOnly(ctx, mongoCfg.URI, mongoCfg.Database)
	if e != nil {
		return fmt.Errorf("Atlas read-only connection failed")
	}
	defer store.Client.Disconnect(context.Background())
	db := store.Client.Database(mongoCfg.Database)
	snap, before, e := lifecycle.Read(ctx, db)
	if e != nil {
		return fmt.Errorf("Atlas read failed")
	}
	if before != selected.Snapshot || len(snap) != 212 {
		return fmt.Errorf("Atlas snapshot changed since selection; re-review required")
	}
	candidates := 0
	matched := 0
	for _, s := range snap {
		v := s.Content
		if v.SourceName == "ssu_catch" && (v.Status == "진행" || v.Status == "unknown") {
			candidates++
		}
		for _, want := range selected.Items {
			if want.ID != v.ID {
				continue
			}
			if want.Title != v.Title || want.Content.Content != v.Content || want.Status != v.Status || want.PublishedAt == nil || v.PublishedAt == nil || !want.PublishedAt.Equal(*v.PublishedAt) {
				return fmt.Errorf("selected input changed")
			}
			matched++
		}
	}
	if candidates != 86 || matched != 10 {
		return fmt.Errorf("unexpected candidate or selection count")
	}
	client, e := ex.NewClient(ctx, cfg, *dir)
	if e != nil {
		return e
	}
	pre := client.Preflight(ctx)
	pre["atlas_total"] = len(snap)
	pre["atlas_candidates"] = candidates
	pre["snapshot_sha256"] = before
	if e = ex.Write(filepath.Join(*dir, "preflight.json"), pre); e != nil {
		return e
	}
	fmt.Printf("Preflight: project=%s location=%s model=%s candidates=%d metadata=%v\n", cfg.Project, cfg.Location, ex.Model, candidates, pre["model_metadata_accessible"])
	if !*execute {
		return nil
	}
	if pre["vertex_api_http_status"] != 403 && pre["vertex_api_state"] != "ENABLED" {
		return fmt.Errorf("Vertex API state unavailable or disabled; generation not attempted")
	}
	if pre["project_identity_http_status"] != 403 && pre["verified_project_id"] != cfg.Project {
		return fmt.Errorf("project identity mismatch or unavailable; generation not attempted")
	}
	// A metadata 403 may reflect separate Model Garden read permissions. The first
	// selected generation is the access test, never an additional paid probe.
	if pre["model_metadata_accessible"] != true {
		return fmt.Errorf("model metadata unavailable; no paid fallback")
	}
	lock, e := os.OpenFile(filepath.Join(*dir, "execution.lock"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return fmt.Errorf("execution already started; automatic replay forbidden")
	}
	fmt.Fprintf(lock, "selected_sha256=%x\nstarted_at=%s\n", sha256.Sum256(raw), time.Now().UTC().Format(time.RFC3339))
	e = lock.Sync()
	lock.Close()
	if e != nil {
		return e
	}
	results := []result{}
	comparisons := []map[string]any{}
	retries := 0
	permanent := false
	for _, v := range selected.Items {
		r := result{ID: v.ID.Hex(), Title: v.Title, Status: v.Status}
		if permanent {
			r.Error = "not_attempted_after_permanent_failure"
			results = append(results, r)
			continue
		}
		input := map[string]any{"title": v.Title, "content": v.Content.Content, "published_at": v.PublishedAt, "original_status": v.Status}
		var resp *genai.GenerateContentResponse
		var err error
		for {
			callCtx, stop := context.WithTimeout(ctx, cfg.Timeout)
			resp, err = client.Generate(callCtx, r.ID, input)
			stop()
			code := ex.Status(err)
			if err == nil || retries >= 2 || (code != 429 && (code < 500 || code > 599)) {
				break
			}
			retries++
			fmt.Printf("explicit retry: id=%s status=%d global_retry=%d/2\n", r.ID, code, retries)
			timer := time.NewTimer(time.Duration(retries*2) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		if err != nil {
			r.Error = classification.ErrorCode(err)
			r.HTTPStatus = ex.Status(err)
			if r.HTTPStatus == 400 || r.HTTPStatus == 401 || r.HTTPStatus == 403 || r.HTTPStatus == 404 || r.HTTPStatus == 0 {
				permanent = true
			}
		} else if resp == nil {
			r.Error = "empty_response"
		} else {
			r.Raw = resp.Text()
			r.ModelVersion = resp.ModelVersion
			r.Usage = resp.UsageMetadata
			if len(resp.Candidates) != 1 || resp.Candidates[0].FinishReason != genai.FinishReasonStop {
				r.Error = "non_stop_or_missing_candidate"
			} else if out, err := ex.Decode(r.Raw); err != nil {
				r.Error = "invalid_json_schema"
			} else {
				r.Output = &out
				validation := ex.Validate(out, v.Title, v.Content.Content)
				r.Validation = &validation
				clean := ex.ConservativeOutput(out, v.Title, v.Content.Content)
				r.ValidatedOutput = &clean
			}
		}
		results = append(results, r)
		comparisons = append(comparisons, map[string]any{"id": r.ID, "title": v.Title, "rules": lifecycle.Extract(v.Content, time.Now()), "gemini": r.Output, "validation": r.Validation, "note": "규칙 추출기는 단일 마감일과 날짜 종료 시각 추정 정책을 사용한다. AI는 복수 일정과 시각 미기재 null을 보존한다. 두 방식의 차이를 곧바로 정확도 차이로 해석하지 않는다."})
		if e = ex.Write(filepath.Join(*dir, "results.json"), results); e != nil {
			return e
		}
		if e = ex.Write(filepath.Join(*dir, "comparison.json"), comparisons); e != nil {
			return e
		}
		if e = ex.Write(filepath.Join(*dir, "usage.json"), map[string]any{"model": ex.Model, "project": cfg.Project, "location": cfg.Location, "attempts": client.Budget.Attempts, "notices_processed": len(results), "retries": retries, "results": resultsUsage(results)}); e != nil {
			return e
		}
		fmt.Printf("notice %d/10 id=%s http_error=%d schema_valid=%v\n", len(results), r.ID, r.HTTPStatus, r.Validation != nil)
	}
	if e = ex.Write(filepath.Join(*dir, "results.json"), results); e != nil {
		return e
	}
	_, after, e := lifecycle.Read(ctx, db)
	if e != nil {
		return fmt.Errorf("post-run Atlas verification failed")
	}
	e = ex.Write(filepath.Join(*dir, "database-verification.json"), map[string]any{"before_sha256": before, "after_sha256": after, "unchanged": before == after, "documents": len(snap), "database_writes": 0, "school_http_requests": 0, "selected_sha256": fmt.Sprintf("%x", sha256.Sum256(raw))})
	if e != nil {
		return e
	}
	if before != after {
		return fmt.Errorf("Atlas snapshot differs after experiment")
	}
	return nil
}
func resultsUsage(rows []result) []map[string]any {
	out := []map[string]any{}
	for _, r := range rows {
		out = append(out, map[string]any{"id": r.ID, "usage": r.Usage, "error": r.Error, "model_version": r.ModelVersion})
	}
	return out
}

// This mode never loads .env or creates any network client.
func revalidate(dir string, selected selection) error {
	raw, e := os.ReadFile(filepath.Join(dir, "results.json"))
	if e != nil {
		return e
	}
	var rows []result
	if e = json.Unmarshal(raw, &rows); e != nil {
		return e
	}
	if len(rows) != 10 {
		return fmt.Errorf("expected saved results for ten notices")
	}
	for i := range rows {
		r := &rows[i]
		v := selected.Items[i]
		if r.ID != v.ID.Hex() {
			return fmt.Errorf("saved result ID mismatch")
		}
		if r.Output == nil {
			continue
		}
		out, e := ex.Decode(r.Raw)
		if e != nil {
			return fmt.Errorf("saved schema invalid")
		}
		validation := ex.Validate(out, v.Title, v.Content.Content)
		r.Validation = &validation
		clean := ex.ConservativeOutput(out, v.Title, v.Content.Content)
		r.ValidatedOutput = &clean
	}
	return ex.Write(filepath.Join(dir, "results.json"), rows)
}
