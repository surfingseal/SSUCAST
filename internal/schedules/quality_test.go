package schedules

import (
 "example.com/crawlserver/internal/testutil/localfixture"
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"strings"
	"testing"
)

func TestQualityActual76Retains186AndGroundedAdditions(t *testing.T) {
	localfixture.Require(t, "testdata/actual76.json")
	raw, e := os.ReadFile("testdata/actual76.json")
	if e != nil {
		t.Fatal(e)
	}
	var corpus struct {
		Items []struct {
			ID string `json:"id"`
			Document
			Raw string `json:"raw_response"`
		} `json:"items"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("fixture")
	}
	seen := map[string]bool{}
	count, added, zero, spans := 0, 0, 0, 0
	states := map[string]int{}
	categories := map[string]int{}
	for _, x := range corpus.Items {
		if seen[x.ID] {
			t.Fatal("duplicate")
		}
		seen[x.ID] = true
		old, e := RevalidateV2(x.ID, x.Raw, x.Document)
		if e != nil {
			t.Fatal(e)
		}
		q, e := RevalidateQuality(x.ID, x.Raw, x.Document)
		if e != nil {
			t.Fatal(e)
		}
		if len(old.Schedules) != len(q.Notice.Schedules) || old.Summary != q.Notice.Summary {
			t.Fatal("lost schedules or modified raw summary")
		}
		if old.Audience.ReviewRequired && !q.Notice.Audience.ReviewRequired {
			t.Fatal("unjustified audience promotion")
		}
		count += len(q.Notice.Schedules)
		added += len(q.Notice.RuleSupplements)
		if q.Zero != nil {
			zero++
			categories[q.Zero.Category]++
		}
		all := append(append([]model.Schedule{}, q.Notice.Schedules...), q.Notice.RuleSupplements...)
		if len(ValidateSet(all)) != 0 {
			t.Fatal("invalid schedule graph")
		}
		for i, s := range all {
			if !ValidSpans(s.EvidenceSpans, x.Document) {
				t.Fatal("ungrounded spans", s.ID)
			}
			spans++
			if i < len(old.Schedules) && s.ID != old.Schedules[i].ID {
				t.Fatal("changed ID")
			}
			states[s.Quality.State]++
			if (s.Quality.State == "uncertain" || s.Quality.State == "blocked") && (s.Quality.DisplayAllowed || s.Quality.DeadlineCandidate) {
				t.Fatal("unsafe policy")
			}
			if s.Quality.DeadlineCandidate && (s.EndDate == nil || s.EndTime == nil) {
				t.Fatal("clock invented")
			}
			data, e := bson.Marshal(s)
			if e != nil {
				t.Fatal(e)
			}
			var back model.Schedule
			if bson.Unmarshal(data, &back) != nil {
				t.Fatal("bson")
			}
			before, _ := json.Marshal(s)
			after, _ := json.Marshal(back)
			if string(before) != string(after) {
				t.Fatal("BSON precision/spans/quality loss")
			}
			if strings.Contains(s.Label, "Session ") && strings.Contains(x.Title, "SIC 4쿼터") {
				if s.StartDate == nil || *s.StartDate != "2026-11-18" || s.StartTime != nil || s.EndTime != nil || s.ParentID == nil || len(s.EvidenceSpans) != 3 || s.Quality.State != "uncertain" {
					t.Fatal("parent clock inherited/uncertainty removed")
				}
			}
		}
	}
	if len(seen) != 76 || count != 186 || added != 2 || zero != 11 || spans != 188 {
		t.Fatalf("%d %d %d %d %d", len(seen), count, added, zero, spans)
	}
	if states["blocked"] != 7 || states["uncertain"] != 104 || states["partial"] != 64 || states["verified"] != 13 {
		t.Fatal(states)
	}
	if categories["A"] != 2 || categories["B"] != 2 || categories["C"] != 1 || categories["D"] != 2 || categories["E"] != 4 {
		t.Fatal(categories)
	}
}
func TestQualitySpansNeverAcceptSyntheticQuotation(t *testing.T) {
	doc := Document{Content: "일시 2026.11.18.\n다른 문장\nSession 1. 안내"}
	if MatchEvidence("", doc.Content, "2026.11.18. Session 1. 안내").Matched {
		t.Fatal("joined quote accepted")
	}
	ss := []model.SourceSpan{span(doc, "date", "2026.11.18."), span(doc, "session", "Session 1. 안내")}
	if !ValidSpans(ss, doc) {
		t.Fatal("separate spans rejected")
	}
	ss[0].Location.ByteStart++
	if ValidSpans(ss, doc) {
		t.Fatal("forged offset accepted")
	}
}
func TestQualityDateOnlyPolicyAndMissingYear(t *testing.T) {
	s := model.Schedule{Type: "application", EndDate: sp("2026-11-18"), ReviewReasons: []string{}}
	q := DeriveQuality(s, nil)
	if q.State != "partial" || !q.DisplayAllowed || q.DeadlineCandidate {
		t.Fatal(q)
	}
	s.EndTime = sp("18:00")
	q = DeriveQuality(s, nil)
	if !q.DeadlineCandidate {
		t.Fatal(q)
	}
	s.ReviewReasons = []string{"missing_year"}
	q = DeriveQuality(s, nil)
	if q.State != "uncertain" || q.DeadlineCandidate {
		t.Fatal(q)
	}
	q = DeriveQuality(s, []string{"evidence_mismatch"})
	if q.State != "blocked" {
		t.Fatal(q)
	}
}
func TestQualitySemanticAndCalendarConflicts(t *testing.T) {
	if !weekdayConflict("일시: 2027.2.3.(화)") || weekdayConflict("일시: 2027.2.3.(수)") || weekdayConflict("2027학년도 모집: 2.3.(화)") {
		t.Fatal("calendar/academic-year distinction")
	}
	w := baseV2("운영 기간: ~2026.12.31.(목)")
	w.Label = "프로그램 운영 기간"
	w.EndDate = sp("2026-12-31")
	q, e := RevalidateQuality("x", rawV2(t, w), Document{Content: w.Evidence})
	if e != nil || q.Notice.Schedules[0].Type != "event" || q.Notice.Schedules[0].Quality.DeadlineCandidate {
		t.Fatal(q, e)
	}
}
func TestQualityZeroUnknownAndConditionalNoFakeDate(t *testing.T) {
	for _, d := range []Document{{Title: "2026학년도 겨울계절제 국내대학 학점교류 신청 안내", Content: "국내대학 학점교류 신청 게시판 바로가기"}, {Title: "전공설계상담 안내", Content: "학기 중: 09:00 ~ 17:30"}, {Title: "특별장학금 신청 안내", Content: "붙임 참조\n2026.10.27"}} {
		q, e := RevalidateQuality("x", rawV2(t), d)
		if e != nil || len(q.Notice.Schedules) > 0 || len(q.Notice.RuleSupplements) > 0 || q.Zero == nil {
			t.Fatal(q, e)
		}
	}
}
