package schedules

import (
	"crypto/sha256"
	"example.com/crawlserver/internal/model"
	"fmt"
	"strings"
	"time"
)

const Version = "schedule-v2"

type LegacySchedule struct {
	Type           string  `json:"type"`
	Label          string  `json:"label"`
	StartAt        *string `json:"start_at"`
	EndAt          *string `json:"end_at"`
	Evidence       string  `json:"evidence"`
	Scope          *string `json:"scope"`
	ReviewRequired bool    `json:"review_required"`
	ReviewReason   *string `json:"review_reason"`
}
type LegacyOutput struct {
	Summary               string           `json:"summary"`
	TargetAudience        *string          `json:"target_audience"`
	SummaryReviewRequired bool             `json:"summary_review_required"`
	Schedules             []LegacySchedule `json:"schedules"`
}
type Document struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func stableID(id string, i int) string { return fmt.Sprintf("%s/schedule/%02d", id, i+1) }

// FromLegacy never uses published_at, today's year, discarded clocks or a model
// to fill a missing calendar component. All recovered values come from matched
// evidence. Raw output is retained by the caller; derivation is labelled rule.
func FromLegacy(id string, i int, old LegacySchedule, doc Document) model.Schedule {
	s := model.Schedule{ID: stableID(id, i), Type: old.Type, Label: old.Label, Scope: old.Scope, Evidence: old.Evidence, Timezone: "Asia/Seoul", Precision: "unknown", StartPrecision: "unknown", EndPrecision: "unknown", ExtractionMethod: "rule", Origin: "saved_ai_response", ConversionMethod: "legacy_v1_evidence_reparse", ReviewReasons: []string{}, Omissions: []string{}, Qualifiers: []string{}, Corrections: []model.ScheduleCorrection{}}
	s.EvidenceLocation = MatchEvidence(doc.Title, doc.Content, old.Evidence)
	if !s.EvidenceLocation.Matched {
		reason(&s, "evidence_mismatch")
		return s
	}
	text := s.EvidenceLocation.OriginalText
	if unsupportedClockInSource(text) {
		reason(&s, "unsupported_datetime")
	}
	ds := dates(text)
	cs := clocks(text, ds)
	if len(ds) == 0 {
		reason(&s, "unsupported_datetime")
		return s
	}
	deadline := old.Type == "application" || old.Type == "submission" || old.Type == "completion"
	// Legacy endpoints are hints only for direction, never authoritative dates.
	if len(ds) == 2 {
		if !hasRange(text[ds[0].end:ds[1].start]) {
			reason(&s, "multiple_scopes")
		} else {
			var startClock, endClock *string
			for _, c := range cs {
				if c.start >= ds[0].end && c.start < ds[1].start {
					startClock = sp(c.value)
				}
				if c.start >= ds[1].end {
					endClock = sp(c.value)
				}
			}
			setBoundary(&s, true, ds[0], startClock)
			setBoundary(&s, false, ds[1], endClock)
		}
	} else if len(ds) == 1 {
		var clock *string
		if len(cs) > 0 {
			clock = sp(cs[0].value)
		}
		if len(cs) == 2 && hasRange(text[cs[0].end:cs[1].start]) {
			setBoundary(&s, true, ds[0], sp(cs[0].value))
			setBoundary(&s, false, ds[0], sp(cs[1].value))
		} else if deadline && (old.StartAt == nil || strings.Contains(text, "까지") || strings.Contains(text, "마감") || strings.Contains(text, "~")) {
			if len(cs) > 1 {
				reason(&s, "multiple_scopes")
			}
			setBoundary(&s, false, ds[0], clock)
		} else {
			setBoundary(&s, true, ds[0], clock)
			if len(cs) == 2 && hasRange(text[cs[0].end:cs[1].start]) {
				setBoundary(&s, false, ds[0], sp(cs[1].value))
			} else if len(cs) > 1 {
				reason(&s, "multiple_scopes")
			}
		}
	} else {
		reason(&s, "multiple_scopes")
	}
	// Calendar precision describes what is actually known, not the old review bit.
	updatePrecision(&s)
	context := evidenceLineContext(doc, s.EvidenceLocation)
	if strings.Contains(context, "예정") || strings.Contains(context, "추후") || strings.Contains(context, "변경될") {
		reason(&s, "tentative_schedule")
		s.Qualifiers = unique(s.Qualifiers, "tentative")
	}
	if strings.Contains(text, "이후") {
		s.Qualifiers = unique(s.Qualifiers, "on_or_after")
		reason(&s, "conditional_deadline")
	}
	if strings.Contains(context, "인원 미달") || strings.Contains(context, "추가 모집") {
		reason(&s, "conditional_deadline")
	}
	if strings.Contains(doc.Title, "겨울") && strings.Contains(doc.Content, "여름학기") {
		reason(&s, "source_conflict")
	}
	for j, raw := range []*string{old.StartAt, old.EndAt} {
		if raw == nil {
			continue
		}
		var current *string
		field := "start_at"
		if j == 0 {
			current = s.StartAt
		} else {
			current = s.EndAt
			field = "end_at"
		}
		if !sameTimestamp(raw, current) {
			s.Corrections = append(s.Corrections, model.ScheduleCorrection{Code: "unsupported_datetime", Field: field, Original: *raw})
		}
	}
	if old.ReviewRequired && (old.ReviewReason == nil || strings.TrimSpace(*old.ReviewReason) != "시각 미기재") {
		reason(&s, "legacy_review_unresolved")
	}
	ValidateSchedule(&s, doc)
	return s
}
func sameTimestamp(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	x, e := time.Parse(time.RFC3339, *a)
	y, f := time.Parse(time.RFC3339, *b)
	return e == nil && f == nil && x.Equal(y)
}
func updatePrecision(s *model.Schedule) {
	s.StartAt = compat(s.StartDate, s.StartTime, s.Timezone)
	s.EndAt = compat(s.EndDate, s.EndTime, s.Timezone)
	s.Precision = s.StartPrecision
	if s.Precision == "unknown" {
		s.Precision = s.EndPrecision
	} else if s.EndPrecision != "unknown" && s.EndPrecision != s.Precision {
		s.Precision = "mixed"
	}
}
func compat(date, clock *string, zone string) *string {
	if date == nil || clock == nil || zone != "Asia/Seoul" {
		return nil
	}
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return nil
	}
	t, e := time.ParseInLocation("2006-01-02 15:04", *date+" "+*clock, loc)
	if e != nil {
		return nil
	}
	return sp(t.Format(time.RFC3339))
}
func evidenceLineContext(doc Document, loc model.EvidenceLocation) string {
	source := doc.Content
	if loc.Field == "title" {
		source = doc.Title
	}
	if !loc.Matched {
		return ""
	}
	start := strings.LastIndex(source[:loc.ByteStart], "\n") + 1
	end := strings.Index(source[loc.ByteEnd:], "\n")
	if end < 0 {
		end = len(source)
	} else {
		end += loc.ByteEnd
	}
	return source[start:end]
}

// Fingerprint preserves scope and hierarchy, so parallel institutions/rounds
// and an overall event with a child session are not accidentally collapsed.
func Fingerprint(s model.Schedule) string {
	val := func(x *string) string {
		if x == nil {
			return ""
		}
		return *x
	}
	raw := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%v|%v|%s|%s", s.Type, val(s.Scope), val(s.ParentID), val(s.StartDate), val(s.StartTime), val(s.EndDate), partsKey(s.StartPartial), partsKey(s.EndPartial), val(s.EndTime), s.Timezone)
	if s.Precision == "unknown" && s.StartDate == nil && s.EndDate == nil && s.StartPartial == nil && s.EndPartial == nil {
		raw += "|" + NormalizeWhitespace(s.Evidence)
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
}
func partsKey(p *model.CalendarParts) string {
	if p == nil {
		return ""
	}
	v := func(n *int) string {
		if n == nil {
			return "?"
		}
		return fmt.Sprint(*n)
	}
	return v(p.Year) + "-" + v(p.Month) + "-" + v(p.Day)
}

// FromV2 is the conversion layer for future responses using the new calendar
// fields. It never invokes Gemini. Supplied precision/compatibility conflicts
// are reviewed rather than silently overwritten.
func FromV2(s model.Schedule, doc Document) model.Schedule {
	if s.StartPrecision == "" {
		s.StartPrecision = boundaryPrecision(s.StartDate, s.StartTime, s.StartPartial)
	}
	if s.EndPrecision == "" {
		s.EndPrecision = boundaryPrecision(s.EndDate, s.EndTime, s.EndPartial)
	}
	if s.Precision == "" {
		copy := s
		updatePrecision(&copy)
		s.Precision = copy.Precision
	}
	if s.StartAt == nil {
		s.StartAt = compat(s.StartDate, s.StartTime, s.Timezone)
	}
	if s.EndAt == nil {
		s.EndAt = compat(s.EndDate, s.EndTime, s.Timezone)
	}
	ValidateSchedule(&s, doc)
	return s
}
