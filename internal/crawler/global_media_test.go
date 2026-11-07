package crawler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/testutil/localfixture"
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

func mediaFixture(t *testing.T, name string) []byte {
	t.Helper()
	if !strings.HasPrefix(name, "synthetic/") {
		localfixture.Require(t, "testdata/global_media/"+name)
	}
	b, e := os.ReadFile("testdata/global_media/" + name)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestGlobalMediaActualStaticEvidence(t *testing.T) {
	raw := mediaFixture(t, "list.actual.html")
	doc, e := goquery.NewDocumentFromReader(strings.NewReader(string(raw)))
	if e != nil {
		t.Fatal(e)
	}
	if doc.Find("#board-skeleton-container").Length() != 1 || clean(doc.Find("main h1").Text()) != "학부 공지" {
		t.Fatal("actual landing structure changed")
	}
	if _, _, e := ParseGlobalMediaList(raw); e == nil {
		t.Fatal("skeleton silently treated as an empty notice list")
	}
	if !strings.Contains(string(raw), `\"id\":136,\"title\":\"학부 공지\"`) {
		t.Fatal("menu ID evidence missing")
	}
	js := string(mediaFixture(t, "board-page.actual.js"))
	for _, v := range []string{`u.GET("/v1/board/"`, `size:"15"`, `Math.abs(parseInt(p))-1`, `t.isPinned`, `t.userName`, `t.createdAt`, `t.content`, `<redacted>`, `previous_page=`} {
		if !strings.Contains(js, v) {
			t.Fatalf("missing client evidence %s", v)
		}
	}
	var p struct {
		Artifacts                []struct{ Path, SHA256 string }
		DirectSchoolHTTPRequests int `json:"direct_school_http_requests"`
	}
	if e := json.Unmarshal(mediaFixture(t, "provenance.json"), &p); e != nil {
		t.Fatal(e)
	}
	if p.DirectSchoolHTTPRequests != 7 {
		t.Fatal("inspection ledger changed")
	}
	for _, f := range p.Artifacts {
		if fmt.Sprintf("%x", sha256.Sum256(mediaFixture(t, f.Path))) != f.SHA256 {
			t.Fatalf("capture hash differs: %s", f.Path)
		}
	}
}
func TestGlobalMediaPolicyAndSchemaMakeZeroRequests(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	f := NewFetcher(time.Second)
	f.FixtureOrigin = srv.URL
	c := GlobalMediaCrawler{Fetcher: f}
	source := model.Source{BaseURL: srv.URL, ListURL: "/board/notices"}
	_, err := c.Crawl(context.Background(), source)
	code(t, err, "policy_unreviewed")
	source.PolicyApproved = true
	_, err = c.Crawl(context.Background(), source)
	code(t, err, "schema_unverified")
	var review GlobalMediaReview
	_ = json.Unmarshal(mediaFixture(t, "synthetic/review.json"), &review)
	review.APIOrigin = srv.URL
	c.Review = &review
	f.FixtureOrigin = "" // a synthetic review cannot enable a non-Fixture client
	_, err = c.Crawl(context.Background(), source)
	code(t, err, "schema_unverified")
	if hits.Load() != 0 {
		t.Fatal("unreviewed collector made requests")
	}
}
func TestGlobalMediaSyntheticPaginationPinnedExclusionAndNoDownloads(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		switch {
		case r.URL.Path == "/robots.txt":
			allow(w)
		case r.URL.Path == "/synthetic/list":
			if !strings.Contains(r.Header.Get("Accept"), "application/json") {
				t.Error("JSON API not accepted")
			}
			if r.URL.Query().Get("menuId") != "136" || r.URL.Query().Get("size") != "15" {
				t.Error("query contract")
			}
			_, _ = w.Write(mediaFixture(t, "synthetic/list.page"+r.URL.Query().Get("page")+".json"))
		case strings.HasPrefix(r.URL.Path, "/synthetic/detail/"):
			id := strings.TrimPrefix(r.URL.Path, "/synthetic/detail/")
			if id == "1003" || id == "1004" || id == "1005" {
				t.Error("excluded/private notice was requested")
				http.Error(w, "forbidden", 403)
				return
			}
			_, _ = w.Write(mediaFixture(t, "synthetic/detail."+id+".json"))
		default:
			t.Error("unexpected download", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	var review GlobalMediaReview
	if e := json.Unmarshal(mediaFixture(t, "synthetic/review.json"), &review); e != nil {
		t.Fatal(e)
	}
	review.APIOrigin = srv.URL
	f := NewFetcher(5 * time.Second)
	f.FixtureOrigin = srv.URL
	reasons := map[string]string{}
	c := GlobalMediaCrawler{Fetcher: f, Review: &review, Audit: func(u string, r model.ExclusionReason) { reasons[u] = r.Code }}
	source := model.Source{ID: bson.NewObjectID(), Name: "ssu_global_media", ClassificationStrategy: "hybrid", BaseURL: srv.URL, ListURL: "/board/notices", PolicyApproved: true, MaxPages: 2, MaxContents: 10, MaxRequests: 12, RequestIntervalMS: 1}
	items, e := c.Crawl(context.Background(), source)
	if e != nil {
		t.Fatal(e)
	}
	if len(items) != 3 || len(reasons) != 3 {
		t.Fatalf("items=%d reasons=%v", len(items), reasons)
	}
	first := items[0]
	if first.Author != "합성 작성자" || len(first.Attachments) != 1 || first.Attachments[0].Extension != "pdf" || first.Category != "unknown" || first.ClassificationStatus != "needs_review" || first.Metadata["is_pinned"] != "true" {
		t.Fatalf("fields missing: %+v", first)
	}
	if first.Metadata["has_images"] != "true" || first.Metadata["images_analyzed"] != "false" {
		t.Fatal("media markers missing")
	}
	for _, bad := range []string{"菜单", "본문 내 메뉴", "꼬리말", "bad()", "바깥 UI"} {
		if strings.Contains(first.Content, bad) {
			t.Fatal("body contaminated", bad)
		}
	}
	for _, v := range items {
		if strings.Contains(v.OriginalURL, "?") {
			t.Fatal("navigation query persisted")
		}
	}
	if len(paths) != 7 {
		t.Fatalf("unexpected synthetic requests %v", paths)
	} // robots + two lists + four details
	if reasons[srv.URL+"/board/notices/1006"] != "employment" {
		t.Fatal("body exclusion missing")
	}
	t.Logf("synthetic parsed=%d exclusions=%d requests=%d; paid model calls=0", len(items), len(reasons), len(paths))
}
func TestGlobalMediaMalformedAndCanonicalURL(t *testing.T) {
	for _, v := range []string{"/board/notices/42?previous_page=3&previous_search_keyword=a#x", "https://media.ssu.ac.kr/board/notices/42?page=2"} {
		u, e := GlobalMediaURL("https://media.ssu.ac.kr", v)
		if e != nil || u != "https://media.ssu.ac.kr/board/notices/42" {
			t.Fatal(u, e)
		}
	}
	for _, v := range []string{"https://evil.example/board/notices/42", "/board/notices/../new", "/board/notices/4%2F2", "/board/notices/new", "/board/notices/42/edit"} {
		if _, e := GlobalMediaURL("https://media.ssu.ac.kr", v); e == nil {
			t.Fatal("unsafe route accepted", v)
		}
	}
	for _, raw := range []string{`{}`, `{"data":{"boards":[],"totalPage":-1}}`, `{"data":{"boards":[{"id":1}],"totalPage":1}}`} {
		if _, _, e := ParseGlobalMediaList([]byte(raw)); e == nil {
			t.Fatal("invalid response accepted")
		}
	}
	var r GlobalMediaReview
	_ = json.Unmarshal(mediaFixture(t, "synthetic/review.json"), &r)
	rows, _, e := ParseGlobalMediaList(mediaFixture(t, "synthetic/list.page0.json"))
	if e != nil {
		t.Fatal(e)
	}
	bad := strings.Replace(string(mediaFixture(t, "synthetic/detail.1001.json")), `"id": 1001`, `"id": 9999`, 1)
	if _, e := ParseGlobalMediaDetail([]byte(bad), "https://fixture.invalid/board/notices/1001", rows[0], r); e == nil {
		t.Fatal("detail ID mismatch accepted")
	}
	r.BodySelector = ".not-present"
	if _, e := ParseGlobalMediaDetail(mediaFixture(t, "synthetic/detail.1001.json"), "https://fixture.invalid/board/notices/1001", rows[0], r); e == nil {
		t.Fatal("missing body silently accepted")
	}
}
