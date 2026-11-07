package schedules

import (
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"strings"
	"testing"
)

func rawV2(t *testing.T, ss ...V2Schedule) string {
	t.Helper()
	b, e := json.Marshal(V2Response{Summary: "공고 신청을 안내합니다.", Schedules: append([]V2Schedule{}, ss...)})
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func baseV2(ev string) V2Schedule {
	return V2Schedule{Type: "application", Label: "신청", Evidence: ev, Timezone: "Asia/Seoul", Precision: "date", ReviewReasons: []string{}}
}
func TestDirectV2DatePartialAndNoClockImputation(t *testing.T) {
	w := baseV2("마감: 2026년 11월 23일까지")
	w.EndDate = sp("2026-11-23")
	n, e := RevalidateV2("x", rawV2(t, w), Document{Content: w.Evidence})
	if e != nil || len(n.Schedules) != 1 || n.Schedules[0].EndTime != nil || n.Schedules[0].ReviewRequired {
		t.Fatal(n, e)
	}
	w = baseV2("신청: 11/13까지")
	w.EndDate = sp("2026-11-13")
	n, e = RevalidateV2("x", rawV2(t, w), Document{Content: w.Evidence})
	if e != nil || n.Schedules[0].EndDate != nil || n.Schedules[0].EndPartial == nil || n.Schedules[0].EndPartial.Year != nil || !n.Schedules[0].ReviewRequired {
		t.Fatal(n, e)
	}
}
func TestDirectV2RetainsUnknownAndRejectsWrongSchema(t *testing.T) {
	w := baseV2("없는 문장 2026.11.30")
	w.EndDate = sp("2026-11-30")
	n, e := RevalidateV2("x", rawV2(t, w), Document{Content: "기한 추후 공지"})
	if e != nil || len(n.Schedules) != 1 || !SafeReviewedPlaceholder(n.Schedules[0]) {
		t.Fatal(n, e)
	}
	raw := strings.Replace(rawV2(t), `"schedules":[]`, `"schedules":null`, 1)
	if _, e = DecodeV2(raw); e == nil {
		t.Fatal("null array accepted")
	}
}
func TestDirectV2ParentSessionsAndDuplicateRemoval(t *testing.T) {
	ev := "행사: 2026년 11월 25일 07:00~17:00\n* 4타임으로 구분하여 신청\n1) 07~10시\n2) 10~12시\n3) 13~15시\n4) 15~17시"
	w := baseV2(strings.Split(ev, "\n")[0])
	w.Type = "event"
	w.StartDate = sp("2026-11-25")
	w.EndDate = sp("2026-11-25")
	w.StartTime = sp("07:00")
	w.EndTime = sp("17:00")
	w.Precision = "minute"
	n, e := RevalidateV2("x", rawV2(t, w, w), Document{Content: ev})
	if e != nil || len(n.Schedules) != 1 || len(n.RuleSupplements) != 4 || len(n.SetIssues) != 1 {
		t.Fatal(n, e)
	}
}
func TestAbbreviatedDayPreservesBothBoundaries(t *testing.T) {
	s := FromLegacy("x", 0, LegacySchedule{Type: "application", Evidence: "지원: 11.2(월) 오후 2시~12(목) 오후 2시"}, Document{Content: "지원: 11.2(월) 오후 2시~12(목) 오후 2시"})
	if s.StartPartial == nil || s.EndPartial == nil || *s.EndPartial.Month != 11 || *s.EndPartial.Day != 12 || s.EndPartial.Year != nil || value(s.EndTime) != "14:00" {
		t.Fatal(s)
	}
}
func TestDistinctUncertainProposalsNotCollapsed(t *testing.T) {
	a := model.Schedule{Type: "event", Precision: "unknown", Evidence: "안내 일정 추후 공지"}
	b := a
	b.Evidence = "면담 일정 별도 공지"
	if Fingerprint(a) == Fingerprint(b) {
		t.Fatal("different uncertain evidence collapsed")
	}
}
func TestExplicitNumericMonthAndAbbreviatedYearStayPartial(t *testing.T) {
	w := baseV2("면접: 2026.11월 중")
	w.Type = "interview"
	w.Precision = "month"
	w.StartPartial = &model.CalendarParts{Year: ip(2026), Month: ip(11)}
	n, e := RevalidateV2("x", rawV2(t, w), Document{Content: w.Evidence})
	if e != nil || n.Schedules[0].StartPartial == nil || n.Schedules[0].StartDate != nil || *n.Schedules[0].StartPartial.Month != 11 {
		t.Fatal(n, e)
	}
	w = baseV2("교육기간: ’27. 1. ~ ’27. 12.")
	w.Type = "event"
	w.Precision = "month"
	w.StartPartial = &model.CalendarParts{Year: ip(2027), Month: ip(1)}
	w.EndPartial = &model.CalendarParts{Year: ip(2027), Month: ip(12)}
	n, e = RevalidateV2("x", rawV2(t, w), Document{Content: w.Evidence})
	if e != nil || n.Schedules[0].StartPartial == nil || n.Schedules[0].EndPartial == nil || n.Schedules[0].StartPartial.Year != nil || n.Schedules[0].EndPartial.Year != nil {
		t.Fatal(n, e)
	}
}
func TestBackgroundYearDoesNotBecomeRangeStart(t *testing.T) {
	w := baseV2("2026년 지원사업 안내(~11.9.월 18시)")
	w.EndDate = sp("2026-11-09")
	w.EndTime = sp("18:00")
	w.StartPartial = &model.CalendarParts{}
	w.EndPartial = &model.CalendarParts{}
	n, e := RevalidateV2("x", rawV2(t, w), Document{Title: w.Evidence})
	if e != nil || n.Schedules[0].StartDate != nil || n.Schedules[0].StartPartial != nil || n.Schedules[0].EndDate != nil || n.Schedules[0].EndPartial == nil || n.Schedules[0].EndPartial.Year != nil {
		t.Fatal(n, e)
	}
}
func TestDurationHoursAreNotClocks(t *testing.T) {
	s := FromLegacy("x", 0, LegacySchedule{Type: "event", Evidence: "11월 14일 14:00~19:00 / 5시간"}, Document{Content: "11월 14일 14:00~19:00 / 5시간"})
	if value(s.StartTime) != "14:00" || value(s.EndTime) != "19:00" || s.StartPartial == nil || s.EndPartial == nil {
		t.Fatal(s)
	}
}
