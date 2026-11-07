package main

import (
	"context"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/imageanalysis"
	"example.com/crawlserver/internal/imageapply"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/mvp"
	"example.com/crawlserver/internal/repository"
	"flag"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

func main() {
	debug.SetMemoryLimit(384 << 20) // soft Go heap target; pixel/byte bounds remain mandatory
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "image analysis stopped:", e)
		os.Exit(1)
	}
}
func run() error {
	out := flag.String("output", "", "private output/checkpoint directory")
	media := flag.String("media-capture", "", "previously saved Lexical detail directory")
	budgetPrior := flag.String("budget-prior", "", "completed additional run; share25 call and US$3 cap")
	prior := flag.String("prior", "", "prior image run; remaining IDs and image cache")
	compact := flag.Bool("compact", false, "schedule-only output for newly authorized calls")
	apply := flag.Bool("apply", false, "explicit manual Atlas write; otherwise preview")
	input := flag.String("input", "", "prepared JSON; default output/prepared.json")
	retry := flag.Bool("retry-failed", false, "one explicit retry for prior failed429/5xx only; no success replay")
	phase := flag.String("phase", "candidates", "candidates/remaining/download/analyze/preview/apply/review-evidence/local-verify")
	flag.Parse()
	if *out == "" {
		return fmt.Errorf("output required")
	}
	if e := os.MkdirAll(*out, 0700); e != nil {
		return e
	}
	cfg, e := config.LoadMongoConnection("", "")
	if e != nil {
		return e
	}
	if cfg.Database != "ssu_crawler" {
		return fmt.Errorf("expected ssu_crawler")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()
	store, e := repository.OpenReadOnly(ctx, cfg.URI, cfg.Database)
	if e != nil {
		return fmt.Errorf("database unavailable")
	}
	defer store.Client.Disconnect(context.Background())
	rows, _, e := mvp.Collection(ctx, store.Client.Database(cfg.Database), "crawled_contents")
	if e != nil {
		return e
	}
	items := []model.Content{}
	for _, r := range rows {
		var v model.Content
		if bson.Unmarshal(r, &v) != nil {
			return fmt.Errorf("invalid content")
		}
		items = append(items, v)
	}
	if *phase == "review-evidence" {
		var reviews map[string]imageanalysis.VisualReview
		var old []imageanalysis.Result
		if *input == "" || mvp.Read(*input, &reviews) != nil || mvp.Read(filepath.Join(*prior, "results.json"), &old) != nil {
			return fmt.Errorf("reviews and prior results required")
		}
		by := map[string]model.Content{}
		for _, v := range items {
			by[v.ID.Hex()] = v
		}
		prepared := []imageanalysis.Prepared{}
		for _, r := range old {
			review, ok := reviews[r.ID]
			if !ok {
				continue
			}
			v, ok := by[r.ID]
			if !ok || v.ContentHash != r.Original.ContentHash {
				return fmt.Errorf("review original changed")
			}
			p, e := imageanalysis.Prepare(imageanalysis.ReviewedResult(r, v, review, time.Now().UTC()))
			if e != nil {
				return e
			}
			prepared = append(prepared, p)
		}
		return mvp.Write(*out, "prepared.json", prepared)
	}
	if *phase == "remaining" {
		var old struct {
			Candidates []imageanalysis.Candidate `json:"candidates"`
		}
		if *prior == "" || mvp.Read(filepath.Join(*prior, "candidates.json"), &old) != nil {
			return fmt.Errorf("prior candidate corpus required")
		}
		by := map[string]model.Content{}
		for _, v := range items {
			by[v.ID.Hex()] = v
		}
		selected := []imageanalysis.Candidate{}
		excluded := []string{}
		for _, c := range old.Candidates {
			v, ok := by[c.Original.ID.Hex()]
			if !ok {
				return fmt.Errorf("prior candidate missing")
			}
			if v.ContentHash != c.Original.ContentHash {
				return fmt.Errorf("candidate content changed")
			}
			if v.ImageAnalysisUsed {
				excluded = append(excluded, v.ID.Hex())
				continue
			}
			c.Original = v
			selected = append(selected, c)
		}
		if len(selected) > 20 {
			return fmt.Errorf("remaining notice cap20")
		}
		if e = mvp.Write(*out, "selected.json", selected); e != nil {
			return e
		}
		return mvp.Write(*out, "remaining.json", map[string]any{"selected": len(selected), "already_applied": excluded, "max_additional_requests": 25, "cost_cap_usd": 3})
	}
	if *phase == "apply" {
		file := *input
		if file == "" {
			file = filepath.Join(*out, "prepared.json")
		}
		var prepared []imageanalysis.Prepared
		if mvp.Read(file, &prepared) != nil {
			return fmt.Errorf("prepared input required")
		}
		return imageapply.Batch(ctx, store, prepared, filepath.Join(*out, "operations", time.Now().UTC().Format("20060102T150405.000000000")), *apply)
	}
	if *phase == "local-verify" {
		var prepared []imageanalysis.Prepared
		if mvp.Read(filepath.Join(*out, "prepared.json"), &prepared) != nil || len(prepared) == 0 {
			return fmt.Errorf("prepared results required")
		}
		return imageapply.VerifyLocal(ctx, store.Client.Database(cfg.Database), prepared, *out)
	}
	if *phase == "preview" {
		var results []imageanalysis.Result
		if mvp.Read(filepath.Join(*out, "results.json"), &results) != nil {
			return fmt.Errorf("saved results required")
		}
		prepared := []imageanalysis.Prepared{}
		plans := []imageapply.Plan{}
		counts := map[string]int{}
		failures := []map[string]string{}
		for _, r := range results {
			p, e := imageanalysis.Prepare(r)
			if e != nil {
				failures = append(failures, map[string]string{"id": r.ID, "reason": e.Error()})
				continue
			}
			plan, e := imageapply.Preview(ctx, store.Client.Database(cfg.Database), p)
			if e != nil {
				return e
			}
			prepared = append(prepared, p)
			plans = append(plans, plan)
			counts[plan.Outcome]++
		}
		if e = mvp.Write(*out, "prepared.json", prepared); e != nil {
			return e
		}
		mvp.Write(*out, "preparation-failures.json", failures)
		return mvp.Write(*out, "atlas-preview.json", map[string]any{"plans": plans, "outcomes": counts, "atlas_writes": 0, "index_changes": 0})
	}
	if *phase == "analyze" {
		var selected []imageanalysis.Candidate
		if mvp.Read(filepath.Join(*out, "downloaded.json"), &selected) != nil {
			return fmt.Errorf("downloaded images required")
		}
		current := map[string]model.Content{}
		for _, v := range items {
			current[v.ID.Hex()] = v
		}
		for _, c := range selected {
			v, ok := current[c.Original.ID.Hex()]
			if !ok || v.ContentHash != c.Original.ContentHash || v.ScheduleAnalysisID != c.Original.ScheduleAnalysisID {
				return fmt.Errorf("notice changed before paid call")
			}
		}
		ai, e := classification.LoadConfig()
		if e != nil {
			return e
		}
		if *retry {
			if e = mvp.Write(*out, "retry-authorization.json", map[string]any{"reason": "user explicitly requested resume/retry", "max_attempts_per_failed_notice": 4, "total_cap": 25, "no_success_replay": true}); e != nil {
				return e
			}
			ai.Timeout = 90 * time.Second
			return imageanalysis.AnalyzeWithRetry(ctx, *out, selected, ai, 4)
		}
		if *compact {
			ai.Timeout = 120 * time.Second
			return imageanalysis.AnalyzeCompactWithPrior(ctx, *out, selected, ai, *budgetPrior)
		}
		return imageanalysis.Analyze(ctx, *out, selected, ai)
	}
	if *phase == "download" {
		var selected []imageanalysis.Candidate
		if mvp.Read(filepath.Join(*out, "selected.json"), &selected) != nil {
			return fmt.Errorf("selected input required")
		}
		sources, e := store.Sources(ctx)
		if e != nil {
			return e
		}
		cache := map[string]imageanalysis.ImageInput{}
		if *prior != "" {
			var old []imageanalysis.Candidate
			if mvp.Read(filepath.Join(*prior, "downloaded.json"), &old) != nil {
				return fmt.Errorf("prior download cache required")
			}
			for _, c := range old {
				for _, im := range c.Images {
					cache[im.URL] = im
				}
			}
		}
		return imageanalysis.DownloadCached(ctx, *out, selected, sources, cache)
	}
	if *phase != "candidates" {
		return fmt.Errorf("unknown phase")
	}
	candidates, stats := imageanalysis.Candidates(items, *media)
	n := len(candidates)
	if n > 20 {
		n = 20
	}
	selected := append([]imageanalysis.Candidate{}, candidates[:n]...)
	hasMedia := false
	for _, c := range selected {
		if c.Original.SourceName == "ssu_global_media" {
			hasMedia = true
		}
	}
	if !hasMedia && len(selected) > 0 {
		for _, c := range candidates {
			if c.Original.SourceName == "ssu_global_media" {
				selected[len(selected)-1] = c
				break
			}
		}
	}
	if e = mvp.Write(*out, "candidates.json", map[string]any{"stats": stats, "candidates": candidates, "max_notices": 20, "max_images": 3, "max_requests": 25, "estimated_typical_cost_usd": float64(n) * .04, "hard_cost_cap_usd": 3, "estimate_not_billing": true}); e != nil {
		return e
	}
	if e = mvp.Write(*out, "selected.json", selected); e != nil {
		return e
	}
	fmt.Println("Candidate statistics", stats, "selected", n, "details", filepath.Join(*out, "candidates.json"))
	return nil
}
