package schedules

import (
	"example.com/crawlserver/internal/model"
	"fmt"
)

type BoundaryDiff struct {
	Field      string               `json:"field"`
	PreviousAt *string              `json:"previous_at"`
	Date       *string              `json:"date"`
	Partial    *model.CalendarParts `json:"partial"`
	Time       *string              `json:"time"`
	Restored   bool                 `json:"restored"`
}
type ScheduleDiff struct {
	ID                         string         `json:"id"`
	OldAIReviewRequired        bool           `json:"old_ai_review_required"`
	OldValidatedReviewRequired bool           `json:"old_validated_review_required"`
	NewReviewRequired          bool           `json:"new_review_required"`
	NewReviewReasons           []string       `json:"new_review_reasons"`
	OldExactEvidenceMatched    bool           `json:"old_exact_evidence_matched"`
	NewEvidenceMatch           string         `json:"new_evidence_match"`
	Boundaries                 []BoundaryDiff `json:"boundaries"`
}
type NoticeResult struct {
	ID              string             `json:"id"`
	Title           string             `json:"title"`
	Summary         string             `json:"summary"`
	Audience        AudienceValidation `json:"audience_validation"`
	Schedules       []model.Schedule   `json:"schedules"`
	RuleSupplements []model.Schedule   `json:"rule_supplements"`
	SetIssues       []string           `json:"set_issues"`
	Diffs           []ScheduleDiff     `json:"diffs"`
}

func Revalidate(id string, raw, baseline LegacyOutput, doc Document) (NoticeResult, error) {
	if len(raw.Schedules) != len(baseline.Schedules) {
		return NoticeResult{}, fmt.Errorf("legacy schedule count mismatch")
	}
	out := NoticeResult{ID: id, Title: doc.Title, Summary: raw.Summary, Audience: ValidateAudience(raw, doc), Schedules: []model.Schedule{}, RuleSupplements: []model.Schedule{}, Diffs: []ScheduleDiff{}}
	for i, old := range raw.Schedules {
		prev := baseline.Schedules[i]
		if prev.Evidence != old.Evidence || prev.Type != old.Type || prev.Label != old.Label {
			return NoticeResult{}, fmt.Errorf("legacy schedule alignment mismatch")
		}
		s := FromLegacy(id, i, old, doc)
		out.Schedules = append(out.Schedules, s)
		d := ScheduleDiff{ID: s.ID, OldAIReviewRequired: old.ReviewRequired, OldValidatedReviewRequired: prev.ReviewRequired, NewReviewRequired: s.ReviewRequired, NewReviewReasons: s.ReviewReasons, OldExactEvidenceMatched: exactEvidence(doc, old.Evidence), NewEvidenceMatch: s.EvidenceLocation.MatchType, Boundaries: []BoundaryDiff{}}
		d.Boundaries = append(d.Boundaries, BoundaryDiff{"start", prev.StartAt, s.StartDate, s.StartPartial, s.StartTime, prev.StartAt == nil && (s.StartDate != nil || s.StartPartial != nil)})
		d.Boundaries = append(d.Boundaries, BoundaryDiff{"end", prev.EndAt, s.EndDate, s.EndPartial, s.EndTime, prev.EndAt == nil && (s.EndDate != nil || s.EndPartial != nil)})
		out.Diffs = append(out.Diffs, d)
	}
	out.RuleSupplements = SupplementSessions(out.Schedules, doc)
	out.SetIssues = ValidateSet(append(append([]model.Schedule{}, out.Schedules...), out.RuleSupplements...))
	return out, nil
}
func exactEvidence(doc Document, evidence string) bool {
	m := MatchEvidence(doc.Title, doc.Content, evidence)
	return m.Matched && m.MatchType == "exact"
}
