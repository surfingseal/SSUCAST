package schedules

import (
	"example.com/crawlserver/internal/model"
	"fmt"
	"strings"
	"time"
)

func emptyParts(p *model.CalendarParts) bool {
	return p != nil && p.Year == nil && p.Month == nil && p.Day == nil
}

// repairV2Boundaries uses the model's proposed boundary to select matching
// explicit source components. It does not choose the latest date or derive an
// end date from a duration. Missing years/days remain missing.
func repairV2Boundaries(s model.Schedule, doc Document) model.Schedule {
	out := s
	out.StartDate = nil
	out.EndDate = nil
	out.StartTime = nil
	out.EndTime = nil
	out.StartPartial = nil
	out.EndPartial = nil
	out.StartAt = nil
	out.EndAt = nil
	out.StartPrecision = "unknown"
	out.EndPrecision = "unknown"
	out.Precision = "unknown"
	out.EvidenceLocation = MatchEvidence(doc.Title, doc.Content, s.Evidence)
	if !out.EvidenceLocation.Matched {
		return out
	}
	tokens := dates(out.EvidenceLocation.OriginalText)
	times := clocks(out.EvidenceLocation.OriginalText, tokens)
	for j := 0; j < 2; j++ {
		d, p, clock := s.StartDate, s.StartPartial, s.StartTime
		if j == 1 {
			d, p, clock = s.EndDate, s.EndPartial, s.EndTime
		}
		var claim *model.CalendarParts
		if d != nil {
			if t, e := time.Parse("2006-01-02", *d); e == nil {
				claim = &model.CalendarParts{Year: ip(t.Year()), Month: ip(int(t.Month())), Day: ip(t.Day())}
			}
		}
		if claim == nil && p != nil && !emptyParts(p) {
			copy := *p
			claim = &copy
		}
		if claim == nil {
			continue
		}
		matches := []dateToken{}
		for _, t := range tokens {
			// Month/day must not be obtained from a background year token.
			if claim.Month != nil && (t.parts.Month == nil || *t.parts.Month != *claim.Month) {
				continue
			}
			if claim.Day != nil && t.parts.Day != nil && *claim.Day != *t.parts.Day {
				continue
			}
			if claim.Month == nil && claim.Year != nil && (t.parts.Year == nil || *t.parts.Year != *claim.Year) {
				continue
			}
			matches = append(matches, t)
		}
		if len(matches) == 0 {
			continue
		}
		// Prefer a complete explicit token matching the claimed year over a shorter
		// repeated month/day reference, without filling a year that is not printed.
		chosen := matches[0]
		score := func(t dateToken) int {
			v := 0
			if t.parts.Year != nil {
				v += 2
				if claim.Year != nil && *t.parts.Year == *claim.Year {
					v += 4
				}
			}
			if t.parts.Day != nil {
				v++
			}
			return v
		}
		for _, t := range matches[1:] {
			if score(t) > score(chosen) {
				chosen = t
			}
		}
		var verifiedTime *string
		if clock != nil {
			for _, c := range times {
				if c.value == *clock {
					verifiedTime = sp(c.value)
					break
				}
			}
		}
		setBoundary(&out, j == 0, chosen, verifiedTime)
	}
	updatePrecision(&out)
	out.ExtractionMethod = "rule"
	out.ConversionMethod = "v2_source_component_repair"
	out.Corrections = append(out.Corrections, model.ScheduleCorrection{Code: "unsupported_datetime", Field: "model_calendar", Original: "source components retained; original claims remain in raw_response"})
	return out
}
func partialsNormalize(s *model.Schedule) {
	for j := 0; j < 2; j++ {
		p, d := s.StartPartial, s.StartDate
		if j == 1 {
			p, d = s.EndPartial, s.EndDate
		}
		if emptyParts(p) {
			if j == 0 {
				s.StartPartial = nil
			} else {
				s.EndPartial = nil
			}
			s.Corrections = append(s.Corrections, model.ScheduleCorrection{Code: "empty_partial_normalized", Field: fmt.Sprint(j), Original: "all partial components null"})
			continue
		}
		if p != nil && p.Year != nil && p.Month != nil && p.Day != nil {
			value := fmt.Sprintf("%04d-%02d-%02d", *p.Year, *p.Month, *p.Day)
			if d == nil {
				d = sp(value)
			} else if *d != value {
				reason(s, "conflicting_calendar_values")
			}
			if j == 0 {
				s.StartDate = d
				s.StartPartial = nil
			} else {
				s.EndDate = d
				s.EndPartial = nil
			}
			s.Corrections = append(s.Corrections, model.ScheduleCorrection{Code: "full_partial_normalized", Field: fmt.Sprint(j), Original: value})
		}
	}
}
func sourceApproximation(s *model.Schedule, doc Document) {
	text := s.Evidence
	if strings.Contains(text, "초중순") || strings.Contains(text, "중하순") || strings.Contains(text, "월 말") || strings.Contains(text, "월 초") || strings.Contains(text, "월 중") {
		reason(s, "approximate_date")
	}
	if strings.ContainsAny(text, "’‘'") && (strings.Contains(text, "’26") || strings.Contains(text, "’27") || strings.Contains(text, "'26") || strings.Contains(text, "'27")) {
		reason(s, "abbreviated_year")
	}
}
