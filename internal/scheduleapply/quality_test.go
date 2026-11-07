package scheduleapply

import (
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/schedules"
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
	"time"
)

func TestQualityProjectionSemanticRebuildRejectsRehashedEdits(t *testing.T) {
	original := actual(t).Items[0].Original
	original.Title = "신청 안내"
	original.Content = "신청: 2026년 11월 23일까지"
	original.ContentHash = model.ContentDigest(original)
	raw := `{"summary":"신청을 안내합니다.","target_audience":null,"summary_review_required":false,"schedules":[{"type":"application","label":"신청","start_date":null,"start_time":null,"end_date":"2026-11-23","end_time":null,"start_partial":null,"end_partial":null,"timezone":"Asia/Seoul","precision":"date","scope":null,"evidence":"신청: 2026년 11월 23일까지","parent_index":null,"review_required":false,"review_reasons":[]}]}`
	item, e := PrepareQuality(original, raw, "gemini-3.5-flash-lite", bson.M{}, time.Now())
	if e != nil || ValidateItem(item) != nil {
		t.Fatal(e)
	}
	item.Analysis.ValidatedOutput.Schedules[0].Quality.State = "verified"
	item.Analysis.ValidatedOutput.ScheduleResultHash = ProjectionHash(item.Analysis.ValidatedOutput)
	item.Analysis.ID = analysisID(item.Analysis)
	item.Analysis.ValidatedOutput.ScheduleAnalysisID = item.Analysis.ID
	if ValidateItem(item) == nil {
		t.Fatal("rehashed ungrounded quality promotion accepted")
	}
}

func TestFlashAnalysisCannotImpersonateLite(t *testing.T) {
	original := actual(t).Items[0].Original
	original.Title = "신청 안내"
	original.Content = "신청: 2026년 11월 23일까지"
	original.ContentHash = model.ContentDigest(original)
	raw := `{"summary":"신청 안내","target_audience":null,"summary_review_required":false,"schedules":[]}`
	n, e := schedules.RevalidateV2(original.ID.Hex(), raw, schedules.Document{Title: original.Title, Content: original.Content})
	if e != nil {
		t.Fatal(e)
	}
	flash, e := PrepareFlash(original, n, raw, "gemini-3.8-flash", bson.M{}, time.Now())
	if e != nil || ValidateItem(flash) != nil {
		t.Fatal(e)
	}
	if _, e = PrepareFlash(original, n, raw, "gemini-3.5-flash-lite", bson.M{}, time.Now()); e == nil {
		t.Fatal("Lite falsely relabeled")
	}
	if _, e = PrepareV2(original, n, raw, "gemini-3.8-flash", bson.M{}, time.Now()); e == nil {
		t.Fatal("Flash allowed in Lite history format")
	}
}
