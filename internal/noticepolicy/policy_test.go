package noticepolicy

import (
	"example.com/crawlserver/internal/model"
	"reflect"
	"testing"
	"time"
)

func ip(v int) *int       { return &v }
func sp(v string) *string { return &v }
func candidate() model.Schedule {
	return model.Schedule{ID: "s1", Type: "application", Timezone: "Asia/Seoul", EndPartial: &model.CalendarParts{Month: ip(11), Day: ip(5)}, ReviewRequired: true, ReviewReasons: []string{"missing_year"}, Quality: &model.ScheduleQuality{State: "uncertain"}}
}
func TestUserYearAndExpiration(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, Seoul)
	s := candidate()
	v := model.Content{Schedules: []model.Schedule{s}}
	p, e := Project(v, 2026, now)
	if e != nil || p.BoundariesFilled != 1 || p.Schedules[0].EndDate == nil || *p.Schedules[0].EndDate != "2026-11-05" || p.Schedules[0].EndTime != nil || p.Schedules[0].ReviewRequired || p.ExpiresAt == nil || p.ExpiresAt.After(now) {
		t.Fatal(p, e)
	}
	if !reflect.DeepEqual(v.Schedules, []model.Schedule{s}) {
		t.Fatal("raw schedules mutated")
	}
	if p.Schedules[0].EndAt != nil {
		t.Fatal("invented midnight timestamp")
	}
	// The date alone remains active throughout that day.
	p, e = Project(v, 2026, time.Date(2026, 11, 5, 23, 59, 59, 0, Seoul))
	if e != nil || !p.ExpiresAt.After(time.Date(2026, 11, 5, 23, 59, 59, 0, Seoul)) {
		t.Fatal("early expiration")
	}
}
func TestAllApplicationScopesAndExplicitYears(t *testing.T) {
	s := candidate()
	future := candidate()
	future.EndPartial.Day = ip(20)
	v := model.Content{Schedules: []model.Schedule{s, future}}
	p, _ := Project(v, 2026, time.Now())
	if p.ExpiresAt == nil || p.ExpiresAt.In(Seoul).Day() != 21 {
		t.Fatal(p)
	}
	future.EndPartial.Day = nil
	v.Schedules[1] = future
	p, _ = Project(v, 2026, time.Now())
	if p.ExpiresAt != nil {
		t.Fatal("month-only scope hidden")
	}
	v.Schedules = []model.Schedule{{Type: "event", Timezone: "Asia/Seoul", EndDate: sp("2027-01-20"), Quality: &model.ScheduleQuality{State: "partial"}}}
	p, _ = Project(v, 2026, time.Now())
	if *p.Schedules[0].EndDate != "2027-01-20" || p.ExpiresAt != nil {
		t.Fatal("explicit future year/event corrupted")
	}
}
func TestAmbiguityInvalidCalendarAndRollover(t *testing.T) {
	s := candidate()
	s.ReviewReasons = append(s.ReviewReasons, "conditional_deadline")
	p, _ := Project(model.Content{Schedules: []model.Schedule{s}}, 2026, time.Now())
	if !p.Schedules[0].ReviewRequired || p.ExpiresAt != nil {
		t.Fatal(p)
	}
	s = candidate()
	s.EndPartial.Month = ip(2)
	s.EndPartial.Day = ip(29)
	p, _ = Project(model.Content{Schedules: []model.Schedule{s}}, 2026, time.Now())
	if p.Schedules[0].EndDate != nil || !p.Schedules[0].ReviewRequired {
		t.Fatal(p)
	}
	s = candidate()
	s.StartPartial = &model.CalendarParts{Month: ip(12), Day: ip(20)}
	s.EndPartial = &model.CalendarParts{Month: ip(1), Day: ip(5)}
	p, _ = Project(model.Content{Schedules: []model.Schedule{s}}, 2026, time.Now())
	if p.ExpiresAt != nil || p.Schedules[0].Quality.State != "blocked" {
		t.Fatal("invented rollover", p)
	}
}
func TestPolicyInvalidatesOnChangedInput(t *testing.T) {
	v := model.Content{ContentHash: "a", ScheduleMetadata: model.ScheduleMetadata{ScheduleAnalysisID: "a", ScheduleInputHash: "b"}}
	p, _ := Project(v, 2026, time.Now())
	v.NoticePolicy = &p
	if !Valid(v) {
		t.Fatal("policy not linked")
	}
	v.ContentHash = "changed"
	if Valid(v) {
		t.Fatal("stale policy valid")
	}
}

func TestImageYearNeverAssumed(t *testing.T) {
	s := candidate()
	s.ImageEvidence = []model.ImageEvidence{{ImageURL: "https://example.invalid/poster.png", Text: "11월5일"}}
	p, _ := Project(model.Content{Schedules: []model.Schedule{s}}, 2026, time.Now())
	if p.Schedules[0].EndDate != nil || p.Schedules[0].EndPartial.Year != nil || p.ExpiresAt != nil {
		t.Fatal("image year auto-filled", p)
	}
}
