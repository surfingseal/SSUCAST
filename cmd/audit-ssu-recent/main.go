// Local evidence/DB audit. It never requests school, images or Vertex URLs.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"flag"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"go.mongodb.org/mongo-driver/v2/bson"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func write(path string, v any) error {
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
func run() error {
	dir := flag.String("dir", "", "completed capture directory")
	uri := flag.String("mongo-uri", "mongodb://127.0.0.1:27030", "dataset MongoDB")
	db := flag.String("database", "ssu_recent_20261106", "dataset DB")
	flag.Parse()
	read := func(name string, v any) error {
		b, e := os.ReadFile(filepath.Join(*dir, name))
		if e != nil {
			return e
		}
		return json.Unmarshal(b, v)
	}
	var report crawler.SSURecentReport
	if e := read("report.json", &report); e != nil {
		return e
	}
	if report.Status != "completed" && report.Status != "completed_with_review" {
		return fmt.Errorf("completed category traversal required")
	}
	var artifacts []struct{ Stage, File, URL, SHA256 string }
	if e := read("artifacts.json", &artifacts); e != nil {
		return e
	}
	rawByURL := map[string][]byte{}
	for _, a := range artifacts {
		if filepath.Base(a.File) != a.File {
			return fmt.Errorf("unsafe evidence filename")
		}
		b, e := os.ReadFile(filepath.Join(*dir, "raw", a.File))
		if e != nil {
			return e
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != a.SHA256 {
			return fmt.Errorf("evidence hash mismatch")
		}
		rawByURL[a.URL] = b
	}
	// Re-evaluate dates locally without rewriting the historical execution log.
	oldDates := map[string]string{}
	for _, a := range artifacts {
		if strings.HasPrefix(a.Stage, "list_") {
			d, e := goquery.NewDocumentFromReader(strings.NewReader(string(rawByURL[a.URL])))
			if e != nil {
				return e
			}
			d.Find("ul.notice-lists > li:not(.notice_head)").Each(func(_ int, row *goquery.Selection) {
				date, e := crawler.SSUDate(row.Find(".notice_col1 .h2").Text())
				if e == nil && date.Before(report.DateFrom) {
					u, e := crawler.SSUCatchURL(a.URL, row.Find(".notice_col3 a[href]").AttrOr("href", ""))
					if e == nil {
						oldDates[u] = date.In(report.DateFrom.Location()).Format("2006-01-02")
					}
				}
			})
		}
	}
	originalReviews := 0
	corrections := []map[string]string{}
	for category, stats := range report.Categories {
		originalReviews += len(stats.Issues)
		remaining := []crawler.SSUReviewIssue{}
		for _, issue := range stats.Issues {
			if date, ok := oldDates[issue.URL]; ok && issue.Code == "row_schema" {
				stats.DateExcluded++
				corrections = append(corrections, map[string]string{"category": category, "url": issue.URL, "published_date": date, "corrected_reason": "before_date_from"})
			} else {
				remaining = append(remaining, issue)
			}
		}
		stats.Issues = remaining
	}
	if e := write(filepath.Join(*dir, "date-corrections.json"), corrections); e != nil {
		return e
	}
	if e := write(filepath.Join(*dir, "report.corrected.json"), report); e != nil {
		return e
	}
	reviews := []map[string]any{}
	for category, stats := range report.Categories {
		for _, issue := range stats.Issues {
			images := []string{}
			files := []model.Attachment{}
			code := issue.Code
			if raw, ok := rawByURL[issue.URL]; ok {
				d, e := goquery.NewDocumentFromReader(strings.NewReader(string(raw)))
				if e != nil {
					return e
				}
				body := d.Find("#contents .bg-white > hr + div")
				body.Find("img[src]").Each(func(_ int, s *goquery.Selection) {
					base, _ := url.Parse(issue.URL)
					u, e := base.Parse(s.AttrOr("src", ""))
					if e == nil {
						if n, e := model.NormalizeContentURL(u.String()); e == nil {
							images = append(images, n)
						}
					}
				})
				body.Find("ul.download-list a[href]").Each(func(_ int, s *goquery.Selection) {
					base, _ := url.Parse(issue.URL)
					u, e := base.Parse(s.AttrOr("href", ""))
					if e == nil {
						files = append(files, model.Attachment{Name: model.NormalizeText(s.Text()), URL: u.String()})
					}
				})
				if code == "detail_parse" && strings.Contains(issue.Reason, "empty notice body") {
					if len(images) > 0 {
						code = "image_only_no_text"
					} else if len(files) > 0 {
						code = "attachments_only_no_text"
					}
				}
			}
			reviews = append(reviews, map[string]any{"category": category, "url": issue.URL, "code": code, "original_code": issue.Code, "reason": issue.Reason, "image_urls": images, "attachments": files, "ocr_executed": false, "download_executed": false})
		}
	}
	if e := write(filepath.Join(*dir, "reviews.json"), reviews); e != nil {
		return e
	}
	var items []model.Content
	if e := read("items.json", &items); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	store, e := repository.Open(ctx, *uri, *db)
	if e != nil {
		return e
	}
	defer func() { _ = store.Client.Disconnect(context.Background()) }()
	saved, e := store.Contents(ctx, model.Filter{IncludeCompleted: true, IncludeExpired: true, SourceName: "ssu_catch", Page: 1, Limit: 10000})
	if e != nil {
		return e
	}
	if len(saved) != len(items) {
		return fmt.Errorf("capture/DB count mismatch")
	}
	identities := map[string]bool{}
	for _, v := range saved {
		if v.Title == "" || strings.TrimSpace(v.Content) == "" || v.PublishedAt == nil || v.PublishedAt.Before(report.DateFrom) || v.PublishedAt.After(report.DateTo) || v.ID.IsZero() || v.ClassificationMethod != "source" || v.ModelVersion != "" || model.IsExcludedNotice(v) {
			return fmt.Errorf("invalid saved notice")
		}
		if identities[v.OriginalURL] {
			return fmt.Errorf("duplicate URL")
		}
		identities[v.OriginalURL] = true
	}
	histories, e := store.Client.Database(*db).Collection("content_classifications").CountDocuments(ctx, bson.M{})
	if e != nil || histories != 0 {
		return fmt.Errorf("unexpected AI classification history")
	}
	handler := (api.API{Store: store, Token: "local-recent-audit-only"}).Handler()
	categoryResults := map[string]any{}
	for _, spec := range crawler.SSUOfficialCategories {
		rows, e := store.Contents(ctx, model.Filter{IncludeCompleted: true, IncludeExpired: true, SourceName: "ssu_catch", Category: spec.Code, Page: 1, Limit: 10000})
		if e != nil {
			return e
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/contents?source=ssu_catch&category="+spec.Code+"&limit=1", nil)
		req.Header.Set("Authorization", "Bearer local-recent-audit-only")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != 200 {
			return fmt.Errorf("category API failed")
		}
		categoryResults[spec.Code] = map[string]any{"stored_memberships": len(rows), "api_http_status": w.Code}
	}
	return write(filepath.Join(*dir, "audit.json"), map[string]any{"database": *db, "stored": len(saved), "unique_urls": len(identities), "ai_history_count": histories, "category_api": categoryResults, "review_notices": len(reviews), "original_execution_review_count": originalReviews, "out_of_range_rows_reclassified": len(corrections), "artifacts_hash_verified": len(artifacts), "school_http_requests": 0, "vertex_requests": 0, "image_downloads": 0, "attachment_downloads": 0})
}
