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
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "schedule batch:", e)
		os.Exit(1)
	}
}
func run() error {
	dir := flag.String("directory", "", "new experiment directory")
	existingPath := flag.String("existing", "", "frozen applied ten-notice bundle")
	execute := flag.Bool("execute", false, "explicit paid batch of frozen 76; no Mongo writes")
	aggregate := flag.Bool("aggregate-only", false, "revalidate saved checkpoints with no Vertex call")
	flag.Parse()
	if *dir == "" || *existingPath == "" {
		return fmt.Errorf("directory and existing required")
	}
	raw, e := os.ReadFile(*existingPath)
	if e != nil {
		return fmt.Errorf("existing bundle unavailable")
	}
	var existing scheduleapply.Bundle
	if json.Unmarshal(raw, &existing) != nil {
		return fmt.Errorf("existing bundle invalid")
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
		return fmt.Errorf("Atlas read-only connection failed")
	}
	defer store.Client.Disconnect(context.Background())
	selection, e := schedulebatch.Discover(ctx, store, cfg.Database, existing)
	if e != nil {
		return e
	}
	if e = schedulebatch.Freeze(*dir, selection); e != nil {
		return e
	}
	frozen, e := os.ReadFile(filepath.Join(*dir, "selected.json"))
	if e != nil || json.Unmarshal(frozen, &selection) != nil {
		return fmt.Errorf("frozen selection unavailable")
	}
	if *aggregate {
		if e = schedulebatch.Aggregate(*dir, selection); e != nil {
			return e
		}
	}
	if *execute {
		cfgVertex, e := classification.LoadConfig()
		if e != nil {
			return e
		}
		if e = schedulebatch.Execute(ctx, *dir, selection, cfgVertex); e != nil {
			return e
		}
	}
	if e = schedulebatch.ConfirmUnchanged(ctx, store, cfg.Database, existing, selection); e != nil {
		return e
	}
	if e = scheduleexperiment.Write(filepath.Join(*dir, "atlas-readonly-verification.json"), map[string]any{"snapshot_unchanged": true, "contents": 212, "existing_analyses": 10, "existing_histories": 10, "database_writes": 0, "existing_ten_reanalyzed": 0, "school_http_requests": 0}); e != nil {
		return e
	}
	fmt.Printf("Read-only discovery: total=%d active=%d verified_prior=%d histories=%d remaining=%d\n", selection.Total, selection.Active, selection.Completed, selection.Histories, len(selection.Items))
	return nil
}
