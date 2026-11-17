package predictions

import (
	"testing"
	"time"
)

func TestSyntheticSlotsHorizonAndCalendarMonths(t *testing.T) {
	from, _ := ParseDate("2026-11-17")
	slots := Slots(from)
	if len(slots) != 36 || slots[0].Date != "2026-11-17" || slots[35].Date != "2027-02-17" {
		t.Fatal(slots)
	}
	seen := map[string]bool{}
	for _, s := range slots {
		if seen[s.Date] {
			t.Fatal("duplicate date")
		}
		seen[s.Date] = true
	}
	d, _ := ParseDate("2026-01-31")
	if AddMonths(d, 3).Format("2006-01-02") != "2026-04-30" {
		t.Fatal("month end clamp")
	}
}
func TestSyntheticValidationAndProvenance(t *testing.T) {
	from, _ := ParseDate("2026-11-17")
	slots := Slots(from)[:1]
	rows, e := Build("run", "2026-11-17", "2027-02-17", slots, []Item{{SlotID: slots[0].ID, Title: "겨울방학 학습 지원 모집", Category: "scholarship", Summary: "학습 지원 참여자를 모집하는 가상 공고입니다.", Reason: "합성 시나리오로 배치했습니다."}}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if rows[0].DataOrigin != "synthetic" || rows[0].DateOrigin != "simulation_slot" {
		t.Fatal("origin")
	}
	bad := rows[0]
	bad.Date = "2027-02-18"
	if Validate(bad) == nil {
		t.Fatal("outside horizon accepted")
	}
	bad = rows[0]
	bad.Title = "변경"
	if Validate(bad) == nil {
		t.Fatal("payload mutation accepted")
	}
	bad = rows[0]
	bad.DataOrigin = "real"
	if Validate(bad) == nil {
		t.Fatal("real provenance accepted")
	}
	if _, e = Build("run", rows[0].HorizonFrom, rows[0].HorizonTo, slots, []Item{}, time.Now()); e == nil {
		t.Fatal("missing rows accepted")
	}
}
