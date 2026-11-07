package imageanalysis

import (
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/schedulebatch"
	"fmt"
	"strings"
)

const ReviewVersion = "schedule-image-visual-review-v2"

type ReviewedCandidate struct {
	OriginalIndex int            `json:"original_candidate_index" bson:"original_candidate_index"`
	Value         ImageCandidate `json:"value" bson:"value"`
	Quotes        []string       `json:"source_quotes" bson:"source_quotes"`
	Reason        string         `json:"reason" bson:"reason"`
}
type VisualReview struct {
	RawHash    string              `json:"original_raw_hash" bson:"original_raw_hash"`
	Method     string              `json:"method" bson:"method"`
	Candidates []ReviewedCandidate `json:"candidates" bson:"candidates"`
}

func reviewHash(c Candidate, r *VisualReview) string {
	return schedulebatch.Hash([]any{ReviewVersion, InputHash(c), r})
}
func reviewedOutput(r Result) (Output, error) {
	if r.Review == nil || r.Review.RawHash != digest([]byte(r.Raw)) || r.Review.Method != "agent_visual_source_check" {
		return Output{}, fmt.Errorf("review provenance missing")
	}
	original, e := Decode(r.Raw)
	if e != nil {
		return Output{}, e
	}
	out := Output{Candidates: []ImageCandidate{}, SummarySupport: []string{}, AudienceSupport: []string{}, Reasons: []string{"agent_visual_review_not_human_verified"}, Review: true, Transcriptions: []Transcription{}}
	texts := map[int][]string{}
	order := []int{}
	for _, c := range r.Review.Candidates {
		if c.OriginalIndex < 0 || c.OriginalIndex >= len(original.Candidates) || len(c.Quotes) == 0 || c.Reason == "" {
			return Output{}, fmt.Errorf("invalid source correction")
		}
		v := c.Value
		v.EvidenceText = strings.Join(c.Quotes, "\n")
		v.ReviewRequired = true
		v.ReviewReasons = appendUnique(v.ReviewReasons, "agent_visual_review_not_human_verified")
		if _, ok := texts[v.ImageIndex]; !ok {
			order = append(order, v.ImageIndex)
		}
		texts[v.ImageIndex] = append(texts[v.ImageIndex], c.Quotes...)
		out.Candidates = append(out.Candidates, v)
	}
	for _, index := range order {
		out.Transcriptions = append(out.Transcriptions, Transcription{Index: index, Text: strings.Join(texts[index], "\n"), Readable: true})
	}
	return out, nil
}
func applyReviewedEvidence(ss []model.Schedule, r Result) {
	// Each quotation remains separate: no synthesized contiguous quotation.
	for i := range ss {
		for _, v := range r.Review.Candidates {
			if ss[i].Label != v.Value.Label {
				continue
			}
			old := ss[i].ImageEvidence[0]
			ss[i].ImageEvidence = nil
			for _, q := range v.Quotes {
				p := old
				p.Text = q
				p.Validation = "agent_visual_source_check_not_human_verified"
				ss[i].ImageEvidence = append(ss[i].ImageEvidence, p)
			}
			ss[i].Evidence = "개별 이미지 근거 참조"
			ss[i].Origin = "image_ai"
			ss[i].ExtractionMethod = "ai_with_visual_validation"
			ss[i].ConversionMethod = ReviewVersion
		}
	}
}
