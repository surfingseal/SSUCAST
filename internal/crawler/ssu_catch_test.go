package crawler

import (
	"context"
	"errors"
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

func ssuFixture(t *testing.T, name string) []byte {
	t.Helper()
	if !strings.Contains(name, "synthetic") {
		localfixture.Require(t, "testdata/ssu_catch/"+name)
	}
	b, e := os.ReadFile("testdata/ssu_catch/" + name)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestSSUActualList(t *testing.T) {
	rows, next, err := ParseSSUCatchList(ssuFixture(t, "list.actual.html"), SSUCatchOrigin+ssuNoticePath)
	if err != nil || len(rows) != 11 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	first := rows[0]
	if first.Category != "volunteer" || first.OriginalCategory != "봉사" || first.Department != "사회공헌팀" || first.Status != "진행" || !strings.HasPrefix(first.Title, "[초록우산]") || strings.HasPrefix(first.Title, "봉사") {
		t.Fatal(first)
	}
	if first.PublishedAt.Format(time.RFC3339) != "2026-11-03T15:00:00Z" || !strings.HasSuffix(next, "/page/2/") {
		t.Fatal(first.PublishedAt, next)
	}
	if strings.Contains(first.OriginalURL, "category=") || strings.Contains(first.OriginalURL, "paged=") || !strings.Contains(first.OriginalURL, "slug=") {
		t.Fatal(first.OriginalURL)
	}
}
func TestSSUCategories(t *testing.T) {
	for input, want := range map[string]string{"학사": "academic", "장학": "scholarship", "채용": "unknown", "봉사": "volunteer", "비교과·행사": "extracurricular", "국제교류": "international", "외국인유학생": "international_student", "기타": "other", "교직": "teaching", "": "unknown", "교원채용": "unknown"} {
		if got := SSUCategory(input); got != want {
			t.Errorf("%s=%s want %s", input, got, want)
		}
	}
}
func TestSSUDates(t *testing.T) {
	for _, v := range []string{"2026.02.30", "2026-11-05", "", "tomorrow"} {
		if _, err := SSUDate(v); err == nil {
			t.Fatal(v)
		}
	}
	v, err := SSUDate(" 2024.02.29 ")
	if err != nil || v.Format(time.RFC3339) != "2024-02-28T15:00:00Z" {
		t.Fatal(v, err)
	}
}
func TestSSUCanonicalURL(t *testing.T) {
	base := SSUCatchOrigin + ssuNoticePath
	a, e := SSUCatchURL(base, "?category=학사&slug=증명서-발급-서비스-일시-중단-안내-5&utm_source=x#top")
	b, e2 := SSUCatchURL(base, "/공지사항/page/2/?f&paged=2&slug=증명서-발급-서비스-일시-중단-안내-5")
	if e != nil || e2 != nil || a != b {
		t.Fatal(a, b, e, e2)
	}
	for _, ref := range []string{"?slug=", "?slug=a&slug=b", "/download.php?slug=x", "https://attacker.invalid/공지사항/?slug=x", "//u:p@scatch.ssu.ac.kr/공지사항/?slug=x", "javascript:alert(1)"} {
		if _, err := SSUCatchURL(base, ref); err == nil {
			t.Fatal(ref)
		}
	}
}
func TestSSUListFailuresAndDuplicates(t *testing.T) {
	actual := string(ssuFixture(t, "list.actual.html"))
	for _, raw := range []string{"<html>empty</html>", strings.ReplaceAll(actual, "notice_col4", "removed"), strings.Replace(actual, "2026.11.04", "not-a-date", 1), "<title>CAPTCHA</title>"} {
		if _, _, err := ParseSSUCatchList([]byte(raw), SSUCatchOrigin+ssuNoticePath); err == nil {
			t.Fatal("accepted invalid HTML")
		}
	}
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(actual))
	row := doc.Find("ul.notice-lists > li:not(.notice_head)").Eq(1).Clone()
	row.Find(".label").SetText("학사")
	h, _ := goquery.OuterHtml(row)
	doc.Find("ul.notice-lists").AppendHtml(h)
	html, _ := doc.Html()
	rows, _, err := ParseSSUCatchList([]byte(html), SSUCatchOrigin+ssuNoticePath)
	if err != nil || len(rows) != 11 || len(rows[0].Categories) != 2 {
		t.Fatal(rows, err)
	}
}
func TestSSUSyntheticDetail(t *testing.T) {
	rows, _, err := ParseSSUCatchList(ssuFixture(t, "list.actual.html"), SSUCatchOrigin+ssuNoticePath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseSSUCatchDetail(ssuFixture(t, "detail.synthetic.html"), rows[0].OriginalURL, rows[0], ".fixture-notice-body")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Content, "장학금 신청 안내") || strings.Contains(got.Content, "메뉴") || strings.Contains(got.Content, "추천 글") || strings.Contains(got.Content, "exclude()") || strings.Contains(got.Content, "푸터") {
		t.Fatal(got.Content)
	}
	if len(got.Attachments) != 2 || got.Attachments[0].Name != "신청서.hwp" || !strings.Contains(got.Attachments[0].URL, "/download.php?id=123") || got.Deadline != nil {
		t.Fatal(got)
	}
	for _, selector := range []string{"", "body", "article", ".missing", "["} {
		if _, e := ParseSSUCatchDetail(ssuFixture(t, "detail.synthetic.html"), rows[0].OriginalURL, rows[0], selector); e == nil {
			t.Fatal(selector)
		}
	}
	for _, raw := range []string{`<div class="fixture-notice-body"> </div>`, `<input type="password"><div class="fixture-notice-body">hidden</div>`, `<div class="fixture-notice-body">one</div><div class="fixture-notice-body">two</div>`} {
		if _, e := ParseSSUCatchDetail([]byte(raw), rows[0].OriginalURL, rows[0], ".fixture-notice-body"); e == nil {
			t.Fatal(raw)
		}
	}
}
func TestSSUPolicyAndSchemaFailWithoutRequests(t *testing.T) {
	var calls atomic.Int32
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer web.Close()
	f := NewFetcher(time.Second)
	f.FixtureOrigin = web.URL
	c := &SSUCatchCrawler{Fetcher: f}
	source := model.Source{BaseURL: web.URL, ListURL: ssuNoticePath}
	_, err := c.Crawl(context.Background(), source)
	code(t, err, "policy_unreviewed")
	source.PolicyApproved = true
	_, err = c.Crawl(context.Background(), source)
	code(t, err, "schema_unverified")
	if calls.Load() != 0 {
		t.Fatal(calls.Load())
	}
}
func TestSSUCrawlLocalBudgetAndCancellation(t *testing.T) {
	localfixture.Require(t, "testdata/ssu_catch/list.actual.html")
	var hits atomic.Int32
	var web *httptest.Server
	web = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch {
		case r.URL.Path == "/robots.txt":
			fmt.Fprint(w, string(ssuFixture(t, "robots.txt")))
		case r.URL.Path == ssuNoticePath && r.URL.Query().Get("slug") == "":
			fmt.Fprint(w, strings.ReplaceAll(string(ssuFixture(t, "list.actual.html")), SSUCatchOrigin, web.URL))
		case r.URL.Path == ssuNoticePath && r.URL.Query().Get("slug") != "":
			fmt.Fprint(w, string(ssuFixture(t, "detail.synthetic.html")))
		default:
			t.Errorf("forbidden request: %s", r.URL)
			w.WriteHeader(403)
		}
	}))
	defer web.Close()
	f := NewFetcher(time.Second)
	f.FixtureOrigin = web.URL
	c := &SSUCatchCrawler{Fetcher: f}
	source := model.Source{ID: bson.NewObjectID(), Name: "ssu_catch", BaseURL: web.URL, ListURL: ssuNoticePath, PolicyApproved: true, DetailBodySelector: ".fixture-notice-body", MaxPages: 100, MaxContents: 100, MaxRequests: 100, RequestIntervalMS: 1, MaxConcurrency: 16}
	started := time.Now()
	items, err := c.Crawl(context.Background(), source)
	if err != nil || len(items) != 3 || hits.Load() != 5 {
		t.Fatalf("items=%d hits=%d err=%v", len(items), hits.Load(), err)
	}
	if time.Since(started) < 8*time.Second {
		t.Fatal("2 second interval not enforced")
	}
	for _, v := range items {
		if v.SourceName != "ssu_catch" || v.SourceID != source.ID || v.ContentHash == "" || v.CrawledAt.IsZero() || v.LastSeenAt.IsZero() || v.Deadline != nil {
			t.Fatal(v)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.Crawl(ctx, source)
	if !errors.Is(err, context.Canceled) || hits.Load() != 5 {
		t.Fatal(err, hits.Load())
	}
}
func TestSSUManualOnly(t *testing.T) {
	yes := true
	s := model.Source{Name: "ssu_catch", Enabled: true, AutoEnabled: &yes}
	if s.Automatic() {
		t.Fatal("SSU schedules must remain off")
	}
}

func TestSSUAbsentStatusIsUnknown(t *testing.T) {
	rows, _, err := ParseSSUCatchList(ssuFixture(t, "list.actual.html"), SSUCatchOrigin+ssuNoticePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if strings.Contains(row.Title, "통학로") {
			if row.Status != "unknown" || row.Category != "scholarship" {
				t.Fatal(row)
			}
			return
		}
	}
	t.Fatal("통학로 notice absent")
}

func TestSSUExcludedCategoriesNeverFetchDetails(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(ssuFixture(t, "list.actual.html"))))
	if err != nil {
		t.Fatal(err)
	}
	doc.Find("ul.notice-lists .label").SetText("채용")
	doc.Find("ul.notice-lists .label").First().SetText("교원채용")
	html, err := doc.Html()
	if err != nil {
		t.Fatal(err)
	}
	var hits, details atomic.Int32
	var web *httptest.Server
	web = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/robots.txt" {
			fmt.Fprint(w, "User-agent: *\nAllow: /\n")
			return
		}
		if r.URL.Query().Get("slug") != "" {
			details.Add(1)
			t.Error("excluded detail requested")
			w.WriteHeader(403)
			return
		}
		fmt.Fprint(w, strings.ReplaceAll(html, SSUCatchOrigin, web.URL))
	}))
	defer web.Close()
	f := NewFetcher(time.Second)
	f.FixtureOrigin = web.URL
	source := model.Source{BaseURL: web.URL, ListURL: ssuNoticePath, PolicyApproved: true, DetailBodySelector: ".fixture-notice-body", MaxPages: 1, MaxContents: 3, MaxRequests: 5, RequestIntervalMS: 2000}
	items, err := (&SSUCatchCrawler{Fetcher: f}).Crawl(context.Background(), source)
	if err != nil || len(items) != 0 || details.Load() != 0 || hits.Load() != 2 {
		t.Fatal(len(items), err, hits.Load(), details.Load())
	}
}
func TestSSUExcludedLabelOnDuplicateBlocksWholeNotice(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(ssuFixture(t, "list.actual.html"))))
	if err != nil {
		t.Fatal(err)
	}
	row := doc.Find("ul.notice-lists > li:not(.notice_head)").Eq(1).Clone()
	row.Find(".label").SetText("교원채용")
	fragment, _ := goquery.OuterHtml(row)
	doc.Find("ul.notice-lists").AppendHtml(fragment)
	html, _ := doc.Html()
	items, _, err := ParseSSUCatchList([]byte(html), SSUCatchOrigin+ssuNoticePath)
	if err != nil || len(items) != 10 {
		t.Fatal(len(items), err)
	}
	for _, item := range items {
		if strings.Contains(item.Title, "우리함께런") {
			t.Fatal("mixed excluded category was accepted", item)
		}
	}
}
