package integration_test

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/lifecycle"
	"example.com/crawlserver/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCompletedVisibilitySourceAndLifecycleComposition(t *testing.T) {
	s := strategyStore(t)
	ctx := context.Background()
	db := s.Client.Database(sourcesDatabaseName(t, s))
	ssu := model.Source{ID: bson.NewObjectID(), Name: "ssu_catch", CrawlerType: "ssu_catch", ClassificationStrategy: "source", BaseURL: "https://fixture.invalid"}
	department := model.Source{ID: bson.NewObjectID(), Name: "department", CrawlerType: "example", ClassificationStrategy: "source", BaseURL: "https://fixture.invalid"}
	alias := model.Source{ID: bson.NewObjectID(), Name: "ssu_alias", CrawlerType: "ssu_catch", ClassificationStrategy: "source", BaseURL: "https://fixture.invalid"}
	if e := s.SourceRepository.ApplySources(ctx, []model.Source{ssu, department, alias}); e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	pub := now.AddDate(0, 0, -10)
	past := now.In(lifecycle.Seoul).AddDate(0, 0, -1).Format("2006.01.02")
	makeItem := func(source model.Source, path, status, body string) model.Content {
		return model.Content{SourceID: source.ID, Title: "공지 " + path, Content: body, OriginalURL: "https://fixture.invalid/" + path, OriginalCategory: "학사", Status: status, PublishedAt: &pub}
	}
	items := []model.Content{makeItem(ssu, "ongoing", "진행", "일반 안내"), makeItem(ssu, "unknown", "unknown", "일반 안내"), makeItem(ssu, "completed", "완료", "일반 안내"), makeItem(ssu, "completed-expired", "완료", "신청마감: "+past), makeItem(ssu, "ongoing-expired", "진행", "신청마감: "+past), makeItem(department, "department-completed", "완료", "일반 안내"), makeItem(alias, "alias-completed", "완료", "일반 안내")}
	if e := s.Save(ctx, items); e != nil {
		t.Fatal(e)
	}
	snapshots, _, e := lifecycle.Read(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	assessment := lifecycle.Preview(snapshots, now)
	if _, e = lifecycle.Apply(ctx, db, snapshots, assessment); e != nil {
		t.Fatal(e)
	}
	_, before, e := lifecycle.Read(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	h := (api.API{Store: s, Token: "completed-visibility-test"}).Handler()
	for _, tc := range []struct {
		query         string
		status, count int
	}{{"", 200, 3}, {"?include_completed=true", 200, 5}, {"?include_expired=true", 200, 4}, {"?include_completed=true&include_expired=true", 200, 7}, {"?lifecycle_status=expired", 200, 1}, {"?lifecycle_status=expired&include_completed=true", 200, 2}, {"?source=department", 200, 1}, {"?source=ssu_alias", 200, 0}, {"?source=ssu_alias&include_completed=true", 200, 1}, {"?source=ssu_catch&category=academic", 200, 2}, {"?include_completed=maybe", 400, 0}, {"?include_completed=true&include_completed=false", 400, 0}} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/contents"+tc.query, nil)
		req.Header.Set("Authorization", "Bearer completed-visibility-test")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatal(tc.query, w.Code, w.Body.String())
		}
		if tc.status == 200 {
			var out struct {
				Items []model.Content `json:"items"`
			}
			if e = json.Unmarshal(w.Body.Bytes(), &out); e != nil || len(out.Items) != tc.count {
				t.Fatal(tc.query, len(out.Items), tc.count, e)
			}
		}
	}
	// Limit/offset must apply after the completion filter, never over a sparse page.
	filtered, e := s.Contents(ctx, model.Filter{Page: 2, Limit: 2})
	if e != nil || len(filtered) != 1 {
		t.Fatal("filtered pagination", filtered, e)
	}
	// Policy is list-only: direct links and internal maintenance retain access.
	all, e := s.Contents(ctx, model.Filter{IncludeCompleted: true, IncludeExpired: true, Page: 1, Limit: 20})
	if e != nil || len(all) != 7 {
		t.Fatal(all, e)
	}
	for _, v := range all {
		if v.Status == "완료" && v.LifecycleStatus != "expired" {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/contents/"+v.ID.Hex(), nil)
			req.Header.Set("Authorization", "Bearer completed-visibility-test")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != 200 {
				t.Fatal("completed direct link hidden", w.Code)
			}
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/contents?include_completed=true", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal("auth bypass")
	}
	_, after, e := lifecycle.Read(ctx, db)
	if e != nil || before != after {
		t.Fatal("read visibility changed stored values", e)
	}
}
