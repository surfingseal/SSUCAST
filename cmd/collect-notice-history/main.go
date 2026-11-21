// Manual archive collection. Never changes live notices, sources or schedules.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/mvp"
	"example.com/crawlserver/internal/predictions"
	"example.com/crawlserver/internal/repository"
	"flag"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	from := flag.String("from", "2025-11-01", "inclusive KST publication date")
	to := flag.String("to", "2026-02-28", "inclusive KST publication date")
	out := flag.String("output", "", "new private evidence directory")
	execute := flag.Bool("execute", false, "explicit HTTP collection")
	apply := flag.Bool("apply", false, "store only in isolated historical collections")
	pages := flag.Int("max-pages", 300, "per-category page budget")
	requests := flag.Int("max-requests", 40, "total request budget, including robots/retries")
	contents := flag.Int("max-contents", 10, "detail budget; start small")
	flag.Parse()
	bounds, e := crawler.ParseSSUCollectionRange(*from, *to)
	if e != nil {
		return e
	}
	sources, e := config.LoadSources("configs/sources.ssu-recent.json")
	if e != nil {
		return e
	}
	s := sources[0]
	s.MaxPages = *pages
	s.MaxRequests = *requests
	s.MaxContents = *contents
	if e = crawler.ValidateSource(s.WithDefaults()); e != nil {
		return e
	}
	preview, e := crawler.PreviewSSURange(s, time.Now(), bounds)
	if e != nil {
		return e
	}
	if !*execute {
		if *apply {
			return fmt.Errorf("apply requires execute")
		}
		b, _ := json.MarshalIndent(map[string]any{"report": preview, "limits": s, "http_requests": 0, "database_writes": 0}, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if *out == "" {
		return fmt.Errorf("new output directory required")
	}
	if e = os.Mkdir(*out, 0700); e != nil {
		return e
	}
	if e = os.Mkdir(filepath.Join(*out, "raw"), 0700); e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	ctx, stop := context.WithTimeout(ctx, 100*time.Minute)
	defer stop()
	runID := bson.NewObjectID().Hex()
	report := preview
	c := crawler.SSUCatchCrawler{Fetcher: crawler.NewFetcher(20 * time.Second), CollectionRange: bounds}
	seq := 0
	c.Capture = func(stage string, p crawler.Page) error {
		seq++
		name := fmt.Sprintf("%05d-%s.html", seq, stage)
		if e := os.WriteFile(filepath.Join(*out, "raw", name), p.Body, 0600); e != nil {
			return e
		}
		return mvp.Write(*out, "last-capture.json", map[string]any{"url": p.URL, "stage": stage, "sha256": fmt.Sprintf("%x", sha256.Sum256(p.Body)), "at": time.Now().UTC()})
	}
	c.RecentReport = func(r crawler.SSURecentReport) { report = r; _ = mvp.Write(*out, "report.json", r) }
	checkpoint, e := os.OpenFile(filepath.Join(*out, "checkpoint.ndjson"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer checkpoint.Close()
	c.RecentItem = func(v model.Content) error { return json.NewEncoder(checkpoint).Encode(v) }
	rows, crawlErr := c.Crawl(ctx, s)
	if e = mvp.Write(*out, "items.json", rows); e != nil {
		return e
	}
	if *apply {
		cfg, e := config.LoadMongoConnection("", "")
		if e != nil {
			return fmt.Errorf("Mongo configuration unavailable")
		}
		store, e := repository.OpenReadOnly(ctx, cfg.URI, cfg.Database)
		if e != nil {
			return fmt.Errorf("Mongo connection unavailable")
		}
		defer store.Client.Disconnect(context.Background())
		run := predictions.HistoryRun{ID: runID, From: *from, To: *to, Completed: crawlErr == nil, Collected: len(rows), At: time.Now().UTC(), Report: report}
		if e = repository.SaveNoticeHistory(ctx, store.Client.Database(cfg.Database), run, rows); e != nil {
			return e
		}
		if e = mvp.Write(*out, "saved.json", run); e != nil {
			return e
		}
	}
	fmt.Printf("history run=%s collected=%d requests=%d status=%s\n", runID, len(rows), report.HTTPRequests, report.Status)
	return crawlErr
}
