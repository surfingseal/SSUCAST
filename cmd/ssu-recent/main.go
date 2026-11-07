// ssu-recent previews without I/O; manual execution uses the existing Runner.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/service"
	"flag"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func jsonFile(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp := path + ".tmp"
	if e = os.WriteFile(tmp, append(b, '\n'), 0600); e != nil {
		return e
	}
	return os.Rename(tmp, path)
}

type observedStore struct {
	*repository.Store
	mu      sync.Mutex
	Report  crawler.SSURecentReport
	Dir, DB string
	Err     error
}

func (s *observedStore) update(r crawler.SSURecentReport) {
	b, e := json.Marshal(r)
	if e != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = json.Unmarshal(b, &s.Report)
	if e = jsonFile(filepath.Join(s.Dir, "report.json"), s.Report); e != nil {
		s.Err = e
	}
}
func (s *observedStore) Save(ctx context.Context, items []model.Content) error {
	news, updates := map[string]int{}, map[string]int{}
	for _, item := range items {
		var old model.Content
		e := s.Client.Database(s.DB).Collection("crawled_contents").FindOne(ctx, bson.M{"original_url": item.OriginalURL}).Decode(&old)
		if e != nil && e != mongo.ErrNoDocuments {
			return e
		}
		category := item.Metadata["discovery_category"]
		if e == mongo.ErrNoDocuments {
			news[category]++
		} else {
			updates[category]++
		}
	}
	if e := s.Store.Save(ctx, items); e != nil {
		return e
	}
	if e := jsonFile(filepath.Join(s.Dir, "items.json"), items); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, n := range news {
		s.Report.NewStored += n
		if v := s.Report.Categories[k]; v != nil {
			v.NewStored += n
		}
	}
	for k, n := range updates {
		s.Report.Updated += n
		if v := s.Report.Categories[k]; v != nil {
			v.Updated += n
		}
	}
	return jsonFile(filepath.Join(s.Dir, "report.json"), s.Report)
}
func (s *observedStore) Finish(ctx context.Context, j model.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	for _, v := range s.Report.Categories {
		j.ErrorCount += len(v.Issues)
	}
	if j.Status == "succeeded" && j.ErrorCount > 0 {
		j.Status = "partial"
		j.ErrorMessage = "some notices require review; see collection report"
	}
	return s.Store.Finish(ctx, j)
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	execute := flag.Bool("execute", false, "explicit manual crawl; requires approved Source; never enables automatic scheduling")
	path := flag.String("sources", "configs/sources.ssu-recent.json", "source settings")
	dir := flag.String("output-dir", "", "new output directory for manual run; optional preview file directory")
	uri := flag.String("mongo-uri", "", "override MONGODB_URI from process/.env")
	db := flag.String("database", "", "override MONGODB_DATABASE from process/.env; existing documents are never deleted")
	flag.Parse()
	sources, e := config.LoadSources(*path)
	if e != nil {
		return e
	}
	var source model.Source
	for _, s := range sources {
		if s.CrawlerKey() == "ssu_catch" && s.CollectionMode == "recent_three_months" {
			source = s
			break
		}
	}
	if source.Name == "" {
		return fmt.Errorf("SSU recent Source required")
	}
	preview, e := crawler.PreviewSSURecent(source, time.Now())
	if e != nil {
		return e
	}
	if !*execute {
		b, e := json.MarshalIndent(map[string]any{"report": preview, "limits": source, "database_changes": 0, "school_http_requests": 0, "vertex_requests": 0}, "", "  ")
		if e != nil {
			return e
		}
		fmt.Println(string(b))
		return nil
	}
	if !source.PolicyApproved {
		return fmt.Errorf("policy_unreviewed: approved Source required; no HTTP or DB changes")
	}
	if *dir == "" {
		return fmt.Errorf("new evidence output directory required")
	}
	if e = os.Mkdir(*dir, 0700); e != nil {
		return e
	}
	if e = os.Mkdir(filepath.Join(*dir, "raw"), 0700); e != nil {
		return e
	}
	connection, e := config.LoadMongoConnection(*uri, *db)
	if e != nil {
		return e
	}
	*uri = connection.URI
	*db = connection.Database
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Minute)
	defer cancel()
	store, e := repository.Open(ctx, *uri, *db)
	if e != nil {
		return config.RedactMongoError(e, *uri)
	}
	defer func() { _ = store.Client.Disconnect(context.Background()) }()
	source.Enabled = true
	no := false
	source.AutoEnabled = &no
	// No content reconciliation/reclassification or deletion of historical data.
	if e = store.SourceRepository.ApplySources(ctx, []model.Source{source}); e != nil {
		return e
	}
	sources, e = store.Sources(ctx)
	if e != nil {
		return e
	}
	for _, s := range sources {
		if s.Name == source.Name {
			source = s
			break
		}
	}
	observed := &observedStore{Store: store, Dir: *dir, DB: *db}
	f := crawler.NewFetcher(20 * time.Second)
	c := &crawler.SSUCatchCrawler{Fetcher: f, RecentReport: observed.update}
	seq := 0
	artifacts := []map[string]any{}
	c.Capture = func(stage string, p crawler.Page) error {
		seq++
		name := fmt.Sprintf("%05d-%s.html", seq, stage)
		if e := os.WriteFile(filepath.Join(*dir, "raw", name), p.Body, 0600); e != nil {
			return e
		}
		artifacts = append(artifacts, map[string]any{"stage": stage, "file": name, "url": p.URL, "status": p.Status, "sha256": fmt.Sprintf("%x", sha256.Sum256(p.Body)), "saved_at": time.Now().UTC()})
		return jsonFile(filepath.Join(*dir, "artifacts.json"), artifacts)
	}
	checkpoint, e := os.OpenFile(filepath.Join(*dir, "checkpoint.ndjson"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer checkpoint.Close()
	c.RecentItem = func(v model.Content) error { return json.NewEncoder(checkpoint).Encode(v) }
	registry := crawler.NewRegistry()
	if e = registry.Register("ssu_catch", func(model.Source) (crawler.Crawler, error) { return c, nil }); e != nil {
		return e
	}
	runner := service.NewWithRegistry(observed, registry, 1, 70*time.Minute)
	defer runner.Stop()
	job, e := runner.Start(ctx, source.ID)
	if e != nil {
		return e
	}
	fmt.Println("job", job.ID.Hex(), "database", *db)
	defer func() {
		source.Enabled = false
		_ = store.SourceRepository.ApplySources(context.Background(), []model.Source{source})
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			current, e := store.Job(ctx, job.ID)
			if e != nil {
				return e
			}
			if current.Status == "running" {
				continue
			}
			if e = jsonFile(filepath.Join(*dir, "job.json"), current); e != nil {
				return e
			}
			fmt.Printf("job status=%s collected=%d review_errors=%d\n", current.Status, current.CollectedCount, current.ErrorCount)
			if current.Status != "succeeded" && current.Status != "partial" {
				return errors.New(current.ErrorMessage)
			}
			return nil
		}
	}
}
