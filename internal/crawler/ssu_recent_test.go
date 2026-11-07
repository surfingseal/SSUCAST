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
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecentCategoryURLsAndActualPagination(t *testing.T) {
	localfixture.Require(t, "testdata/ssu_catch/recent_categories/provenance.json")
	metaRaw, e := os.ReadFile("testdata/ssu_catch/recent_categories/provenance.json")
	if e != nil {
		t.Fatal(e)
	}
	var provenance struct {
		Artifacts map[string]struct {
			SHA256 string `json:"sha256"`
		} `json:"artifacts"`
	}
	if e = json.Unmarshal(metaRaw, &provenance); e != nil {
		t.Fatal(e)
	}
	for _, spec := range SSUOfficialCategories {
		u, e := SSUCategoryURL(SSUCatchOrigin, spec)
		if e != nil {
			t.Fatal(e)
		}
		p, _ := url.Parse(u)
		if p.Query().Get("category") != spec.Query || p.Query().Get("f") != "" || p.Path != ssuNoticePath || strings.Contains(u, " ") {
			t.Fatal(u)
		}
		localfixture.Require(t, "testdata/ssu_catch/recent_categories/"+spec.Code+".actual.html")
		raw, e := os.ReadFile("testdata/ssu_catch/recent_categories/" + spec.Code + ".actual.html")
		if e != nil {
			t.Fatal(e)
		}
		if fmt.Sprintf("%x", sha256.Sum256(raw)) != provenance.Artifacts[spec.Code].SHA256 {
			t.Fatal("actual category capture changed")
		}
		page, e := parseRecentPage(raw, u, nil)
		if e != nil || page.RawCount != 15 || len(page.Issues) != 0 {
			t.Fatal(spec.Code, page.RawCount, e, page.Issues)
		}
		n, e := recentNext(u, page.Next, spec.Query)
		if e != nil {
			t.Fatal(e)
		}
		np, _ := url.Parse(n)
		if np.Query().Get("category") != spec.Query || !strings.HasSuffix(np.Path, "/page/2/") {
			t.Fatal(n)
		}
	}
	for _, ref := range []string{"/공지사항/page/2/", "https://evil.invalid/공지사항/page/2/?category=학사", "/공지사항/page/2/?category=장학"} {
		if _, e := recentNext(SSUCatchOrigin+ssuNoticePath, ref, "학사"); e == nil {
			t.Fatal("unverified category pagination accepted", ref)
		}
	}
}
func TestRecentCalendarDateRange(t *testing.T) {
	now := time.Date(2026, 11, 5, 16, 30, 0, 0, time.UTC)
	from, to := SSURecentRange(now)
	if from.Format(time.RFC3339) != "2026-08-06T00:00:00+09:00" || to.Format("2006-01-02") != "2026-11-06" {
		t.Fatal(from, to)
	}
	may := time.Date(2026, 5, 31, 12, 0, 0, 0, to.Location())
	got, _ := SSURecentRange(may)
	want := time.Date(2026, 5, 31, 0, 0, 0, 0, to.Location()).AddDate(0, -3, 0)
	if !got.Equal(want) || got.Equal(may.Add(-90*24*time.Hour)) {
		t.Fatal("must use calendar AddDate", got, want)
	}
}
func recentTestRow(id, date, category string, pinned bool) string {
	return fmt.Sprintf(`<li data-fixture-pinned="%t"><div class="notice_col1"><div class="h2">%s</div></div><div class="notice_col2"><span class="tag">진행</span></div><div class="notice_col3"><a href="/공지사항/?slug=%s"><span class="label">%s</span>참가자 모집 %s</a></div><div class="notice_col4">등록부서</div></li>`, pinned, date, id, category, id)
}
func recentTestPage(rows, next string) string {
	x := `<ul class="notice-lists">` + rows + `</ul>`
	if next != "" {
		x += `<nav class="board-pagination"><a class="next-btn" href="` + html.EscapeString(next) + `">next</a></nav>`
	}
	return x
}
func TestRecentEightCategoriesBoundaryPinnedAndNoAI(t *testing.T) {
	var mu sync.Mutex
	details := map[string]int{}
	pages := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			allow(w)
			return
		}
		if id := r.URL.Query().Get("slug"); id != "" {
			mu.Lock()
			details[id]++
			mu.Unlock()
			if id == "old" || id == "invalid" || id == "future" || id == "job" || id == "pinned-old" {
				t.Error("ineligible detail requested", id)
			}
			fmt.Fprint(w, `<div class="fixture-body">학과 참가자 프로그램 안내 본문입니다. 행사 참가자 모집을 채용으로 제외하지 않습니다.<img src="/poster.png"></div>`)
			return
		}
		cat := r.URL.Query().Get("category")
		mu.Lock()
		pages[cat]++
		mu.Unlock()
		next := func(n int) string {
			u, _ := url.Parse("/공지사항/page/" + fmt.Sprint(n) + "/")
			u.RawQuery = url.Values{"category": {cat}, "f": {""}, "keyword": {""}}.Encode()
			return u.String()
		}
		switch cat {
		case "학사":
			switch r.URL.Path {
			case ssuNoticePath:
				fmt.Fprint(w, recentTestPage(recentTestRow("pinned-old", "2026.07.01", "학사", true)+recentTestRow("fresh", "2026.11.06", "학사", false)+recentTestRow("boundary", "2026.08.06", "학사", false)+recentTestRow("invalid", "bad", "학사", false)+recentTestRow("future", "2026.11.07", "학사", false)+recentTestRow("job", "2026.11.06", "교원채용", false), next(2)))
			case ssuNoticePath + "page/2/":
				fmt.Fprint(w, recentTestPage(recentTestRow("pinned-old", "2026.07.01", "학사", true)+recentTestRow("old", "2026.07.30", "학사", false), next(3)))
			default:
				fmt.Fprint(w, recentTestPage(recentTestRow("late-new", "2026.09.01", "학사", false), ""))
			}
		case "장학":
			fmt.Fprint(w, recentTestPage(recentTestRow("boundary", "2026.08.06", "장학", false), ""))
		case "교직":
			fmt.Fprint(w, recentTestPage(recentTestRow("teacher", "2026.10.01", "교직", false), ""))
		default:
			fmt.Fprint(w, recentTestPage("", ""))
		}
	}))
	defer srv.Close()
	f := NewFetcher(3 * time.Second)
	f.FixtureOrigin = srv.URL
	var report SSURecentReport
	c := SSUCatchCrawler{Fetcher: f, Now: func() time.Time { return time.Date(2026, 11, 6, 14, 0, 0, 0, time.FixedZone("Asia/Seoul", 9*3600)) }, RecentReport: func(r SSURecentReport) { report = r }, RecentPinned: func(s *goquery.Selection) bool { return s.AttrOr("data-fixture-pinned", "") == "true" }}
	source := model.Source{ID: bson.NewObjectID(), Name: "ssu_catch", CollectionMode: "recent_three_months", ClassificationStrategy: "source", BaseURL: srv.URL, ListURL: ssuNoticePath, PolicyApproved: true, DetailBodySelector: ".fixture-body", MaxPages: 5, MaxContents: 30, MaxRequests: 80, RequestIntervalMS: 1}
	items, e := c.Crawl(context.Background(), source)
	if e != nil {
		t.Fatal(e)
	}
	if len(items) != 4 || len(pages) != 8 || pages["학사"] != 3 || report.VertexRequests != 0 {
		t.Fatal(len(items), pages, report, e)
	}
	if details["boundary"] != 1 || details["fresh"] != 1 || details["late-new"] != 1 || len(details) != 4 {
		t.Fatal("eligibility/dedup failed", details)
	}
	for _, v := range items {
		if v.ClassificationMethod != "source" || len(v.ImageURLs) != 1 {
			t.Fatal(v)
		}
		if strings.Contains(v.OriginalURL, "boundary") && len(v.Categories) != 2 {
			t.Fatal("memberships not merged", v.Categories)
		}
	}
	academic := report.Categories["academic"]
	if academic.DateInvalid != 1 || academic.FutureExcluded != 1 || academic.RecruitmentExcluded != 1 || academic.OrderVerified {
		t.Fatal(academic)
	}
	if report.Status != "completed_with_review" {
		t.Fatal(report.Status)
	}
}
func TestRecentPolicyDeniedHasNoHTTP(t *testing.T) {
	c := SSUCatchCrawler{}
	_, e := c.Crawl(context.Background(), model.Source{Name: "ssu_catch", BaseURL: SSUCatchOrigin, CollectionMode: "recent_three_months"})
	code(t, e, "policy_unreviewed")
}

func TestRecentOrderingLimitsAndRepeatedPages(t *testing.T) {
	for _, mode := range []string{"ordered", "repeat", "limit", "empty"} {
		t.Run(mode, func(t *testing.T) {
			visited := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/robots.txt" {
					allow(w)
					return
				}
				if r.URL.Query().Get("slug") != "" {
					fmt.Fprint(w, `<div class="body-reviewed">실제 공지 대신 사용하는 합성 본문입니다.</div>`)
					return
				}
				cat := r.URL.Query().Get("category")
				if cat != "학사" || mode == "empty" {
					fmt.Fprint(w, recentTestPage("", ""))
					return
				}
				visited++
				u, _ := url.Parse("/공지사항/page/2/")
				u.RawQuery = url.Values{"category": {cat}, "keyword": {""}, "f": {""}}.Encode()
				if visited == 1 {
					fmt.Fprint(w, recentTestPage(recentTestRow("new", "2026.11.01", "학사", false), u.String()))
					return
				}
				if mode == "repeat" {
					fmt.Fprint(w, recentTestPage(recentTestRow("new", "2026.11.01", "학사", false), u.String()))
					return
				}
				if visited > 2 {
					t.Error("ordered old page should stop before page 3")
				}
				u.Path = "/공지사항/page/3/"
				fmt.Fprint(w, recentTestPage(recentTestRow("old", "2026.07.01", "학사", false), u.String()))
			}))
			defer srv.Close()
			f := NewFetcher(time.Second)
			f.FixtureOrigin = srv.URL
			var report SSURecentReport
			c := SSUCatchCrawler{Fetcher: f, Now: func() time.Time { return time.Date(2026, 11, 6, 12, 0, 0, 0, time.FixedZone("Asia/Seoul", 9*3600)) }, RecentReport: func(v SSURecentReport) { report = v }}
			maxPages := 5
			if mode == "limit" {
				maxPages = 1
			}
			_, e := c.Crawl(context.Background(), model.Source{Name: "ssu_catch", CollectionMode: "recent_three_months", ClassificationStrategy: "source", BaseURL: srv.URL, ListURL: ssuNoticePath, PolicyApproved: true, DetailBodySelector: ".body-reviewed", RequestIntervalMS: 1, MaxPages: maxPages, MaxContents: 30, MaxRequests: 50})
			if mode == "ordered" {
				if e != nil || visited != 2 || report.Categories["academic"].StopReason != "all_general_old_observed_descending" {
					t.Fatal(e, visited, report)
				}
			} else if e == nil {
				t.Fatal("incomplete or empty crawl reported success", mode)
			}
		})
	}
}

func TestRecentDateCheckedBeforeOptionalOldFields(t *testing.T) {
	localfixture.Require(t, "testdata/ssu_catch/recent_categories/old-empty-department.actual.html")
	raw, e := os.ReadFile("testdata/ssu_catch/recent_categories/old-empty-department.actual.html")
	if e != nil {
		t.Fatal(e)
	}
	from, to := SSURecentRange(time.Date(2026, 11, 6, 12, 0, 0, 0, time.FixedZone("Asia/Seoul", 9*3600)))
	p, e := parseRecentPage(raw, SSUCatchOrigin+"/공지사항/page/3/?category=기타", nil, from, to)
	if e != nil || p.OldCount != 15 || len(p.Rows) != 0 || len(p.Issues) != 0 {
		t.Fatal("old rows with blank department must be date-excluded before schema checks", p, e)
	}
}
