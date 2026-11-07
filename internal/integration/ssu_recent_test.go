package integration_test

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRecentRepositoryPreservesOldAndMergesMembership(t *testing.T) {
	s := strategyStore(t)
	ctx := context.Background()
	source := model.Source{ID: bson.NewObjectID(), Name: "ssu_catch", CrawlerType: "ssu_catch", BaseURL: "https://scatch.ssu.ac.kr", CollectionMode: "recent_three_months", ClassificationStrategy: "source"}
	now := time.Now().UTC()
	old := model.Content{SourceID: source.ID, Title: "과거 학사 공지", Content: "과거 본문", OriginalURL: "https://scatch.ssu.ac.kr/공지사항/?slug=old-preserved", OriginalCategory: "학사", PublishedAt: func() *time.Time { x := now.AddDate(-1, 0, 0); return &x }(), CrawledAt: now.AddDate(-1, 0, 0), UpdatedAt: now.AddDate(-1, 0, 0)}
	if e := s.SourceRepository.ApplySources(ctx, []model.Source{source}); e != nil {
		t.Fatal(e)
	}
	if e := s.Save(ctx, []model.Content{old}); e != nil {
		t.Fatal(e)
	}
	before, e := s.Contents(ctx, model.Filter{Page: 1, Limit: 10})
	if e != nil || len(before) != 1 {
		t.Fatal(before, e)
	}
	old = before[0]
	fresh := model.Content{SourceID: source.ID, Title: "최근 장학생 모집", Content: "장학 안내 본문", OriginalURL: "https://scatch.ssu.ac.kr/공지사항/?slug=fresh", OriginalCategory: "장학", OriginalCategories: []string{"장학"}, CrawledAt: now, LastSeenAt: now, PublishedAt: &now, ImageURLs: []string{"https://scatch.ssu.ac.kr/poster.png"}}
	if e := s.Save(ctx, []model.Content{fresh}); e != nil {
		t.Fatal(e)
	}
	all, e := s.Contents(ctx, model.Filter{Page: 1, Limit: 10})
	if e != nil || len(all) != 2 {
		t.Fatal(all, e)
	}
	var first model.Content
	for _, v := range all {
		if v.OriginalCategory == "장학" {
			first = v
		}
	}
	fresh.OriginalCategory = "학사"
	fresh.OriginalCategories = []string{"학사"}
	fresh.LastSeenAt = now.Add(time.Hour)
	if e := s.Save(ctx, []model.Content{fresh}); e != nil {
		t.Fatal(e)
	}
	after, e := s.Content(ctx, first.ID)
	if e != nil || !after.CrawledAt.Equal(first.CrawledAt) || after.ID != first.ID || len(after.Categories) != 2 || len(after.OriginalCategories) != 2 || !after.LastSeenAt.After(first.LastSeenAt) || len(after.ImageURLs) != 1 {
		t.Fatal(after, e)
	}
	preserved, e := s.Content(ctx, old.ID)
	if e != nil || preserved.ContentHash != old.ContentHash || !preserved.UpdatedAt.Equal(old.UpdatedAt) {
		t.Fatal("old data modified", preserved, e)
	}
	rows, e := s.Classifications(ctx, first.ID)
	if e != nil || len(rows) != 0 {
		t.Fatal("unexpected AI history", rows, e)
	}
	h := (api.API{Store: s, Token: "recent-test-token"}).Handler()
	for _, cat := range []string{"academic", "scholarship"} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/contents?category="+cat, nil)
		r.Header.Set("Authorization", "Bearer recent-test-token")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var out struct {
			Items []model.Content `json:"items"`
		}
		e := json.Unmarshal(w.Body.Bytes(), &out)
		if e != nil || w.Code != 200 || len(out.Items) == 0 {
			t.Fatal(w.Code, w.Body.String(), e)
		}
	}
}
