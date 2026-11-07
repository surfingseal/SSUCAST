package noticepolicy

import (
	"example.com/crawlserver/internal/model"
	"reflect"
	"strings"
	"testing"
	"time"
)

func imageSchedule() model.Schedule {
	return model.Schedule{Type: "application", EndDate: sp("2026-11-05"), Timezone: "Asia/Seoul", ReviewRequired: true, ReviewReasons: []string{"image_evidence_not_independently_verified", "image_model_review_required", "IMAGE_SCHEDULE_EXTRACTED"}, ImageEvidence: []model.ImageEvidence{{ImageURL: "https://scatch.ssu.ac.kr/wp-content/uploads/synthetic.png", ImageSHA256: strings.Repeat("a", 64), Text: "합성 접수마감2026.11.05", Confidence: "high"}}, Quality: &model.ScheduleQuality{State: "uncertain"}}
}
func TestImageUserAcceptanceAndRawPreservation(t *testing.T) {
	s := imageSchedule()
	v := model.Content{Schedules: []model.Schedule{s}}
	p, e := Project(v, 2026, time.Now())
	if e != nil || p.ImageAcceptedSchedules != 1 || p.Schedules[0].ReviewRequired || p.ExpiresAt == nil || p.Schedules[0].EndTime != nil {
		t.Fatal(p, e)
	}
	if !reflect.DeepEqual(v.Schedules[0], s) {
		t.Fatal("raw image result changed")
	}
	p, e = Project(v, 2026, time.Date(2026, 11, 5, 23, 59, 0, 0, Seoul))
	if e != nil || !p.ExpiresAt.After(p.AppliedAt) {
		t.Fatal("date-only expired before end of day")
	}
	v.Schedules[0].EndDate = sp("2026-11-20")
	p, _ = Project(v, 2026, time.Now())
	if p.ExpiresAt == nil || p.ExpiresAt.Before(time.Date(2026, 11, 20, 0, 0, 0, 0, Seoul)) {
		t.Fatal("future image application hidden")
	}
}
func TestImageActualUncertaintyNotCleared(t *testing.T) {
	for _, reason := range []string{"source_conflict", "missing_year", "unsupported_datetime", "conditional_deadline", "tentative_schedule", "conflict_with_content", "임의의 검토 사유"} {
		s := imageSchedule()
		s.ReviewReasons = append(s.ReviewReasons, reason)
		p, _ := Project(model.Content{Schedules: []model.Schedule{s}}, 2026, time.Now())
		if p.ImageAcceptedSchedules != 0 || p.ExpiresAt != nil || !p.Schedules[0].ReviewRequired {
			t.Fatal(reason, p)
		}
	}
	s := imageSchedule()
	s.EndDate = nil
	s.EndPartial = &model.CalendarParts{Month: ip(11), Day: ip(5)}
	p, _ := Project(model.Content{Schedules: []model.Schedule{s}}, 2026, time.Now())
	if p.Schedules[0].EndDate != nil || p.ImageAcceptedSchedules != 0 {
		t.Fatal("image year invented")
	}
	s = imageSchedule()
	s.ImageEvidence[0].Confidence = "low"
	p, _ = Project(model.Content{Schedules: []model.Schedule{s}}, 2026, time.Now())
	if p.ImageAcceptedSchedules != 0 {
		t.Fatal("low-confidence image promoted")
	}
}
