package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"go.mongodb.org/mongo-driver/v2/bson"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

// verifyCaptured re-parses the actual HTTP bytes before any database connection.
func verifyCaptured(dir string) (report, error) {
	var r report
	raw, e := os.ReadFile(filepath.Join(dir, "run.json"))
	if e != nil {
		return r, e
	}
	if e = json.Unmarshal(raw, &r); e != nil {
		return r, e
	}
	if r.Mode != "live_ssu_manual_user_approved" || !r.UserApproved || r.Error != "" || len(r.Items) == 0 || len(r.Items) > 3 || len(r.Requests) > 5 {
		return r, fmt.Errorf("successful, bounded actual capture required")
	}
	var list []byte
	details := map[string][]byte{}
	for _, a := range r.Artifacts {
		if filepath.Base(a.File) != a.File {
			return r, fmt.Errorf("unsafe capture filename")
		}
		b, e := os.ReadFile(filepath.Join(dir, a.File))
		if e != nil {
			return r, e
		}
		if a.Status != 200 || fmt.Sprintf("%x", sha256.Sum256(b)) != a.SHA256 {
			return r, fmt.Errorf("capture integrity failed")
		}
		switch a.Stage {
		case "list":
			list = b
		case "detail":
			details[a.URL] = b
		}
	}
	rows, _, e := crawler.ParseSSUCatchList(list, crawler.SSUCatchOrigin+"/공지사항/")
	if e != nil {
		return r, e
	}
	seen := map[string]bool{}
	for _, item := range r.Items {
		if item.PublishedAt == nil || item.SourceID.IsZero() || item.SourceID != r.Items[0].SourceID {
			return r, fmt.Errorf("invalid source or date in preview")
		}
		if seen[item.OriginalURL] {
			return r, fmt.Errorf("duplicate preview URL")
		}
		seen[item.OriginalURL] = true
		var row model.Content
		for _, v := range rows {
			if v.OriginalURL == item.OriginalURL {
				row = v
				break
			}
		}
		body := details[item.OriginalURL]
		if row.OriginalURL == "" || len(body) == 0 {
			return r, fmt.Errorf("preview without original HTML evidence")
		}
		parsed, e := crawler.ParseSSUCatchDetail(body, item.OriginalURL, row, "#contents .bg-white > hr + div")
		if e != nil {
			return r, e
		}
		if parsed.Title != item.Title || parsed.Content != item.Content || parsed.Category != item.Category || !reflect.DeepEqual(parsed.Attachments, item.Attachments) || !parsed.PublishedAt.Equal(*item.PublishedAt) {
			return r, fmt.Errorf("preview differs from parsed capture")
		}
		doc, e := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
		if e != nil {
			return r, e
		}
		heading := doc.Find("#contents .bg-white > h1")
		date := doc.Find("#contents .bg-white > h1 + .clearfix > .float-left.mr-4")
		if heading.Length() != 1 || model.NormalizeText(heading.Text()) != item.Title || date.Length() != 1 {
			return r, fmt.Errorf("actual detail header mismatch")
		}
		dateValue, e := time.ParseInLocation("2006년 1월 2일", model.NormalizeText(date.Text()), time.FixedZone("Asia/Seoul", 9*3600))
		if e != nil || !dateValue.Equal(*item.PublishedAt) {
			return r, fmt.Errorf("list/detail date mismatch")
		}
		if strings.TrimSpace(item.Content) == "" || !model.IsOfficialCategory(item.Category) || model.IsExcludedNotice(item) {
			return r, fmt.Errorf("invalid/excluded live preview")
		}
	}
	return r, nil
}
func persistPreview(dir string) error {
	r, e := verifyCaptured(dir)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db := "crawltest_ssulive_" + bson.NewObjectID().Hex()
	// Dedicated loopback server/data directory; never load the production .env.
	store, e := repository.Open(ctx, "mongodb://127.0.0.1:27030", db)
	if e != nil {
		return e
	}
	defer func() { _ = store.Client.Disconnect(context.Background()) }()
	no := false
	source := model.Source{ID: r.Items[0].SourceID, Name: "ssu_catch", CrawlerType: "ssu_catch", BaseURL: crawler.SSUCatchOrigin, ListURL: "/공지사항/", ClassificationStrategy: "source", Enabled: false, AutoEnabled: &no, PolicyApproved: false}
	if e := store.ApplySources(ctx, []model.Source{source}); e != nil {
		return e
	}
	if e := store.Save(ctx, r.Items); e != nil {
		return e
	}
	filter := model.Filter{IncludeCompleted: true, SourceName: source.Name, Page: 1, Limit: 20}
	first, e := store.Contents(ctx, filter)
	if e != nil || len(first) != len(r.Items) {
		return fmt.Errorf("initial live save count: %d error=%v", len(first), e)
	}
	if e := store.Save(ctx, r.Items); e != nil {
		return e
	}
	again, e := store.Contents(ctx, filter)
	if e != nil || len(again) != len(first) {
		return fmt.Errorf("duplicate upsert count mismatch")
	}
	for _, a := range first {
		b, e := store.Content(ctx, a.ID)
		if e != nil || b.ID != a.ID || b.ContentHash != a.ContentHash || !b.UpdatedAt.Equal(a.UpdatedAt) {
			return fmt.Errorf("unchanged URL identity/revision changed")
		}
		result, e := store.ClassifyOne(ctx, b, nil, "")
		if e != nil || result.ClassificationMethod != "source" {
			return fmt.Errorf("source normalization failed")
		}
	}
	histories, e := store.Client.Database(db).Collection("content_classifications").CountDocuments(ctx, bson.M{})
	if e != nil || histories != 0 {
		return fmt.Errorf("unexpected AI classification history")
	}
	const token = "local-ssu-live-validation-only"
	handler := (api.API{Store: store, Token: token}).Handler()
	apiCounts := map[string]int{}
	expectedCounts := map[string]int{}
	for _, v := range r.Items {
		expectedCounts[v.Category]++
	}
	for category := range expectedCounts {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/contents?source=ssu_catch&category="+category, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		var got struct {
			Items []model.Content `json:"items"`
		}
		if e = json.Unmarshal(w.Body.Bytes(), &got); e != nil || w.Code != 200 {
			return fmt.Errorf("API category query failed")
		}
		apiCounts[category] = len(got.Items)
	}
	if !reflect.DeepEqual(apiCounts, expectedCounts) {
		return fmt.Errorf("API category result mismatch")
	}
	return saveJSON(filepath.Join(dir, "mongo.json"), map[string]any{"mode": "actual_http_capture_to_isolated_temporary_database", "database": db, "uri": "mongodb://127.0.0.1:27030", "stored": len(first), "after_duplicate_save": len(again), "identity_and_updated_at_preserved": true, "ai_history_count": histories, "classification_method": "source", "school_requests_during_store": 0, "gemini_requests": 0, "api_http_status": 200, "api_category_counts": apiCounts, "database_retained": true, "production_env_loaded": false})
}
