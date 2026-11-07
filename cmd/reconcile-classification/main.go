// Reconcile final categories using persisted Source strategies. No classifier
// is created and no historical AI prediction is removed or relabeled.
package main

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	apply := flag.Bool("apply", false, "write final projections; default is dry-run")
	output := flag.String("report", "", "safe JSON output")
	flag.Parse()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := repository.Open(ctx, cfg.URI, cfg.Database)
	if err != nil {
		return err
	}
	defer s.Client.Disconnect(context.Background())
	report := map[string]any{"mode": "dry_run", "generation_requests": 0, "ai_prediction_labels_preserved": true}
	// Page through stable content order; reconciliation preserves crawled_at.
	rows := []map[string]any{}
	for page := 1; ; page++ {
		items, err := s.Contents(ctx, model.Filter{IncludeCompleted: true, Page: page, Limit: 100})
		if err != nil {
			return err
		}
		for _, v := range items {
			source, d, err := s.CurrentDecision(ctx, v)
			if err != nil {
				return err
			}
			rows = append(rows, map[string]any{"content_id": v.ID.Hex(), "original_category": v.OriginalCategory, "before_category": v.Category, "strategy": source.ClassificationMode(), "needs_ai": d.NeedsAI, "default_policy_category": d.Category})
		}
		if len(items) < 100 {
			break
		}
	}
	report["notices"] = rows
	if *apply {
		n, err := s.ReconcileAll(ctx)
		if err != nil {
			return err
		}
		report["mode"] = "applied"
		report["reconciled_notices"] = n
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if *output != "" {
		return os.WriteFile(*output, raw, 0600)
	}
	fmt.Print(string(raw))
	return nil
}
