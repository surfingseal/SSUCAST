package notices

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"unicode/utf8"
)

// TimingReview is explanatory information, never a computed date or schedule.
// Reviewed source material remains private; only the student-facing excerpt is
// returned. A stale source/analysis fingerprint suppresses the entire overlay.
type TimingEvidence struct {
	Source string `json:"source"`
	Text   string `json:"text"`
	URL    string `json:"url,omitempty"`
}
type TimingReview struct {
	Status         string           `json:"status"`
	Label          string           `json:"label"`
	Message        string           `json:"message"`
	ReviewRequired bool             `json:"review_required"`
	Rules          []string         `json:"rules,omitempty"`
	Evidence       []TimingEvidence `json:"evidence,omitempty"`
}
type TimingRecord struct {
	ID          string       `json:"id"`
	ContentHash string       `json:"content_hash"`
	AnalysisID  string       `json:"analysis_id"`
	Review      TimingReview `json:"review"`
}
type TimingReviews map[string]TimingRecord

func LoadTimingReviews(path string) (TimingReviews, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, 2<<20+1))
	if e != nil || len(raw) > 2<<20 {
		return nil, fmt.Errorf("timing review file too large")
	}
	var records []TimingRecord
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&records) != nil || d.Decode(new(any)) != io.EOF || len(records) > 500 {
		return nil, fmt.Errorf("invalid timing review file")
	}
	out := TimingReviews{}
	valid := map[string]bool{"informational": true, "other_schedule_available": true, "attachment_required": true, "linked_notice_required": true, "image_unavailable": true, "relative_or_recurring": true, "insufficient_information": true}
	for _, r := range records {
		if len(r.ID) != 24 || r.ContentHash == "" || r.AnalysisID == "" || !valid[r.Review.Status] || r.Review.Label == "" || r.Review.Message == "" || utf8.RuneCountInString(r.Review.Label) > 60 || utf8.RuneCountInString(r.Review.Message) > 500 || len(r.Review.Rules) > 10 || len(r.Review.Evidence) > 10 {
			return nil, fmt.Errorf("invalid timing review record")
		}
		if _, ok := out[r.ID]; ok {
			return nil, fmt.Errorf("duplicate timing review record")
		}
		for i, v := range r.Review.Evidence {
			if v.Source != "body" && v.Source != "image" && v.Source != "existing_schedule" {
				return nil, fmt.Errorf("invalid timing evidence source")
			}
			if utf8.RuneCountInString(v.Text) > 2000 {
				return nil, fmt.Errorf("timing excerpt too long")
			}
			r.Review.Evidence[i].URL = SafeURL(v.URL)
		}
		for _, v := range r.Review.Rules {
			if utf8.RuneCountInString(v) > 500 {
				return nil, fmt.Errorf("timing rule too long")
			}
		}
		out[r.ID] = r
	}
	return out, nil
}
func (r TimingReviews) Apply(n *Notice) {
	v, ok := r[n.ID]
	if !ok || v.ContentHash != n.reviewContentHash || v.AnalysisID != n.reviewAnalysisID {
		return
	}
	// Return a copy so a client projection cannot mutate shared server state.
	x := v.Review
	x.Rules = append([]string(nil), x.Rules...)
	x.Evidence = append([]TimingEvidence(nil), x.Evidence...)
	n.TimingReview = &x
}
