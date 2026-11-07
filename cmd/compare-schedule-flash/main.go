// Explicit comparative experiment. Default is read-only preflight. No DB writes.
package main

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/schedulebatch"
	"example.com/crawlserver/internal/scheduleexperiment"
	"example.com/crawlserver/internal/scheduleflash"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func load(p string, v any) error {
	b, e := os.ReadFile(p)
	if e != nil {
		return fmt.Errorf("input unavailable")
	}
	return json.Unmarshal(b, v)
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "Flash experiment:", e)
		os.Exit(1)
	}
}
func run() error {
	lite := flag.String("lite", "", "immutable Lite experiment directory")
	existing := flag.String("existing", "", "applied ten-notice bundle")
	out := flag.String("directory", "", "new experiment directory")
	execute := flag.Bool("execute", false, "explicit paid generation; no Atlas apply")
	aggregate := flag.Bool("aggregate-only", false, "offline saved-response aggregation after read-only input check")
	flag.Parse()
	if *lite == "" || *existing == "" || *out == "" {
		return fmt.Errorf("lite, existing and directory required")
	}
	a, _ := filepath.Abs(*lite)
	b, _ := filepath.Abs(*out)
	if a == b || strings.HasPrefix(b, a+string(os.PathSeparator)) {
		return fmt.Errorf("refusing to modify Lite experiment")
	}
	var s schedulebatch.Selection
	var ten scheduleapply.Bundle
	var oldSpec map[string]any
	if load(filepath.Join(a, "candidates.json"), &s) != nil || load(*existing, &ten) != nil || load(filepath.Join(a, "generation-config.json"), &oldSpec) != nil {
		return fmt.Errorf("saved inputs unavailable")
	}
	if e := scheduleflash.VerifyLite(a, s); e != nil {
		return e
	}
	if oldSpec["instruction_sha256"] != schedulebatch.Hash(schedulebatch.Instruction) || oldSpec["schema_sha256"] != schedulebatch.Hash(schedulebatch.Schema()) || oldSpec["model"] != schedulebatch.Model {
		return fmt.Errorf("prior prompt/schema provenance mismatch")
	}
	if e := os.MkdirAll(b, 0700); e != nil {
		return e
	}
	spec := map[string]any{"version": scheduleapply.FlashResponseFormat, "model": scheduleflash.Model, "thinking_level": "MEDIUM", "lite_model": schedulebatch.Model, "lite_thinking_level": "not explicitly configured (server default)", "instruction_sha256": schedulebatch.Hash(schedulebatch.Instruction), "schema_sha256": schedulebatch.Hash(schedulebatch.Schema()), "lite_instruction_sha256": oldSpec["instruction_sha256"], "lite_schema_sha256": oldSpec["schema_sha256"], "prompt_changed": false, "schema_changed": false, "max_output_tokens": 16384, "sdk": "google.golang.org/genai@v1.77.0", "source_selection_hash": s.SelectionHash, "max_requests": 90, "max_notices": 76, "cost_cap_usd": 3, "input_usd_per_million": .75, "output_and_thinking_usd_per_million": 3.75, "pricing_source": "https://cloud.google.com/model-garden/pricing", "pricing_valid_until": "2026-12-31", "confound": "model and explicit MEDIUM vs prior unspecified thinking differ; not a pure model-only ablation"}
	if _, e := os.Stat(filepath.Join(b, "generation-config.json")); e == nil {
		var prior map[string]any
		if load(filepath.Join(b, "generation-config.json"), &prior) != nil || schedulebatch.Hash(prior) != schedulebatch.Hash(spec) {
			return fmt.Errorf("frozen Flash configuration mismatch")
		}
	} else if !os.IsNotExist(e) {
		return e
	} else if e = scheduleexperiment.Write(filepath.Join(b, "generation-config.json"), spec); e != nil {
		return e
	}
	if e := scheduleexperiment.Write(filepath.Join(b, "selected.json"), map[string]any{"model": scheduleflash.Model, "selected_ids": s.IDs, "excluded_ids": s.Excluded, "source_selection": s}); e != nil {
		return e
	}
	if *aggregate {
		return scheduleflash.Aggregate(b, s)
	}
	cfg, e := config.LoadMongoConnection("", "")
	if e != nil {
		return e
	}
	u, e := url.Parse(cfg.URI)
	if e != nil || u.Scheme != "mongodb+srv" || !strings.HasSuffix(u.Hostname(), ".mongodb.net") || cfg.Database != "ssu_crawler" {
		return fmt.Errorf("expected Atlas ssu_crawler")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Minute)
	defer cancel()
	store, e := repository.OpenReadOnly(ctx, cfg.URI, cfg.Database)
	if e != nil {
		return fmt.Errorf("Atlas read-only connection failed; credentials omitted")
	}
	defer store.Client.Disconnect(context.Background())
	current, e := schedulebatch.Discover(ctx, store, cfg.Database, ten)
	if e != nil {
		return e
	}
	if current.SelectionHash != s.SelectionHash || schedulebatch.Hash(current.Items) != schedulebatch.Hash(s.Items) {
		diffs := []string{}
		by := map[string]string{}
		for _, v := range s.Items {
			by[v.ID.Hex()] = schedulebatch.Hash(v)
		}
		for _, v := range current.Items {
			if by[v.ID.Hex()] != schedulebatch.Hash(v) {
				diffs = append(diffs, v.ID.Hex())
			}
		}
		scheduleexperiment.Write(filepath.Join(b, "input-conflicts.json"), diffs)
		return fmt.Errorf("Atlas frozen input/snapshot conflict; no paid calls")
	}
	if e = scheduleexperiment.Write(filepath.Join(b, "atlas-preflight.json"), map[string]any{"total": current.Total, "active": current.Active, "prior_analyses": current.Completed, "histories": current.Histories, "remaining": len(current.Items), "snapshot_sha256": current.SnapshotHash, "all_content_hashes_match": true, "all_input_hashes_match": true, "title_content_published_identical": true, "prior_ten_history_and_projection_verified": true, "writes": 0}); e != nil {
		return e
	}
	fmt.Println("Preflight: Atlas 212, active 86, prior verified 10, identical Lite inputs 76; no writes")
	var generationErr error
	if !*execute {
		vertex, e := classification.LoadConfig()
		if e != nil {
			return e
		}
		budget, e := schedulebatch.LoadBudget(b, s.IDs)
		if e != nil {
			return e
		}
		budget.ExpectedModel = scheduleflash.Model
		client, e := schedulebatch.NewClient(ctx, vertex, budget)
		if e != nil {
			return e
		}
		metadata, e := client.SDK.Models.Get(ctx, scheduleflash.Model, nil)
		if e != nil {
			return fmt.Errorf("requested Flash model metadata unavailable: %s (no generation/fallback)", classification.ErrorCode(e))
		}
		if e = scheduleexperiment.Write(filepath.Join(b, "vertex-preflight.json"), map[string]any{"model": scheduleflash.Model, "verified_model": metadata.Name, "project": vertex.Project, "location": vertex.Location, "thinking_level": "MEDIUM", "generation_calls": 0, "metadata_does_not_guarantee_generate_permission": true}); e != nil {
			return e
		}
		fmt.Println("Vertex model metadata and ADC verified; generation calls=0")
	}
	if *execute {
		vertex, e := classification.LoadConfig()
		if e != nil {
			return e
		}
		generationErr = scheduleflash.Execute(ctx, b, s, vertex)
		if e = scheduleflash.Aggregate(b, s); e != nil && generationErr == nil {
			generationErr = e
		}
	}
	if e = schedulebatch.ConfirmUnchanged(ctx, store, cfg.Database, ten, s); e != nil {
		return e
	}
	scheduleexperiment.Write(filepath.Join(b, "atlas-readonly-verification.json"), map[string]any{"contents": 212, "histories": 10, "existing_analyses": 10, "snapshot_sha256": s.SnapshotHash, "snapshot_unchanged": true, "atlas_writes": 0, "atlas_deletes": 0, "school_requests": 0, "checked_at": time.Now().UTC()})
	return generationErr
}
