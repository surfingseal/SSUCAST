package imageanalysis

import (
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/schedules"
	"go.mongodb.org/mongo-driver/v2/bson"
	"reflect"
	"testing"
	"time"
)

func str(v string) *string { return &v }
func Synthetic() Result {
	v := model.Content{ID: bson.NewObjectID(), SourceID: bson.NewObjectID(), SourceName: "ssu_catch", OriginalURL: "https://scatch.ssu.ac.kr/notice/1", Title: "합성 설명회 신청", Content: "포스터 참고", ContentHash: "synthetic-content", ImageURLs: []string{"https://scatch.ssu.ac.kr/wp-content/uploads/synthetic.png"}, ScheduleMetadata: model.ScheduleMetadata{ScheduleAnalysisID: "text-history", ScheduleInputHash: "placeholder", ScheduleExtractionVersion: "schedule-v2", ScheduleModelVersion: Model}}
	v.ScheduleInputHash = scheduleapply.InputHash(v)
	im := ImageInput{URL: v.ImageURLs[0], Index: 0, SHA256: "synthetic-hash", Location: "stored_inline_image_urls"}
	out := Output{Transcriptions: []Transcription{{Index: 0, Text: "행사일시: 2026년 11월 20일 14:00~16:00", Readable: true}}, Candidates: []ImageCandidate{{V2Schedule: schedules.V2Schedule{Type: "event", Label: "합성 설명회", StartDate: str("2026-11-20"), EndDate: str("2026-11-20"), StartTime: str("14:00"), EndTime: str("16:00"), Timezone: "Asia/Seoul", Precision: "minute", ReviewReasons: []string{}}, Source: "image", ImageIndex: 0, EvidenceText: "행사일시: 2026년 11월 20일 14:00~16:00", Confidence: "high", Notes: []string{}}}, SummarySupport: []string{}, AudienceSupport: []string{}, Reasons: []string{}}
	b, _ := json.Marshal(out)
	return Result{ID: v.ID.Hex(), Original: v, Images: []ImageInput{im}, InputHash: InputHash(Candidate{Original: v, Images: []ImageInput{im}}), Model: Model, Raw: string(b), At: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)}
}
func TestSyntheticImageValidationAndProvenance(t *testing.T) {
	r := Synthetic()
	o, e := Decode(r.Raw)
	if e != nil {
		t.Fatal(e)
	}
	ss, issues := Validate(o, r.Images)
	if len(ss) != 1 || len(issues) != 0 || ss[0].StartDate == nil || *ss[0].StartDate != "2026-11-20" || len(ss[0].ImageEvidence) != 1 || !ss[0].ReviewRequired {
		t.Fatal(ss, issues)
	}
	if ss[0].EvidenceLocation.Matched {
		t.Fatal("OCR falsely treated as text offset")
	}
	p, e := Prepare(r)
	if e != nil {
		t.Fatal(e)
	}
	if e = ValidatePrepared(p); e != nil {
		t.Fatal(e)
	}
	p.Analysis.ValidatedOutput.Schedules[0].Label = "tampered"
	if ValidatePrepared(p) == nil {
		t.Fatal("tamper accepted")
	}
}
func TestMissingImageEvidenceUnknownYearAndEmpty(t *testing.T) {
	r := Synthetic()
	o, _ := Decode(r.Raw)
	o.Candidates[0].EvidenceText = "invented date 2099"
	if ss, _ := Validate(o, r.Images); len(ss) != 0 {
		t.Fatal("invented quote")
	}
	o, _ = Decode(r.Raw)
	o.Candidates[0].ImageIndex = 99
	if ss, _ := Validate(o, r.Images); len(ss) != 0 {
		t.Fatal("unknown image")
	}
	o, _ = Decode(r.Raw)
	o.Transcriptions[0].Text = "행사: 11월 20일"
	o.Candidates[0].EvidenceText = o.Transcriptions[0].Text
	o.Candidates[0].StartDate = str("2026-11-20")
	o.Candidates[0].EndDate = nil
	o.Candidates[0].StartTime = nil
	o.Candidates[0].EndTime = nil
	ss, _ := Validate(o, r.Images)
	if len(ss) != 1 || ss[0].StartDate != nil || ss[0].StartPartial == nil || ss[0].StartPartial.Year != nil {
		t.Fatal("hallucinated year retained", ss)
	}
	o.Candidates = []ImageCandidate{}
	if ss, _ := Validate(o, r.Images); len(ss) != 0 {
		t.Fatal("empty images generated schedules")
	}
	if _, e := Decode(`{"image_schedule_candidates":[]}`); e == nil {
		t.Fatal("missing schema")
	}
}
func TestMergeDedupComplementConflictAndScopes(t *testing.T) {
	r := Synthetic()
	o, _ := Decode(r.Raw)
	ss, _ := Validate(o, r.Images)
	a := ss[0]
	a.ID = "text/1"
	a.Origin = "text_ai"
	a.ImageEvidence = nil
	a.ReviewRequired = false
	a.ReviewReasons = []string{}
	a.Quality = &model.ScheduleQuality{State: "verified"}
	original := a
	m := Merge([]model.Schedule{a}, ss)
	if m.Matched != 1 || m.Added != 0 || len(m.Schedules) != 1 || m.Schedules[0].Origin != "merged_text_image" {
		t.Fatal(m)
	}
	if !reflect.DeepEqual(a, original) {
		t.Fatal("input mutated")
	}
	a.StartTime = nil
	m = Merge([]model.Schedule{a}, ss)
	if m.Complemented != 1 || m.Schedules[0].StartTime == nil {
		t.Fatal(m)
	}
	a = original
	a.EndDate = str("2026-11-21")
	m = Merge([]model.Schedule{a}, ss)
	if m.Conflicts != 1 || len(m.Schedules) != 2 || !m.Schedules[0].ReviewRequired {
		t.Fatal(m)
	}
	a = original
	a.Scope = str("기관A")
	b := ss[0]
	b.Scope = str("기관B")
	m = Merge([]model.Schedule{a}, []model.Schedule{b})
	if m.Added != 1 || len(m.Schedules) != 2 {
		t.Fatal("institutions merged", m)
	}
}
func TestTriggerAndURLRules(t *testing.T) {
	r := Synthetic()
	v := r.Original
	if len(Reasons(v)) == 0 {
		t.Fatal("missing trigger")
	}
	for _, u := range []string{"http://scatch.ssu.ac.kr/a.png", "https://attacker.example/poster.png", "https://scatch.ssu.ac.kr/tracker?id=1", "https://scatch.ssu.ac.kr/wp-content/uploads/logo.png"} {
		if AllowedImage(v, u) {
			t.Fatal(u)
		}
	}
	valid := model.Schedule{StartDate: str("2026-11-20"), EndDate: str("2026-11-20"), Quality: &model.ScheduleQuality{State: "partial"}}
	a, b := valid, valid
	a.Type = "event"
	b.Type = "application"
	v.Schedules = []model.Schedule{a, b}
	if len(Reasons(v)) != 0 {
		t.Fatal("redundant image analysis")
	}
}

func TestPartialConflictAndMergedReviewReasons(t *testing.T) {
	month, day1, day2 := 11, 20, 21
	a := model.Schedule{ID: "text", Type: "application", Label: "접수", EndPartial: &model.CalendarParts{Month: &month, Day: &day1}}
	b := a
	b.ID = "image"
	b.EndPartial = &model.CalendarParts{Month: &month, Day: &day2}
	b.Origin = "image_ai"
	if m := Merge([]model.Schedule{a}, []model.Schedule{b}); m.Conflicts != 1 || len(m.Schedules) != 2 {
		t.Fatal("different partial dates merged", m)
	}
	b.EndPartial = a.EndPartial
	b.ReviewReasons = []string{"conditional_deadline"}
	b.Qualifiers = []string{"조기 마감"}
	m := Merge([]model.Schedule{a}, []model.Schedule{b})
	if !reflect.DeepEqual(m.Schedules[0].Qualifiers, b.Qualifiers) {
		t.Fatal("conditional qualifier lost")
	}
	found := false
	for _, r := range m.Schedules[0].ReviewReasons {
		if r == "conditional_deadline" {
			found = true
		}
	}
	if !found {
		t.Fatal("review reason lost")
	}
}

func TestSameAudienceDistinctEventsAreNotMerged(t *testing.T) {
	a := model.Schedule{ID: "event-one", Type: "event", Label: "성적 이의신청", Scope: str("학생")}
	b := a
	b.ID = "event-two"
	b.Label = "성적확정일"
	m := Merge(nil, []model.Schedule{a, b})
	if m.Matched != 0 || m.Conflicts != 0 || len(m.Schedules) != 2 {
		t.Fatal("audience mistaken for event identity", m)
	}
}
