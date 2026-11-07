package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/service"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"go.mongodb.org/mongo-driver/v2/bson"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSSULocalMongoRunnerAPI(t *testing.T) {
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("set TEST_MONGODB_URI for real MongoDB test with local HTML")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db := "crawltest_" + bson.NewObjectID().Hex()
	store, err := repository.Open(ctx, uri, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if e := store.Client.Database(db).Drop(cleanup); e != nil {
			t.Error(e)
		}
		if e := store.Client.Disconnect(cleanup); e != nil {
			t.Error(e)
		}
	}()
	raw, err := os.ReadFile("../crawler/testdata/ssu_catch/list.actual.html")
	if err != nil {
		t.Fatal(err)
	}
	detail, err := os.ReadFile("../crawler/testdata/ssu_catch/detail.synthetic.html")
	if err != nil {
		t.Fatal(err)
	}
	// Two rows from the verified list DOM, categories replaced for local API cases.
	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(raw))
	doc.Find("ul.notice-lists > li:not(.notice_head)").Each(func(i int, s *goquery.Selection) {
		if i > 1 {
			s.Remove()
			return
		}
		label := "장학"
		if i == 1 {
			label = "학사"
		}
		s.Find(".label").SetText(label)
	})
	html, _ := doc.Html()
	var hits, revision atomic.Int32
	revision.Store(1)
	var web *httptest.Server
	web = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/robots.txt" {
			fmt.Fprint(w, "User-agent: *\nAllow: /\nDisallow: /download.php\n")
			return
		}
		if r.URL.Path != "/공지사항/" {
			t.Errorf("unexpected path %s", r.URL)
			w.WriteHeader(403)
			return
		}
		if r.URL.Query().Get("slug") == "" {
			fmt.Fprint(w, strings.ReplaceAll(html, crawler.SSUCatchOrigin, web.URL))
			return
		}
		fmt.Fprint(w, strings.Replace(string(detail), "장학금 신청 안내", fmt.Sprintf("로컬 테스트 공지 본문 버전 %d", revision.Load()), 1))
	}))
	defer web.Close()
	no := false
	source := model.Source{ID: bson.NewObjectID(), Name: "ssu_catch", CrawlerType: "ssu_catch", BaseURL: web.URL, ListURL: "/공지사항/", Enabled: true, AutoEnabled: &no, PolicyApproved: true, DetailBodySelector: ".fixture-notice-body", MaxPages: 1, MaxContents: 2, MaxRequests: 5, MaxConcurrency: 1, RequestIntervalMS: 2000, Schedule: "0 */6 * * *"}
	if err = store.ApplySources(ctx, []model.Source{source}); err != nil {
		t.Fatal(err)
	}
	f := crawler.NewFetcher(2 * time.Second)
	f.FixtureOrigin = web.URL
	registry, err := crawler.DefaultRegistry(f)
	if err != nil {
		t.Fatal(err)
	}
	runner := service.NewWithRegistry(store, registry, 1, 20*time.Second)
	defer runner.Stop()
	server := httptest.NewServer((api.API{Store: store, Runner: runner, Token: "local-ssu-integration-token"}).Handler())
	defer server.Close()
	request := func(method, path, body, auth string, target any) int {
		t.Helper()
		r, e := http.NewRequestWithContext(ctx, method, server.URL+path, strings.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		if e = json.NewDecoder(resp.Body).Decode(target); e != nil {
			t.Fatal(e)
		}
		return resp.StatusCode
	}
	token := "local-ssu-integration-token"
	run := func() model.Job {
		t.Helper()
		var j model.Job
		if status := request("POST", "/api/v1/jobs", `{"source_id":"`+source.ID.Hex()+`"}`, token, &j); status != 202 {
			t.Fatalf("POST=%d %+v", status, j)
		}
		for j.Status == "running" {
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(20 * time.Millisecond):
			}
			if request("GET", "/api/v1/jobs/"+j.ID.Hex(), "", token, &j) != 200 {
				t.Fatal("job query")
			}
		}
		return j
	}
	read := func() []model.Content {
		t.Helper()
		var result struct {
			Items []model.Content `json:"items"`
		}
		if request("GET", "/api/v1/contents?source=ssu_catch", "", token, &result) != 200 {
			t.Fatal("source query")
		}
		return result.Items
	}
	j := run()
	if j.Status != "succeeded" || j.CollectedCount != 2 {
		t.Fatal(j)
	}
	first := read()
	if len(first) != 2 {
		t.Fatal(first)
	}
	j = run()
	if j.Status != "succeeded" {
		t.Fatal(j)
	}
	again := read()
	if len(again) != 2 {
		t.Fatal(again)
	}
	byID := map[bson.ObjectID]model.Content{}
	for _, v := range first {
		byID[v.ID] = v
	}
	for _, v := range again {
		old := byID[v.ID]
		if old.ID.IsZero() || !v.UpdatedAt.Equal(old.UpdatedAt) || !v.LastSeenAt.After(old.LastSeenAt) {
			t.Fatal("repeat changed content or failed to touch", v)
		}
	}
	revision.Store(2)
	j = run()
	if j.Status != "succeeded" {
		t.Fatal(j)
	}
	changed := read()
	for _, v := range changed {
		if !strings.Contains(v.Content, "버전 2") || v.ContentHash == byID[v.ID].ContentHash || !v.UpdatedAt.After(byID[v.ID].UpdatedAt) || v.Department == "" || v.OriginalURL == "" || v.PublishedAt == nil || v.Deadline != nil || len(v.Attachments) != 2 {
			t.Fatal(v)
		}
	}
	for _, category := range []string{"scholarship", "academic"} {
		var result struct {
			Items []model.Content `json:"items"`
		}
		if status := request("GET", "/api/v1/contents?category="+category, "", token, &result); status != 200 || len(result.Items) != 1 {
			t.Fatal(category, status, result)
		}
	}
	var unauthorized map[string]any
	if request("GET", "/api/v1/contents?source=ssu_catch", "", "", &unauthorized) != 401 {
		t.Fatal("authentication bypass")
	}
	// Policy-blocked job must be recorded and make no request to the local site.
	before := hits.Load()
	source.PolicyApproved = false
	if err = store.ApplySources(ctx, []model.Source{source}); err != nil {
		t.Fatal(err)
	}
	j = run()
	if j.Status != "failed" || j.ErrorCode != "policy_unreviewed" || j.ErrorStage != "policy" || hits.Load() != before {
		t.Fatal(j, hits.Load())
	}
	t.Logf("LOCAL ONLY: DB=%s documents=2 scholarship=1 academic=1 source=2 repeat=2 update=2 authenticated_API=200 unauthenticated=401 HTTP_attempts=%d live_SSU_details=0", db, hits.Load())
	if output := os.Getenv("SSU_TEST_REPORT"); output != "" {
		report := map[string]any{"mode": "local_fixture_only", "test": t.Name(), "database": db, "database_cleanup": "deferred Drop; see test result", "live_ssu_detail_requests": 0, "local_http_attempts": hits.Load(), "stored_documents": len(changed), "repeat_document_count": len(again), "changed_documents": 2, "api": map[string]any{"scholarship": map[string]int{"status": 200, "count": 1}, "academic": map[string]int{"status": 200, "count": 1}, "source=ssu_catch": map[string]int{"status": 200, "count": 2}, "unauthenticated_status": 401}, "documents": changed}
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
