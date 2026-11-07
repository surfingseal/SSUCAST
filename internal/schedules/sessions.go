package schedules

import (
	"example.com/crawlserver/internal/model"
	"fmt"
	"regexp"
	"strings"
)

var sessionLine = regexp.MustCompile(`(?m)^\s*([1-9][0-9]*)\)\s*([0-9]{1,2})\s*[~～〜–-]\s*([0-9]{1,2})\s*시\s*$`)

// SupplementSessions is a separate rule extraction, not revalidation of missing
// model output. It requires a grounded same-day event followed by an explicit
// contiguous numbered session list. It never downloads or consults a model.
func SupplementSessions(parents []model.Schedule, doc Document) []model.Schedule {
	out := []model.Schedule{}
	seen := map[string]bool{}
	for _, s := range parents {
		seen[Fingerprint(s)] = true
	}
	for _, parent := range parents {
		check := parent
		if !ValidateSchedule(&check, doc) {
			continue
		}
		loc := check.EvidenceLocation
		if parent.Type != "event" || parent.StartDate == nil || parent.EndDate == nil || *parent.StartDate != *parent.EndDate || parent.StartTime == nil || parent.EndTime == nil || !loc.Matched || loc.Field != "content" {
			continue
		}
		tail := doc.Content[loc.ByteEnd:]
		if len(tail) > 1200 {
			tail = tail[:1200]
		}
		header := strings.Index(tail, "타임으로 구분하여 신청")
		if header < 0 || header > 180 {
			continue
		}
		segment := tail[header+len("타임으로 구분하여 신청"):]
		matches := sessionLine.FindAllStringSubmatchIndex(segment, -1)
		if len(matches) == 0 {
			continue
		}
		previousEnd := 0
		for i, m := range matches {
			// Do not cross unrelated prose/headings to find a coincidental later list.
			if strings.TrimSpace(segment[previousEnd:m[0]]) != "" {
				break
			}
			previousEnd = m[1]
			if num(segment[m[2]:m[3]]) != i+1 {
				break
			}
			h1, h2 := num(segment[m[4]:m[5]]), num(segment[m[6]:m[7]])
			if h1 < 0 || h2 > 23 || h1 >= h2 {
				continue
			}
			st, et := fmt.Sprintf("%02d:00", h1), fmt.Sprintf("%02d:00", h2)
			if st < *parent.StartTime || et > *parent.EndTime {
				continue
			}
			ev := strings.TrimSpace(segment[m[0]:m[1]])
			child := model.Schedule{ID: fmt.Sprintf("%s/session/%d", parent.ID, i+1), ParentID: sp(parent.ID), Type: "event", Label: fmt.Sprintf("%s 세션 %d", parent.Label, i+1), StartDate: sp(*parent.StartDate), EndDate: sp(*parent.EndDate), StartTime: sp(st), EndTime: sp(et), StartPrecision: "minute", EndPrecision: "minute", Precision: "minute", Timezone: "Asia/Seoul", Scope: sp(fmt.Sprintf("%s / 세션 %d", value(parent.Scope), i+1)), Evidence: ev, ExtractionMethod: "rule", Origin: "source_rule_supplement", ConversionMethod: "explicit_numbered_session_list", DateContext: "validated_parent_event", ReviewRequired: parent.ReviewRequired, ReviewReasons: append([]string{}, parent.ReviewReasons...), Omissions: []string{}, Qualifiers: append([]string{}, parent.Qualifiers...), Corrections: []model.ScheduleCorrection{}}
			// Dates are inherited only from this grounded parent. The numbered line
			// remains the primary evidence; parent_id links to the date's source span.
			child.EvidenceLocation = MatchEvidence(doc.Title, doc.Content, ev)
			if !child.EvidenceLocation.Matched {
				continue
			}
			child.StartAt = compat(child.StartDate, child.StartTime, child.Timezone)
			child.EndAt = compat(child.EndDate, child.EndTime, child.Timezone)
			// Preserve date context separately so downstream validation can resolve it
			// against the parent instead of trusting a free-form date_context string.
			key := Fingerprint(child)
			duplicate := false
			for _, existing := range parents {
				if existing.Type == "event" && existing.ID != parent.ID && (sameValue(existing.ParentID, child.ParentID) || sameValue(existing.Scope, child.Scope)) && sameValue(existing.StartDate, child.StartDate) && sameValue(existing.EndDate, child.EndDate) && sameValue(existing.StartTime, child.StartTime) && sameValue(existing.EndTime, child.EndTime) {
					duplicate = true
				}
			}
			if !seen[key] && !duplicate {
				out = append(out, child)
				seen[key] = true
			}
		}
	}
	return out
}
func value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func sameValue(a, b *string) bool { return value(a) == value(b) }

// ValidateChild requires an existing grounded parent. Parent association is not
// established by dates alone: this is the same explicit source list used above.
func ValidateChild(child model.Schedule, parents []model.Schedule, doc Document) bool {
	if child.ParentID == nil || child.ExtractionMethod != "rule" || child.Origin != "source_rule_supplement" {
		return false
	}
	for _, expected := range SupplementSessions(parents, doc) {
		if expected.ID == child.ID && Fingerprint(expected) == Fingerprint(child) && expected.Evidence == child.Evidence {
			return true
		}
	}
	return false
}
