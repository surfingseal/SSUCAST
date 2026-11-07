package notices

import (
	"example.com/crawlserver/internal/model"
	"net/url"
	"testing"
	"time"
)

func ptr(s string) *string { return &s }
func TestFilters(t *testing.T) {
	for _, q := range []string{"query=장학&source=ssu_catch&category=scholarship&page=2&limit=12", "upcoming=false&has_schedule=true&status=unknown&sort=event"} {
		if _, e := Parse(mustURL(q)); e != nil {
			t.Fatal(e)
		}
	}
	for _, q := range []string{"page=0", "page=10001", "limit=51", "query=a&query=b", "sort=random", "has_schedule=1", "category=career", "source=%24where", "status=anything", "query=%00", "unknown=1"} {
		if _, e := Parse(mustURL(q)); e == nil {
			t.Fatal("accepted", q)
		}
	}
}
func mustURL(s string) url.Values { v, _ := url.ParseQuery(s); return v }
func TestScheduleCalendarAndReview(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, Seoul)
	base := model.Schedule{Type: "application", Timezone: "Asia/Seoul", EndDate: ptr("2026-11-07"), Precision: "date", Quality: &model.ScheduleQuality{State: "partial"}}
	if v := ScheduleView(base, now); v.State != "신청 중" || v.EndLabel != "2026-11-07" || v.Dates.EndTime != nil {
		t.Fatal(v)
	}
	if v := ScheduleView(base, now.Add(12*time.Hour)); v.State != "신청 마감" {
		t.Fatal(v)
	}
	base.StartDate = ptr("2026-11-08")
	base.EndDate = ptr("2026-11-10")
	if v := ScheduleView(base, now); v.State != "신청 예정" {
		t.Fatal(v)
	}
	base.StartDate = nil
	base.EndDate = ptr("2026-11-07")
	base.EndTime = ptr("11:30")
	if v := ScheduleView(base, now); v.State != "신청 마감" {
		t.Fatal(v)
	}
	base.Type = "event"
	base.EndDate = ptr("2026-11-09")
	if v := ScheduleView(base, now); v.State != "행사 예정" {
		t.Fatal(v)
	}
	base.ReviewRequired = true
	if v := ScheduleView(base, now); v.State != "원문 확인 필요" {
		t.Fatal(v)
	}
	base.ReviewRequired = false
	base.Quality.State = "blocked"
	if v := ScheduleView(base, now); !v.Review {
		t.Fatal(v)
	}
}
func TestPartialAndUnanalyzed(t *testing.T) {
	month := 11
	s := model.Schedule{Timezone: "Asia/Seoul", EndPartial: &model.CalendarParts{Month: &month}, Quality: &model.ScheduleQuality{State: "partial"}}
	v := ScheduleView(s, time.Now())
	if v.State != "일정 미상" || v.EndLabel != "11월 (연도 미상)" {
		t.Fatal(v)
	}
	c := model.Content{Title: "<script>alert(1)</script>", OriginalURL: "javascript:alert(1)", Schedules: []model.Schedule{s}}
	n := View(c, true, time.Now())
	if n.Analyzed || n.OriginalURL != "" || len(n.Schedules) != 0 {
		t.Fatal(n)
	}
	for _, u := range []string{"javascript:alert(1)", "http://example.com", "https://user:pass@example.com", "https://example.com:1234/path", "//example.com"} {
		if SafeURL(u) != "" {
			t.Fatal(u)
		}
	}
	if SafeURL("https://media.ssu.ac.kr/board/notices/1814") == "" {
		t.Fatal("safe URL blocked")
	}
}
