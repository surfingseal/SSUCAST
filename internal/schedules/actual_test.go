package schedules

import (
 "example.com/crawlserver/internal/testutil/localfixture"
	"encoding/json"
	"os"
	"testing"
)

// These are actual previously saved notices and model responses, not synthetic
// fixtures. This test performs only local file reads and pure transformations.
func TestActualTenNoticesTwentyFourSchedules(t *testing.T) {
	localfixture.Require(t, "testdata/actual10.json")
	raw, e := os.ReadFile("testdata/actual10.json")
	if e != nil {
		t.Fatal(e)
	}
	var corpus struct {
		Items []struct {
			ID string `json:"id"`
			Document
			Raw      string       `json:"raw_response"`
			Baseline LegacyOutput `json:"validated_output"`
		} `json:"items"`
	}
	if e = json.Unmarshal(raw, &corpus); e != nil {
		t.Fatal(e)
	}
	total, valid, review, restoredFull, restoredPartial, resolved, children, audience := 0, 0, 0, 0, 0, 0, 0, 0
	for _, v := range corpus.Items {
		var old LegacyOutput
		if e = json.Unmarshal([]byte(v.Raw), &old); e != nil {
			t.Fatal(e)
		}
		n, e := Revalidate(v.ID, old, v.Baseline, v.Document)
		if e != nil {
			t.Fatal(e)
		}
		if n.Summary != old.Summary {
			t.Fatal("original summary altered")
		}
		if n.Audience.ReviewRequired {
			audience++
		}
		for i, s := range n.Schedules {
			total++
			check := s
			if ValidateSchedule(&check, v.Document) {
				valid++
			} else {
				t.Fatalf("invalid conversion %+v", check)
			}
			if s.ReviewRequired {
				review++
			}
			d := n.Diffs[i]
			if !d.OldExactEvidenceMatched && s.EvidenceLocation.Matched {
				resolved++
			}
			for _, b := range d.Boundaries {
				if b.Restored {
					if b.Date != nil {
						restoredFull++
					} else {
						restoredPartial++
					}
				}
			}
		}
		for _, child := range n.RuleSupplements {
			children++
			if !ValidateChild(child, n.Schedules, v.Document) {
				t.Fatal(child)
			}
		}
	}
	if total != 24 || valid != 24 || review != 12 || restoredFull != 8 || restoredPartial != 13 || resolved != 1 || children != 4 || audience != 5 {
		t.Fatalf("total=%d valid=%d review=%d restored=%d+%d whitespace=%d children=%d audience=%d", total, valid, review, restoredFull, restoredPartial, resolved, children, audience)
	}
}
