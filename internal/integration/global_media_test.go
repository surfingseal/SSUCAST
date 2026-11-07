package integration_test

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/testutil/mediafixture"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGlobalMediaSyntheticMongoMockAndAPI(t *testing.T) {
	s := strategyStore(t)
	ctx := context.Background()
	source := model.Source{ID: bson.NewObjectID(), Name: "ssu_global_media", DisplayName: "숭실대학교 글로벌미디어학부", CrawlerType: "ssu_global_media", BaseURL: "https://fixture.invalid", ListURL: "/board/notices", ClassificationStrategy: "hybrid"}
	item := mediafixture.Load(t, "../crawler/testdata/global_media")
	item = saveStrategyNotice(t, s, source, item)
	if item.Author != "합성 작성자" || item.OriginalCategory != "" || item.Category != "unknown" || len(item.Attachments) != 1 {
		t.Fatal("fixture fields lost", item)
	}
	originalID := item.ID
	mock := &strategyMock{category: "extracurricular"}
	record, e := s.ClassifyOne(ctx, item, mock, "mock-media-only")
	if e != nil {
		t.Fatal(e)
	}
	if record.PrimaryCategory != "unknown" || record.ClassificationStatus != "needs_review" || record.RawLabels.PrimaryCategory != "extracurricular" || !record.Applied {
		t.Fatal("media review evidence lost", record)
	}
	cached, e := s.ClassifyOne(ctx, item, mock, "mock-media-only")
	if e != nil || !cached.Reused || mock.calls.Load() != 1 {
		t.Fatal("duplicate model invocation", e)
	}
	if e := s.Save(ctx, []model.Content{item}); e != nil {
		t.Fatal(e)
	}
	again, e := s.Content(ctx, item.ID)
	if e != nil || again.ID != originalID || again.ClassificationStatus != "needs_review" {
		t.Fatal("upsert changed identity", e)
	}
	// A second synthetic captured-shape response contains sufficient plain text,
	// no images or files. Its approved result becomes the final query category.
	read := func(n string) []byte {
		b, e := os.ReadFile("../crawler/testdata/global_media/synthetic/" + n)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	var review crawler.GlobalMediaReview
	_ = json.Unmarshal(read("review.json"), &review)
	rows, _, e := crawler.ParseGlobalMediaList(read("list.page1.json"))
	if e != nil {
		t.Fatal(e)
	}
	second, e := crawler.ParseGlobalMediaDetail(read("detail.1007.json"), "https://fixture.invalid/board/notices/1007", rows[1], review)
	if e != nil {
		t.Fatal(e)
	}
	second = saveStrategyNotice(t, s, source, second)
	m2 := &strategyMock{category: "international"}
	r2, e := s.ClassifyOne(ctx, second, m2, "mock-media-only")
	if e != nil || r2.PrimaryCategory != "international" || r2.ClassificationStatus != "classified" {
		t.Fatal("plain notice classification", r2, e)
	}
	// Both source categories remain empty: AI never rewrites school evidence.
	second, e = s.Content(ctx, second.ID)
	if e != nil || second.OriginalCategory != "" || second.ClassificationMethod != "ai" {
		t.Fatal(second, e)
	}
	excluded := second
	excluded.ID = bson.NilObjectID
	excluded.OriginalURL = "https://fixture.invalid/board/notices/9999"
	excluded.Title = "교원 초빙 공고"
	if e := s.Save(ctx, []model.Content{excluded}); e != nil {
		t.Fatal(e)
	}
	if _, _, e := s.CurrentDecision(ctx, excluded); e != model.ErrExcludedCategory {
		t.Fatal("exclusion did not precede AI", e)
	}
	const token = "test-global-media-only"
	handler := (api.API{Store: s, Token: token}).Handler()
	for _, tc := range []struct {
		category string
		count    int
	}{{"international", 1}, {"unknown", 1}, {"academic", 0}} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/contents?source=ssu_global_media&category="+tc.category, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		var got struct {
			Items []model.Content `json:"items"`
		}
		e := json.Unmarshal(w.Body.Bytes(), &got)
		if e != nil || w.Code != 200 || len(got.Items) != tc.count {
			t.Fatal(tc, w.Code, w.Body.String(), e)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/contents/"+originalID.Hex()+"/classifications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "mock-media-only") {
		t.Fatal(w.Code, w.Body.String())
	}
	total, e := s.Contents(ctx, model.Filter{SourceName: source.Name, Page: 1, Limit: 20})
	if e != nil || len(total) != 2 {
		t.Fatal("excluded notice stored", e, len(total))
	}
	summary := map[string]any{"data_kind": "synthetic", "mongo_saved": 2, "duplicate_documents": 0, "mock_model_requests": mock.calls.Load() + m2.calls.Load(), "paid_vertex_requests": 0, "school_http_requests": 0, "api_status": 200, "categories": []string{"international", "unknown"}, "first_review_reasons": record.Quality.ReviewReasons, "model_tokens_are_mock": true}
	t.Log(fmt.Sprint(summary))
	if path := os.Getenv("MEDIA_TEST_REPORT"); path != "" {
		raw, _ := json.MarshalIndent(summary, "", "  ")
		if e := os.WriteFile(path, append(raw, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
