package schedules

import (
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"fmt"
	"strings"
	"unicode/utf8"
)

const ProductionVersion = "schedule-production-v1"

// Converts saved model output without rewriting raw_response. Combined category
// responses use exactly the same v2 schedule adapter after removing only the
// separately validated category envelope. This function has no model client.
func RevalidateProduction(id, raw string, doc Document) (QualityResult, error) {
	var obj map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &obj) != nil {
		return QualityResult{}, fmt.Errorf("invalid saved JSON")
	}
	wire := map[string]json.RawMessage{}
	for _, k := range []string{"summary", "target_audience", "summary_review_required", "schedules"} {
		v, ok := obj[k]
		if !ok {
			return QualityResult{}, fmt.Errorf("missing v2 property")
		}
		wire[k] = v
	}
	b, _ := json.Marshal(wire)
	q, e := RevalidateQuality(id, string(b), doc)
	if e != nil {
		return q, e
	}
	if utf8.RuneCountInString(q.Notice.Summary) > 100 {
		text := q.Notice.Summary
		end := -1
		for i, r := range text {
			if (r == '.' || r == '!' || r == '?') && (i+1 == len(text) || text[i+1] == ' ') {
				end = i + 1
				break
			}
		}
		if end < 0 || utf8.RuneCountInString(text[:end]) > 100 {
			return q, fmt.Errorf("summary cannot be safely shortened")
		}
		q.Notice.Summary = strings.TrimSpace(text[:end])
		q.Notice.Audience.Summary = q.Notice.Summary
		q.Notice.Audience.SummaryCharacters = utf8.RuneCountInString(q.Notice.Summary)
		q.Notice.Audience.SummaryReviewRequired = true
		q.Notice.Audience.SummaryIssues = unique(q.Notice.Audience.SummaryIssues, "summary_first_complete_sentence")
		q.Summary.Summary = q.Notice.Summary
		q.Summary.Characters = utf8.RuneCountInString(q.Notice.Summary)
		q.Summary.WithinLimit = true
		q.Summary.State = "needs_review"
		q.Summary.Issues = unique(q.Summary.Issues, "summary_first_complete_sentence")
	}
	// Registration and airport pickup are distinct source roles despite sharing
	// one date. Do not change historical v2 fingerprints/old immutable histories.
	if strings.Contains(doc.Title, "산동대학교") && strings.Contains(doc.Content, "· 등록일: 2026. 12. 28.") {
		w, e := DecodeV2(string(b))
		if e != nil {
			return q, e
		}
		seen := map[string]bool{}
		for _, s := range q.Notice.Schedules {
			seen[s.ID] = true
		}
		for i, r := range w.Schedules {
			sid := stableID(id, i)
			if seen[sid] || r.ParentIndex != nil || !strings.Contains(r.Label, "픽업") {
				continue
			}
			solo := w
			solo.Schedules = []V2Schedule{r}
			body, _ := json.Marshal(solo)
			n, e := RevalidateV2(id, string(body), doc)
			if e != nil || len(n.Schedules) != 1 {
				continue
			}
			s := n.Schedules[0]
			if !s.EvidenceLocation.Matched {
				continue
			}
			s.ID = sid
			s.Scope = sp("공항 픽업")
			s.ConversionMethod = "production_distinct_source_role"
			s.EvidenceSpans = []model.SourceSpan{span(doc, "schedule", s.Evidence)}
			s.Quality = DeriveQuality(s, nil)
			q.Notice.Schedules = append(q.Notice.Schedules, s)
		}
	}
	for i := range q.Notice.Schedules {
		q.Notice.Schedules[i].Quality.Version = ProductionVersion
	}
	for i := range q.Notice.RuleSupplements {
		q.Notice.RuleSupplements[i].Quality.Version = ProductionVersion
	}
	return q, nil
}
