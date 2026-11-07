package mvp

import (
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/schedulebatch"
	"example.com/crawlserver/internal/testutil/localfixture"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"testing"
	"time"
	"unicode/utf8"
)

func TestActualFlash76ProductionReuse(t *testing.T) {
	localfixture.Require(t, "testdata/flash76.json")
	b, e := os.ReadFile("testdata/flash76.json")
	if e != nil {
		t.Fatal(e)
	}
	var rows []struct {
		Original   model.Content            `json:"original"`
		Checkpoint schedulebatch.Checkpoint `json:"checkpoint"`
	}
	if json.Unmarshal(b, &rows) != nil {
		t.Fatal("fixture")
	}
	seen := map[string]bool{}
	shortened := 0
	count := 0
	for _, r := range rows {
		c := r.Checkpoint
		if seen[c.ID] || c.ModelVersion != AIModel || c.InputHash != scheduleapply.InputHash(r.Original) {
			t.Fatal("identity")
		}
		seen[c.ID] = true
		usage := bson.M{}
		raw, _ := json.Marshal(c.Usage)
		json.Unmarshal(raw, &usage)
		item, e := scheduleapply.PrepareProduction(r.Original, c.RawResponse, c.ModelVersion, usage, c.CompletedAt)
		if e != nil {
			t.Fatal(c.ID, e)
		}
		if e = scheduleapply.ValidateItem(item); e != nil {
			t.Fatal(e)
		}
		if item.Analysis.RawResponse != c.RawResponse || utf8.RuneCountInString(item.Analysis.ValidatedOutput.Summary) > 100 {
			t.Fatal("raw altered or long summary")
		}
		var original struct {
			Summary string `json:"summary"`
		}
		json.Unmarshal([]byte(c.RawResponse), &original)
		if original.Summary != item.Analysis.ValidatedOutput.Summary {
			shortened++
			if !item.Analysis.ValidatedOutput.SummaryReviewRequired {
				t.Fatal("unreviewed derived summary")
			}
		}
		count += len(item.Analysis.ValidatedOutput.Schedules)
	}
	if len(seen) != 76 || shortened != 2 || count < 187 {
		t.Fatal(len(seen), shortened, count)
	}
}
func TestManualBudgetBoundaries(t *testing.T) {
	if _, e := schedulebatch.LoadBoundedBudget(t.TempDir(), []string{"one"}, AIModel, 121); e == nil {
		t.Fatal("cap relaxed")
	}
	if _, e := schedulebatch.LoadBudget(t.TempDir(), []string{"one"}); e == nil {
		t.Fatal("legacy 76 guard relaxed")
	}
	b, e := schedulebatch.LoadBoundedBudget(t.TempDir(), []string{"one"}, AIModel, 120)
	if e != nil || b.ExpectedModel != AIModel || b.MaxRequests != 120 {
		t.Fatal(e)
	}
}

func TestActualCombinedMedia14(t *testing.T) {
	var f struct {
		Source model.Source `json:"source"`
		Rows   []struct {
			Original   model.Content            `json:"original"`
			Checkpoint schedulebatch.Checkpoint `json:"checkpoint"`
		} `json:"rows"`
	}
	localfixture.Require(t, "testdata/media14.json")
	if e := Read("testdata/media14.json", &f); e != nil {
		t.Fatal(e)
	}
	if len(f.Rows) != 14 || f.Source.Enabled {
		t.Fatal("capture/source")
	}
	for _, r := range f.Rows {
		c := r.Checkpoint
		if c.ModelVersion != AIModel || c.InputHash != scheduleapply.InputHash(r.Original) {
			t.Fatal("model/input")
		}
		labels, e := MediaLabels(f.Source, r.Original, c.RawResponse)
		if e != nil || !model.IsOfficialCategory(labels.PrimaryCategory) {
			t.Fatal(e)
		}
		p, e := scheduleapply.PrepareProduction(r.Original, c.RawResponse, AIModel, bson.M{}, c.CompletedAt)
		if e != nil {
			t.Fatal(e)
		}
		if !c.CompletedAt.UTC().Truncate(time.Millisecond).Equal(p.Analysis.AnalyzedAt) {
			t.Fatal("BSON millisecond time mismatch")
		}
		if e = scheduleapply.ValidateItem(p); e != nil {
			t.Fatal(e)
		}
		var wire map[string]any
		json.Unmarshal([]byte(c.RawResponse), &wire)
		wire["category"] = "invented"
		b, _ := json.Marshal(wire)
		if _, e = MediaLabels(f.Source, r.Original, string(b)); e == nil {
			t.Fatal("invalid category accepted")
		}
		wire["category"] = labels.PrimaryCategory
		wire["category_evidence"] = []string{"not present in this original 999999"}
		b, _ = json.Marshal(wire)
		if _, e = MediaLabels(f.Source, r.Original, string(b)); e == nil {
			t.Fatal("fabricated evidence accepted")
		}
	}
}
