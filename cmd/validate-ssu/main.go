// validate-ssu is an opt-in live validation. It never runs from tests/server.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"flag"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type artifact struct {
	Stage   string    `json:"stage"`
	File    string    `json:"file"`
	URL     string    `json:"url"`
	SHA256  string    `json:"sha256"`
	SavedAt time.Time `json:"saved_at"`
	Status  int       `json:"status"`
}
type report struct {
	Mode              string                  `json:"mode"`
	UserApproved      bool                    `json:"user_approved"`
	StartedAt         time.Time               `json:"started_at"`
	Error             string                  `json:"error,omitempty"`
	Cause             string                  `json:"cause,omitempty"`
	Requests          []crawler.RequestRecord `json:"requests"`
	Artifacts         []artifact              `json:"artifacts"`
	Items             []model.Content         `json:"items"`
	NextPage          string                  `json:"next_page"`
	Excluded          []map[string]string     `json:"excluded"`
	ListEligible      int                     `json:"list_eligible"`
	ListDuplicateRows int                     `json:"list_duplicate_rows"`
	GeminiRequests    int                     `json:"gemini_requests"`
}

func saveJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() (ret error) {
	storePreview := flag.Bool("store-preview", false, "validate captured preview and store twice in a generated temporary local MongoDB only")
	execute := flag.Bool("execute", false, "user explicitly approves one live manual SSU crawl (<=3 notices, <=5 HTTP attempts)")
	dir := flag.String("output-dir", "", "new evidence directory (must not already exist)")
	sourceFile := flag.String("sources", "configs/sources.ssu-catch.json", "existing source settings")
	flag.Parse()
	if *storePreview {
		if *execute {
			return fmt.Errorf("choose live execution or local preview storage")
		}
		return persistPreview(*dir)
	}
	if !*execute {
		return fmt.Errorf("explicit -execute required; no requests attempted")
	}
	if *dir == "" {
		return fmt.Errorf("new output directory required")
	}
	if e := os.Mkdir(*dir, 0700); e != nil {
		return e
	}
	r := report{Mode: "live_ssu_manual_user_approved", UserApproved: true, StartedAt: time.Now().UTC(), Items: []model.Content{}, Artifacts: []artifact{}, Requests: []crawler.RequestRecord{}, Excluded: []map[string]string{}}
	defer func() {
		if ret != nil {
			r.Error = ret.Error()
			var e *crawler.FetchError
			if errors.As(ret, &e) && e.Err != nil {
				r.Cause = e.Err.Error()
			}
		}
		if e := saveJSON(filepath.Join(*dir, "run.json"), r); e != nil {
			ret = errors.Join(ret, e)
		}
		fmt.Printf("live collected=%d HTTP=%d Gemini=%d error=%q\n", len(r.Items), len(r.Requests), r.GeminiRequests, r.Error)
	}()
	sources, e := config.LoadSources(*sourceFile)
	if e != nil {
		return e
	}
	var source model.Source
	for _, s := range sources {
		if s.CrawlerKey() == "ssu_catch" {
			source = s
			break
		}
	}
	if source.Name == "" || source.BaseURL != crawler.SSUCatchOrigin || source.ClassificationMode() != "source" {
		return fmt.Errorf("expected existing official SSU source strategy")
	}
	// Read the already reviewed, user-provided actual HTML selector; do not infer.
	raw, e := os.ReadFile("internal/crawler/testdata/ssu_catch/detail.actual.expected.json")
	if e != nil {
		return e
	}
	var expected struct {
		Reviewed  bool `json:"selectors_reviewed"`
		Selectors struct {
			Body string `json:"body"`
		} `json:"selectors"`
	}
	if e = json.Unmarshal(raw, &expected); e != nil {
		return e
	}
	if !expected.Reviewed || expected.Selectors.Body == "" {
		return fmt.Errorf("reviewed actual selector required")
	}
	source.DetailBodySelector = expected.Selectors.Body
	// This flag represents this user's explicit single-run authorization, not an
	// assertion of university permission. Persisted source config remains false.
	source.PolicyApproved = true
	source.Enabled = false
	no := false
	source.AutoEnabled = &no
	source.ID = bson.NewObjectID()
	source.MaxPages = 1
	source.MaxContents = min(source.WithDefaults().MaxContents, 3)
	source.MaxRequests = 5
	source.MaxConcurrency = 1
	source.RequestIntervalMS = max(source.RequestIntervalMS, 2000)
	f := crawler.NewFetcher(20 * time.Second)
	c := crawler.SSUCatchCrawler{Fetcher: f, Requests: func(v []crawler.RequestRecord) { r.Requests = v }}
	sequence := 0
	c.Capture = func(stage string, p crawler.Page) error {
		sequence++
		name := fmt.Sprintf("%02d-%s.actual.html", sequence, stage)
		if stage == "robots" {
			name = fmt.Sprintf("%02d-robots.actual.txt", sequence)
		}
		if e := os.WriteFile(filepath.Join(*dir, name), p.Body, 0600); e != nil {
			return e
		}
		r.Artifacts = append(r.Artifacts, artifact{stage, name, p.URL, fmt.Sprintf("%x", sha256.Sum256(p.Body)), time.Now().UTC(), p.Status})
		if stage == "list" {
			rows, next, e := crawler.ParseSSUCatchList(p.Body, p.URL)
			if e != nil {
				return e
			}
			r.ListEligible = len(rows)
			r.NextPage = next
			doc, e := goquery.NewDocumentFromReader(bytes.NewReader(p.Body))
			if e != nil {
				return e
			}
			seen := map[string]bool{}
			doc.Find("ul.notice-lists > li:not(.notice_head)").Each(func(_ int, s *goquery.Selection) {
				a := s.Find(".notice_col3 a[href]")
				u, e := crawler.SSUCatchURL(p.URL, a.AttrOr("href", ""))
				if e != nil {
					return
				}
				if seen[u] {
					r.ListDuplicateRows++
				}
				seen[u] = true
				title := a.Clone()
				title.Find(".label").Remove()
				a.Find(".label").Each(func(_ int, label *goquery.Selection) {
					if model.IsExcludedCategory(label.Text()) {
						r.Excluded = append(r.Excluded, map[string]string{"url": u, "title": model.NormalizeText(title.Text()), "category": model.NormalizeText(label.Text()), "reason": "official_category_excluded"})
					}
				})
			})
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	items, e := c.Crawl(ctx, source)
	if e != nil {
		return e
	}
	if len(items) == 0 {
		return fmt.Errorf("empty live result is not success")
	}
	for i := range items {
		item := &items[i]
		if item.Title == "" || strings.TrimSpace(item.Content) == "" || item.PublishedAt == nil || item.OriginalURL == "" || item.OriginalCategory == "" {
			return fmt.Errorf("live notice required fields missing")
		}
		if e := model.PrepareClassification(source, item); e != nil {
			return e
		}
		item.Metadata["data_origin"] = "live_http"
		item.Metadata["authorization"] = "explicit_user_single_run"
		item.Metadata["html_evidence_directory"] = *dir
	}
	r.Items = items
	if e := saveJSON(filepath.Join(*dir, "preview.json"), items); e != nil {
		return e
	}
	fmt.Printf("preview: %s\n", filepath.Join(*dir, "preview.json"))
	return nil
}
