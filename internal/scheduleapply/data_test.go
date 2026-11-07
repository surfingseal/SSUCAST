package scheduleapply

import (
 "example.com/crawlserver/internal/testutil/localfixture"
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/schedules"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"strings"
	"testing"
	"time"
)

func actual(t *testing.T) Bundle {
	t.Helper()
	localfixture.Require(t, "testdata/actual10.json")
	raw, e := os.ReadFile("testdata/actual10.json")
	if e != nil {
		t.Fatal(e)
	}
	var b Bundle
	if e = json.Unmarshal(raw, &b); e != nil {
		t.Fatal(e)
	}
	return b
}
func TestActualApplyBundleGroundingAndSeparation(t *testing.T) {
	b := actual(t)
	if e := ValidateBundle(b); e != nil {
		t.Fatal(e)
	}
	for _, v := range b.Items {
		if v.Analysis.RawResponse == "" || v.Analysis.ValidatedOutput.Summary == "" || v.Analysis.ValidatedOutput.ScheduleAnalyzedAt == nil {
			t.Fatal("analysis provenance lost")
		}
		if v.Original.ContentHash == "" || v.Analysis.InputHash != InputHash(v.Original) {
			t.Fatal("input not bound")
		}
	}
}
func TestTamperedBundleRejected(t *testing.T) {
	b := actual(t)
	b.Items[0].Analysis.ValidatedOutput.Schedules[0].EndTime = nil
	if ValidateBundle(b) == nil {
		t.Fatal("tampered projection accepted")
	}
	b = actual(t)
	b.Items[0].Analysis.RawResponse = "{}"
	if ValidateBundle(b) == nil {
		t.Fatal("raw response tampering accepted")
	}
	b = actual(t)
	b.Items[0].Original.Content += "변경"
	if ValidateBundle(b) == nil {
		t.Fatal("changed source accepted")
	}
	b = actual(t)
	b.Items = append(b.Items, b.Items[0])
	if ValidateBundle(b) == nil {
		t.Fatal("wrong notice count accepted")
	}
}

func TestBoundedBatchDoesNotRelaxLegacyTen(t *testing.T) {
	original := actual(t).Items[0].Original
	original.ID = bson.NewObjectID()
	original.Title = "신청 안내"
	original.Content = "신청: 2026년 11월 23일까지"
	original.Status = "unknown"
	original.ContentHash = model.ContentDigest(original)
	raw := `{"summary":"신청을 안내합니다.","target_audience":null,"summary_review_required":false,"schedules":[{"type":"application","label":"신청","start_date":null,"start_time":null,"end_date":"2026-11-23","end_time":null,"start_partial":null,"end_partial":null,"timezone":"Asia/Seoul","precision":"date","scope":null,"evidence":"신청: 2026년 11월 23일까지","parent_index":null,"review_required":false,"review_reasons":[]}]}`
	n, e := schedules.RevalidateV2(original.ID.Hex(), raw, schedules.Document{Title: original.Title, Content: original.Content})
	if e != nil {
		t.Fatal(e)
	}
	item, e := PrepareV2(original, n, raw, "gemini-3.5-flash-lite", bson.M{}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	selected := []string{original.ID.Hex()}
	for i := 1; i < 76; i++ {
		selected = append(selected, bson.NewObjectID().Hex())
	}
	excluded := []string{}
	for i := 0; i < 10; i++ {
		excluded = append(excluded, bson.NewObjectID().Hex())
	}
	scope := &BatchScope{Database: "ssu_crawler", SourceName: "ssu_catch", SnapshotHash: strings.Repeat("0", 64), SelectedIDs: selected, ExcludedIDs: excluded, SuccessfulIDs: []string{original.ID.Hex()}, ExpectedItems: 1}
	scope.SelectionHash = digest([]any{"remaining-76-v1", scope.Database, "gemini-3.5-flash-lite", scope.SnapshotHash, selected, excluded})
	b := Bundle{Version: BatchBundleVersion, Batch: scope, Items: []Item{item}}
	if e = ValidateBundle(b); e != nil {
		t.Fatal(e)
	}
	b.Version = BundleVersion
	if ValidateBundle(b) == nil {
		t.Fatal("legacy ten-notice gate relaxed")
	}
	b.Version = BatchBundleVersion
	b.Batch.ExcludedIDs[0] = selected[0]
	if ValidateBundle(b) == nil {
		t.Fatal("excluded ID accepted into batch")
	}
}
