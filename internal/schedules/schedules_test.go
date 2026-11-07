package schedules

import (
 "example.com/crawlserver/internal/testutil/localfixture"
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// All cases in this file are synthetic unit fixtures, explicitly separate from
// the saved actual school/Vertex corpus in actual_test.go and testdata/actual10.
func convert(ev string, typ string) model.Schedule {
	return FromLegacy("synthetic", 0, LegacySchedule{Type: typ, Label: "합성 단위 테스트", Evidence: ev}, Document{Content: ev})
}
func TestDateOnlyPreservedWithoutClockOrReview(t *testing.T) {
	s := convert("모집기간 : 2026년 11월 23일 월요일까지", "application")
	if value(s.EndDate) != "2026-11-23" || s.EndTime != nil || s.EndAt != nil || s.ReviewRequired || s.Precision != "date" {
		t.Fatalf("%+v", s)
	}
	if !strings.Contains(strings.Join(s.Omissions, ","), "missing_time") {
		t.Fatal(s)
	}
}
func TestExplicitClocksAndTimestampConsistency(t *testing.T) {
	ev := "행사: 2026.11.12.(목) 12:00~13:00"
	old := LegacySchedule{Type: "event", Evidence: ev, StartAt: sp("2026-11-12T12:00:00+09:00"), EndAt: sp("2026-11-12T13:00:00+09:00")}
	doc := Document{Content: ev}
	s := FromLegacy("synthetic", 0, old, doc)
	if value(s.StartTime) != "12:00" || value(s.EndTime) != "13:00" || len(s.Corrections) > 0 || s.ReviewRequired || !sameTimestamp(s.StartAt, old.StartAt) {
		t.Fatal(s)
	}
	s.StartAt = sp("2026-11-12T11:00:00+09:00")
	if ValidateSchedule(&s, doc) || !s.ReviewRequired {
		t.Fatal("conflicting compatibility timestamp accepted")
	}
	s = convert("신청: 2026.11.12. 09:00~18:00", "application")
	if value(s.StartTime) != "09:00" || value(s.EndTime) != "18:00" {
		t.Fatal(s)
	}
}
func TestInventedClockRemovedDateKept(t *testing.T) {
	ev := "모집: 2026년 11월 23일까지"
	s := FromLegacy("synthetic", 0, LegacySchedule{Type: "application", Evidence: ev, EndAt: sp("2026-11-23T23:59:59+09:00"), ReviewRequired: true, ReviewReason: sp("시각 미기재")}, Document{Content: ev})
	if value(s.EndDate) != "2026-11-23" || s.EndTime != nil || s.ReviewRequired || len(s.Corrections) != 1 {
		t.Fatal(s)
	}
}
func TestPartialCalendarPrecision(t *testing.T) {
	for _, tc := range []struct {
		ev, precision    string
		year, month, day int
		review           bool
	}{
		{"발표: 2026년 12월", "month", 2026, 12, 0, false},
		{"시행: 2026년", "year", 2026, 0, 0, false},
		{"접수: 10/28(수) ~ 11/4(수)", "month_day", 0, 10, 28, true},
	} {
		t.Run(tc.precision, func(t *testing.T) {
			s := convert(tc.ev, "announcement")
			if s.Precision != tc.precision || s.StartDate != nil || s.StartPartial == nil || s.ReviewRequired != tc.review {
				t.Fatal(s)
			}
			p := s.StartPartial
			if tc.year == 0 && p.Year != nil || tc.year != 0 && (p.Year == nil || *p.Year != tc.year) {
				t.Fatal(p)
			}
			if tc.month != 0 && (p.Month == nil || *p.Month != tc.month) {
				t.Fatal(p)
			}
			if tc.day != 0 && (p.Day == nil || *p.Day != tc.day) {
				t.Fatal(p)
			}
		})
	}
}
func TestExplicitRangeYearAndUncertainYearRollover(t *testing.T) {
	s := convert("근무: 2027.1.4.(월) ~ 2.12.(금)", "event")
	if value(s.EndDate) != "2027-02-12" || s.ReviewRequired || s.DateContext != "explicit_range_year" {
		t.Fatal(s)
	}
	s = convert("행사: 2026.12.31. ~ 1.2.", "event")
	if s.EndDate != nil || s.EndPartial == nil || s.EndPartial.Year != nil || !s.ReviewRequired {
		t.Fatal("unconfirmed rollover year invented", s)
	}
}
func TestWhitespaceEvidenceOffsetsAndMeaningfulMismatch(t *testing.T) {
	body := "앞말\n면접일정\n2026. 11. 11.(수) 14:00 예정\n끝"
	ev := "  면접일정   2026. 11. 11.(수) 14:00 예정  "
	m := MatchEvidence("", body, ev)
	if !m.Matched || m.MatchType != "whitespace_normalized" || body[m.ByteStart:m.ByteEnd] != m.OriginalText || utf8.RuneCountInString(body[:m.ByteStart]) != m.RuneStart {
		t.Fatal(m)
	}
	for _, changed := range []string{"면접일정 2026. 11. 12.(수) 14:00 예정", "면접일정 2026. 11. 11.(수) 15:00 예정", "면접일정 2026. 11. 11.(수) 14:00 확정"} {
		if MatchEvidence("", body, changed).Matched {
			t.Fatal("meaning changed but accepted")
		}
	}
	for _, tc := range []struct{ source, evidence string }{{"기관 A 장학금 400만원, 2학기 이상", "기관 B 장학금 400만원, 2학기 이상"}, {"기관 A 장학금 400만원, 2학기 이상", "기관 A 장학금 500만원, 2학기 이상"}, {"기관 A 장학금 400만원, 2학기 이상", "기관 A 장학금 400만원, 1학기 이상"}, {"A B", "AB"}} {
		if MatchEvidence("", tc.source, tc.evidence).Matched {
			t.Fatal(tc)
		}
	}
}
func TestEvidenceMismatchNeverGeneratesDate(t *testing.T) {
	s := FromLegacy("synthetic", 0, LegacySchedule{Type: "application", Evidence: "2026.11.30. 18:00", EndAt: sp("2026-11-30T18:00:00+09:00")}, Document{Content: "마감 2026.11.20. 18:00"})
	if !s.ReviewRequired || s.EndDate != nil || !strings.Contains(strings.Join(s.ReviewReasons, ","), "evidence_mismatch") {
		t.Fatal(s)
	}
}
func TestTentativeAndConditionalSeparateFromMissingClock(t *testing.T) {
	s := convert("발표: 2026년 12월 예정", "announcement")
	if !s.ReviewRequired || strings.Join(s.ReviewReasons, ",") != "tentative_schedule" {
		t.Fatal(s)
	}
	s = convert("발표: 2026.12.16.(수) 이후", "announcement")
	if !strings.Contains(strings.Join(s.ReviewReasons, ","), "conditional_deadline") {
		t.Fatal(s)
	}
}
func TestInstitutionsRoundsAndHierarchyNotCollapsed(t *testing.T) {
	a := convert("모집: 2026.11.23.까지", "application")
	a.ID = "a"
	a.Scope = sp("기관 A / 1차")
	b := a
	b.ID = "b"
	b.Scope = sp("기관 B / 1차")
	c := a
	c.ID = "c"
	c.Scope = sp("기관 A / 2차")
	if len(ValidateSet([]model.Schedule{a, b, c})) != 0 {
		t.Fatal("distinct scopes deduplicated")
	}
	d := a
	d.ID = "d"
	if len(ValidateSet([]model.Schedule{a, d})) != 1 {
		t.Fatal("true duplicate missed")
	}
	d = a
	d.ID = "child"
	d.ParentID = sp(a.ID)
	if len(ValidateSet([]model.Schedule{a, d})) != 0 {
		t.Fatal("parent/child merged")
	}
}
func TestRuleSessionsGroundedAndNoRevalidationFabrication(t *testing.T) {
	body := "행사: 2026년 11월 25일 07:00~17:00\n* 4타임으로 구분하여 신청\n1) 07~10시\n2) 10~12시\n3) 13~15시\n4) 15~17시\n봉사내용"
	doc := Document{Content: body}
	ev := strings.Split(body, "\n")[0]
	parent := FromLegacy("synthetic", 0, LegacySchedule{Type: "event", Evidence: ev}, doc)
	if parent.ReviewRequired {
		t.Fatal(parent)
	}
	children := SupplementSessions([]model.Schedule{parent}, doc)
	if len(children) != 4 {
		t.Fatal(children)
	}
	for _, c := range children {
		if c.ExtractionMethod != "rule" || c.Origin != "source_rule_supplement" || c.ParentID == nil || *c.ParentID != parent.ID || !ValidateChild(c, []model.Schedule{parent}, doc) {
			t.Fatal(c)
		}
	}
	// With no source list, conversion of the AI's overall event stays one event.
	if len(SupplementSessions([]model.Schedule{parent}, Document{Content: ev})) != 0 {
		t.Fatal("missing sessions fabricated")
	}
	forged := children[0]
	forged.StartDate = sp("2026-11-26")
	if ValidateChild(forged, []model.Schedule{parent}, doc) {
		t.Fatal("forged inherited date accepted")
	}
	existing := children[0]
	if len(SupplementSessions([]model.Schedule{parent, existing}, doc)) != 3 {
		t.Fatal("existing child duplicated")
	}
}
func TestAudienceGeneralizationPreferencesAndSummaryPreservation(t *testing.T) {
	body := "재단은 청소년을 돕기 위해 설립되었습니다.\n지원대상: 2~7학기 재학생\n지원자격\n가. 소득분위 9분위 이내\n나. 직전 학기 12학점 이상\n다. 백분위 70점 이상\n2. 선발기준\n경영학과 학생 우대\n"
	old := LegacyOutput{Summary: "장학금 신청을 안내합니다.", TargetAudience: sp("청소년")}
	a := ValidateAudience(old, Document{Content: body})
	if !a.ReviewRequired || a.TargetAudience != nil || a.Summary != old.Summary || len(a.Preferences) != 1 || len(a.Mandatory) < 4 {
		t.Fatal(a)
	}
	for _, c := range a.Mandatory {
		if strings.Contains(c.Text, "우대") {
			t.Fatal("preference became mandatory")
		}
	}
	old.TargetAudience = sp("2~7학기 재학생")
	a = ValidateAudience(old, Document{Content: "지원대상: 2~7학기 재학생\n경영학과 학생 우대"})
	if a.ReviewRequired || a.TargetAudience == nil {
		t.Fatal(a)
	}
	old.TargetAudience = nil
	a = ValidateAudience(old, Document{})
	if a.TargetAudience != nil {
		t.Fatal(a)
	}
	old.Summary = strings.Repeat("가", 101)
	a = ValidateAudience(old, Document{})
	if !a.SummaryReviewRequired || a.SummaryCharacters != 101 {
		t.Fatal(a)
	}
	old.Summary = "장학금 999만원 지급"
	a = ValidateAudience(old, Document{Content: "장학금 400만원 지급"})
	if len(a.UnsupportedBenefitNumbers) != 1 {
		t.Fatal(a)
	}
}
func TestBSONMigrationCompatibilityAndLegacyFields(t *testing.T) {
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	id := bson.NewObjectID()
	old := model.Content{ID: id, SourceID: bson.NewObjectID(), Title: "보존", Content: "본문", ContentHash: "unchanged", OriginalURL: "https://example.test/1", PublishedAt: &now, Status: "진행", Category: "academic", ClassificationStatus: "source", Deadline: &now, LifecycleStatus: "active"}
	before, _ := bson.Marshal(old)
	var legacy model.Content
	if e := bson.Unmarshal(before, &legacy); e != nil || len(legacy.Schedules) != 0 {
		t.Fatal(e)
	}
	hash := model.ContentDigest(old)
	old.Schedules = []model.Schedule{convert("모집: 2026년 11월 23일까지", "application"), convert("발표: 2026년 12월", "announcement")}
	after, e := bson.Marshal(old)
	if e != nil {
		t.Fatal(e)
	}
	var restored model.Content
	if e = bson.Unmarshal(after, &restored); e != nil {
		t.Fatal(e)
	}
	if value(restored.Schedules[0].EndDate) != "2026-11-23" || restored.Schedules[0].EndTime != nil || restored.Schedules[0].Timezone != "Asia/Seoul" || restored.Schedules[1].StartPartial.Day != nil {
		t.Fatal(restored.Schedules)
	}
	restored.Schedules = nil
	roundtrip, _ := bson.Marshal(restored)
	if string(roundtrip) != string(before) {
		t.Fatal("legacy fields changed during BSON roundtrip")
	}
	if model.ContentDigest(old) != hash {
		t.Fatal("derived schedules changed content hash")
	}
	var doc bson.M
	bson.Unmarshal(after, &doc)
	rows := doc["schedules"].(bson.A)
	if _, ok := rows[0].(bson.D); !ok {
		t.Fatal("unexpected BSON structure")
	}
}
func TestFutureV2ValidationUsesSameModel(t *testing.T) {
	s := model.Schedule{Type: "application", EndDate: sp("2026-11-23"), Timezone: "Asia/Seoul", Evidence: "募集"}
	doc := Document{Content: "모집: 2026년 11월 23일까지"}
	s.Evidence = doc.Content
	s = FromV2(s, doc)
	if s.ReviewRequired || s.EndAt != nil || s.EndTime != nil || s.Precision != "date" {
		t.Fatal(s)
	}
	s.EndTime = sp("23:59")
	s = FromV2(s, doc)
	if !s.ReviewRequired {
		t.Fatal("future invented clock accepted")
	}
}

// Prevent accidental network-enabled environment requirements in offline tests.
func TestFixturesClearlyDistinguishActualFromSynthetic(t *testing.T) {
	localfixture.Require(t, "testdata/actual10.json")
	raw, e := os.ReadFile("testdata/actual10.json")
	if e != nil {
		t.Fatal(e)
	}
	var v struct {
		Provenance struct {
			Kind string `json:"kind"`
		} `json:"provenance"`
	}
	if json.Unmarshal(raw, &v) != nil || !strings.Contains(v.Provenance.Kind, "actual_saved") {
		t.Fatal("actual provenance missing")
	}
}
func TestNumericSuffixIsNotEvidence(t *testing.T) {
	for _, tc := range []struct{ source, evidence string }{{"지원금 1400만원", "400만원"}, {"날짜 11/12", "1/12"}, {"제출 2026.11.23.", "2026.11.2"}} {
		if MatchEvidence("", tc.source, tc.evidence).Matched {
			t.Fatal(tc)
		}
	}
}
func TestInvalidCalendarDoesNotBecomeConfirmed(t *testing.T) {
	for _, ev := range []string{"마감: 2026.02.30. 18:00", "마감: 2026년 13월 1일", "마감: 2026.11.23. 25:00"} {
		s := convert(ev, "application")
		if !s.ReviewRequired {
			t.Fatal("invalid explicit timestamp accepted", s)
		}
	}
}
func TestUnknownLegacyReviewIsNotSilentlyCleared(t *testing.T) {
	ev := "신청: 2026.11.23. 18:00까지"
	s := FromLegacy("synthetic", 0, LegacySchedule{Type: "application", Evidence: ev, ReviewRequired: true}, Document{Content: ev})
	if !s.ReviewRequired || !strings.Contains(strings.Join(s.ReviewReasons, ","), "legacy_review_unresolved") {
		t.Fatal(s)
	}
}
func TestMixedPreferenceClauseNeverBecomesMandatory(t *testing.T) {
	a := ValidateAudience(LegacyOutput{TargetAudience: sp("재학생")}, Document{Content: "지원대상: 재학생(경영학과 학생 우대)"})
	if !a.ReviewRequired || a.TargetAudience != nil || len(a.Preferences) != 1 {
		t.Fatal(a)
	}
	for _, c := range a.Mandatory {
		if strings.Contains(c.Text, "우대") {
			t.Fatal("mixed preference became mandatory")
		}
	}
}
