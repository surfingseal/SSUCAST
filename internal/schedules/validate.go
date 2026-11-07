package schedules

import (
	"example.com/crawlserver/internal/model"
	"regexp"
	"time"
)

var dateForm = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
var clockForm = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)

// ValidateSchedule can also validate future v2 responses directly. Unknown
// information remains absent; it never changes legacy deadline/lifecycle state.
// ReviewRequired expresses uncertainty; it is not equivalent to an invalid date.
func ValidateSchedule(s *model.Schedule, doc Document) bool {
	s.EvidenceLocation = MatchEvidence(doc.Title, doc.Content, s.Evidence)
	if s.ReviewReasons == nil {
		s.ReviewReasons = []string{}
	}
	if s.Omissions == nil {
		s.Omissions = []string{}
	}
	if !s.EvidenceLocation.Matched {
		reason(s, "evidence_mismatch")
		return false
	}
	valid := true
	if unsupportedCalendarInSource(s.EvidenceLocation.OriginalText) {
		reason(s, "invalid_calendar_value")
		valid = false
	}
	if unsupportedClockInSource(s.EvidenceLocation.OriginalText) {
		reason(s, "unsupported_datetime")
		valid = false
	}
	if s.Timezone != "Asia/Seoul" {
		reason(s, "unsupported_timezone")
		valid = false
	}
	switch s.Type {
	case "application", "event", "submission", "completion", "announcement", "interview", "selection":
	default:
		reason(s, "unsupported_type")
		valid = false
	}
	ds := dates(s.EvidenceLocation.OriginalText)
	cs := clocks(s.EvidenceLocation.OriginalText, ds)
	for j := 0; j < 2; j++ {
		date, clock, partial, at, precision := s.StartDate, s.StartTime, s.StartPartial, s.StartAt, s.StartPrecision
		if j == 1 {
			date, clock, partial, at, precision = s.EndDate, s.EndTime, s.EndPartial, s.EndAt, s.EndPrecision
		}
		if date != nil {
			if !dateForm.MatchString(*date) {
				reason(s, "invalid_calendar_value")
				valid = false
			} else if _, e := time.Parse("2006-01-02", *date); e != nil {
				reason(s, "invalid_calendar_value")
				valid = false
			}
			if partial != nil {
				reason(s, "conflicting_calendar_values")
				valid = false
			}
			supported := false
			for _, d := range ds {
				if d.parts.Year != nil && d.parts.Month != nil && d.parts.Day != nil {
					t := time.Date(*d.parts.Year, time.Month(*d.parts.Month), *d.parts.Day, 0, 0, 0, 0, time.UTC)
					if t.Format("2006-01-02") == *date {
						supported = true
					}
				}
			}
			// Child dates are explicitly inherited from a validated parent and are
			// checked by SupplementSessions, never by an arbitrary caller's claim.
			if !supported {
				reason(s, "unsupported_datetime")
				valid = false
			}
		}
		if partial != nil {
			if !validPartial(partial) {
				reason(s, "invalid_calendar_value")
				valid = false
			}
			supported := false
			for _, d := range ds {
				if partsKey(partial) == partsKey(&d.parts) {
					supported = true
				}
			}
			if !supported {
				reason(s, "unsupported_datetime")
				valid = false
			}
			if partial.Year == nil {
				reason(s, "missing_year")
			}
		}
		if clock != nil {
			supported := false
			for _, c := range cs {
				if c.value == *clock {
					supported = true
				}
			}
			if !clockForm.MatchString(*clock) || !supported {
				reason(s, "unsupported_datetime")
				valid = false
			}
			if date == nil && partial == nil {
				reason(s, "missing_date")
				valid = false
			}
		} else if date != nil || partial != nil {
			s.Omissions = unique(s.Omissions, "missing_time")
		}
		expected := compat(date, clock, s.Timezone)
		if at != nil && !sameTimestamp(at, expected) {
			reason(s, "unsupported_datetime")
			valid = false
		}
		want := boundaryPrecision(date, clock, partial)
		if precision != want {
			reason(s, "unsupported_precision")
			valid = false
		}
	}
	if s.StartDate != nil && s.EndDate != nil {
		start, end := *s.StartDate, *s.EndDate
		if s.StartTime != nil && s.EndTime != nil {
			start += "T" + *s.StartTime
			end += "T" + *s.EndTime
		}
		if start > end {
			reason(s, "reversed_range")
			valid = false
		}
	}
	expectedPrecision := s.StartPrecision
	if expectedPrecision == "unknown" {
		expectedPrecision = s.EndPrecision
	} else if s.EndPrecision != "unknown" && s.EndPrecision != expectedPrecision {
		expectedPrecision = "mixed"
	}
	if s.Precision != expectedPrecision {
		reason(s, "unsupported_precision")
		valid = false
	}
	if s.StartDate == nil && s.EndDate == nil && s.StartPartial == nil && s.EndPartial == nil {
		reason(s, "unsupported_datetime")
		valid = false
	}
	s.ReviewRequired = len(s.ReviewReasons) > 0
	return valid
}
func boundaryPrecision(date, clock *string, p *model.CalendarParts) string {
	if date != nil {
		if clock != nil {
			return "minute"
		}
		return "date"
	}
	if p == nil {
		return "unknown"
	}
	if p.Month != nil && p.Day == nil {
		return "month"
	}
	if p.Year != nil {
		return "year"
	}
	if p.Month != nil && p.Day != nil {
		return "month_day"
	}
	return "unknown"
}
func validPartial(p *model.CalendarParts) bool {
	if p.Year == nil && p.Month == nil && p.Day == nil {
		return false
	}
	if p.Year != nil && (*p.Year < 1 || *p.Year > 9999) {
		return false
	}
	if p.Month != nil && (*p.Month < 1 || *p.Month > 12) {
		return false
	}
	if p.Day != nil {
		if p.Month == nil || *p.Day < 1 || *p.Day > 31 {
			return false
		}
		year := 2000
		if p.Year != nil {
			year = *p.Year
		}
		if !validDate(year, *p.Month, *p.Day) {
			return false
		}
	}
	return !(p.Year != nil && p.Month != nil && p.Day != nil)
}
func ValidateSet(rows []model.Schedule) []string {
	issues := []string{}
	seen := map[string]string{}
	ids := map[string]bool{}
	for _, s := range rows {
		if ids[s.ID] {
			issues = append(issues, "duplicate_id:"+s.ID)
		}
		ids[s.ID] = true
	}
	for _, s := range rows {
		if s.ParentID != nil && (!ids[*s.ParentID] || *s.ParentID == s.ID) {
			issues = append(issues, "invalid_parent:"+s.ID)
		}
		key := Fingerprint(s)
		if id, ok := seen[key]; ok {
			issues = append(issues, "duplicate_schedule:"+id+":"+s.ID)
		} else {
			seen[key] = s.ID
		}
	}
	return issues
}
