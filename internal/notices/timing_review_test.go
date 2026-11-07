package notices

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTimingReviewFingerprintAndNoDateMutation(t *testing.T) {
	id := "000000000000000000000001"
	input := []TimingRecord{{ID: id, ContentHash: "source-hash", AnalysisID: "analysis-one", Review: TimingReview{Status: "relative_or_recurring", Label: "상대 기한", Message: "개인별 결석일 확인 필요", ReviewRequired: true, Rules: []string{"결석일로부터10일 이내"}, Evidence: []TimingEvidence{{Source: "image", Text: "합성 근거", URL: "javascript:alert(1)"}}}}}
	raw, _ := json.Marshal(input)
	file := filepath.Join(t.TempDir(), "review.json")
	os.WriteFile(file, raw, 0600)
	records, e := LoadTimingReviews(file)
	if e != nil {
		t.Fatal(e)
	}
	n := Notice{ID: id, reviewContentHash: "source-hash", reviewAnalysisID: "analysis-one", Schedules: []Schedule{}}
	records.Apply(&n)
	if n.TimingReview == nil || len(n.Schedules) != 0 || n.TimingReview.Evidence[0].URL != "" {
		t.Fatal("review made dates or unsafe links", n)
	}
	n.TimingReview.Rules[0] = "changed by caller"
	if records[id].Review.Rules[0] == "changed by caller" {
		t.Fatal("shared review mutated")
	}
	b, _ := json.Marshal(n)
	if strings.Contains(string(b), "source-hash") || strings.Contains(string(b), "analysis-one") {
		t.Fatal("private fingerprint leaked")
	}
	n.TimingReview = nil
	n.reviewContentHash = "changed"
	records.Apply(&n)
	if n.TimingReview != nil {
		t.Fatal("stale source review displayed")
	}
	n.reviewContentHash = "source-hash"
	n.reviewAnalysisID = "new-analysis"
	records.Apply(&n)
	if n.TimingReview != nil {
		t.Fatal("stale analysis review displayed")
	}
	input = append(input, input[0])
	raw, _ = json.Marshal(input)
	os.WriteFile(file, raw, 0600)
	if _, e = LoadTimingReviews(file); e == nil {
		t.Fatal("duplicate IDs accepted")
	}
}
