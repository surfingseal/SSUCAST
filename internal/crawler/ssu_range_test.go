package crawler

import (
	"context"
	"example.com/crawlserver/internal/model"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHistoricalRangeInclusiveAndRealCollectionTime(t *testing.T) {
	bounds, e := ParseSSUCollectionRange("2025-11-01", "2026-02-28")
	if e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, bounds.From.Location())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			allow(w)
			return
		}
		if r.URL.Query().Get("slug") != "" {
			fmt.Fprint(w, `<div class="fixture-body">장학금 참가자 지원 신청을 받습니다. 필요한 서류를 준비해 주세요.</div>`)
			return
		}
		if r.URL.Query().Get("category") != "장학" {
			fmt.Fprint(w, recentTestPage("", ""))
			return
		}
		fmt.Fprint(w, recentTestPage(recentTestRow("before", "2025.10.31", "장학", false)+recentTestRow("first", "2025.11.01", "장학", false)+recentTestRow("last", "2026.02.28", "장학", false)+recentTestRow("after", "2026.03.01", "장학", false), ""))
	}))
	defer srv.Close()
	f := NewFetcher(time.Second)
	f.FixtureOrigin = srv.URL
	s := model.Source{Name: "ssu_catch", CollectionMode: "recent_three_months", ClassificationStrategy: "source", BaseURL: srv.URL, ListURL: ssuNoticePath, PolicyApproved: true, DetailBodySelector: ".fixture-body", MaxPages: 3, MaxContents: 10, MaxRequests: 20, RequestIntervalMS: 1}
	var report SSURecentReport
	c := SSUCatchCrawler{Fetcher: f, CollectionRange: bounds, Now: func() time.Time { return now }, RecentReport: func(r SSURecentReport) { report = r }}
	rows, e := c.Crawl(context.Background(), s)
	if e != nil || len(rows) != 2 {
		t.Fatalf("rows=%d err=%v", len(rows), e)
	}
	if !report.ExecutedAt.Equal(now) || report.DateTo.Day() != 28 || report.Categories["scholarship"].FutureExcluded != 1 || len(report.Categories["scholarship"].Issues) != 0 {
		t.Fatal(report)
	}
	for _, r := range rows {
		if r.CrawledAt.Year() < 2026 || r.CrawledAt.Before(*r.PublishedAt) {
			t.Fatal("collection timestamp backdated")
		}
	}
}

func TestHistoricalRangeRejectsBadBounds(t *testing.T) {
	for _, r := range [][2]string{{"2025-02-30", "2025-03-01"}, {"2026-02-28", "2025-11-01"}, {"2024-01-01", "2026-01-01"}, {"2099-01-01", "2099-02-01"}} {
		if _, e := ParseSSUCollectionRange(r[0], r[1]); e == nil {
			t.Fatal(r)
		}
	}
}
