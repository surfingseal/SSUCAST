package schedules

import (
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"fmt"
	"io"
	"strings"
)

// V2Response is only a model wire adapter. Persistence remains model.Schedule.
// Stable IDs, offsets, compatibility timestamps and provenance are assigned in
// Go rather than trusting model-generated bookkeeping.
type V2Schedule struct {
	Type           string               `json:"type"`
	Label          string               `json:"label"`
	StartDate      *string              `json:"start_date"`
	StartTime      *string              `json:"start_time"`
	EndDate        *string              `json:"end_date"`
	EndTime        *string              `json:"end_time"`
	StartPartial   *model.CalendarParts `json:"start_partial"`
	EndPartial     *model.CalendarParts `json:"end_partial"`
	Timezone       string               `json:"timezone"`
	Precision      string               `json:"precision"`
	Scope          *string              `json:"scope"`
	Evidence       string               `json:"evidence"`
	ParentIndex    *int                 `json:"parent_index"`
	ReviewRequired bool                 `json:"review_required"`
	ReviewReasons  []string             `json:"review_reasons"`
}
type V2Response struct {
	Summary               string       `json:"summary"`
	TargetAudience        *string      `json:"target_audience"`
	SummaryReviewRequired bool         `json:"summary_review_required"`
	Schedules             []V2Schedule `json:"schedules"`
}

func DecodeV2(raw string) (V2Response, error) {
	var out V2Response
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(&out); e != nil {
		return out, fmt.Errorf("invalid v2 response JSON")
	}
	if d.Decode(new(any)) != io.EOF {
		return out, fmt.Errorf("trailing v2 JSON")
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &obj) != nil {
		return out, fmt.Errorf("v2 object required")
	}
	if e := shape(obj, map[string]string{"summary": "string", "target_audience": "nullable_string", "summary_review_required": "bool", "schedules": "array"}); e != nil {
		return out, e
	}
	var rows []map[string]json.RawMessage
	json.Unmarshal(obj["schedules"], &rows)
	for i, row := range rows {
		spec := map[string]string{"type": "string", "label": "string", "start_date": "nullable_string", "start_time": "nullable_string", "end_date": "nullable_string", "end_time": "nullable_string", "start_partial": "partial", "end_partial": "partial", "timezone": "string", "precision": "string", "scope": "nullable_string", "evidence": "string", "parent_index": "integer", "review_required": "bool", "review_reasons": "array"}
		if e := shape(row, spec); e != nil {
			return out, e
		}
		s := out.Schedules[i]
		if !oneOf(s.Type, "application", "event", "submission", "completion", "announcement", "interview", "selection") || !oneOf(s.Precision, "minute", "date", "month", "year", "month_day", "mixed", "unknown") || s.Timezone != "Asia/Seoul" {
			return out, fmt.Errorf("invalid v2 enum")
		}
		if s.ReviewReasons == nil {
			return out, fmt.Errorf("review reasons array required")
		}
		for _, f := range []string{"start_partial", "end_partial"} {
			if string(row[f]) == "null" {
				continue
			}
			var p map[string]json.RawMessage
			if json.Unmarshal(row[f], &p) != nil || shape(p, map[string]string{"year": "integer", "month": "integer", "day": "integer"}) != nil {
				return out, fmt.Errorf("invalid partial calendar object")
			}
		}
	}
	return out, nil
}
func shape(obj map[string]json.RawMessage, spec map[string]string) error {
	if len(obj) != len(spec) {
		return fmt.Errorf("missing/unexpected v2 properties")
	}
	for key, kind := range spec {
		r, ok := obj[key]
		if !ok {
			return fmt.Errorf("missing v2 property")
		}
		var v any
		if json.Unmarshal(r, &v) != nil {
			return fmt.Errorf("invalid v2 property")
		}
		if v == nil {
			if kind == "nullable_string" || kind == "integer" || kind == "partial" {
				continue
			}
			return fmt.Errorf("invalid null v2 property")
		}
		valid := false
		switch kind {
		case "string", "nullable_string":
			_, valid = v.(string)
		case "bool":
			_, valid = v.(bool)
		case "array":
			_, valid = v.([]any)
		case "partial":
			_, valid = v.(map[string]any)
		case "integer":
			n, ok := v.(float64)
			valid = ok && n == float64(int(n))
		}
		if !valid {
			return fmt.Errorf("wrong v2 property type")
		}
	}
	return nil
}
func oneOf(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
			return true
		}
	}
	return false
}

// SafeReviewedPlaceholder is deliberately narrower than calendar validation:
// uncertain proposals are retained only with NO materialized date/time values.
// It never turns an unsupported or invented datetime into a confirmed schedule.
func SafeReviewedPlaceholder(s model.Schedule) bool {
	return s.ReviewRequired && len(s.ReviewReasons) > 0 && s.Precision == "unknown" && s.StartPrecision == "unknown" && s.EndPrecision == "unknown" && s.StartDate == nil && s.EndDate == nil && s.StartTime == nil && s.EndTime == nil && s.StartPartial == nil && s.EndPartial == nil && s.StartAt == nil && s.EndAt == nil && s.Timezone == "Asia/Seoul" && oneOf(s.Type, "application", "event", "submission", "completion", "announcement", "interview", "selection")
}
func RevalidateV2(id string, raw string, doc Document) (NoticeResult, error) {
	wire, e := DecodeV2(raw)
	if e != nil {
		return NoticeResult{}, e
	}
	out := NoticeResult{ID: id, Title: doc.Title, Summary: wire.Summary, Audience: ValidateAudience(LegacyOutput{Summary: wire.Summary, TargetAudience: wire.TargetAudience, SummaryReviewRequired: wire.SummaryReviewRequired}, doc), Schedules: []model.Schedule{}, RuleSupplements: []model.Schedule{}, Diffs: []ScheduleDiff{}}
	remap := map[string]string{}
	seen := map[string]string{}
	for i, w := range wire.Schedules {
		sid := stableID(id, i)
		s := model.Schedule{ID: sid, Type: w.Type, Label: w.Label, StartDate: w.StartDate, StartTime: w.StartTime, EndDate: w.EndDate, EndTime: w.EndTime, StartPartial: w.StartPartial, EndPartial: w.EndPartial, Timezone: w.Timezone, Precision: w.Precision, Scope: w.Scope, Evidence: w.Evidence, ExtractionMethod: "ai", Origin: "saved_ai_response", ConversionMethod: "v2_direct_validation", ReviewReasons: append([]string{}, w.ReviewReasons...), Omissions: []string{}, Qualifiers: []string{}, Corrections: []model.ScheduleCorrection{}}
		if w.ReviewRequired && len(s.ReviewReasons) == 0 {
			s.ReviewReasons = append(s.ReviewReasons, "model_review_reason_missing")
		}
		if w.ParentIndex != nil {
			if *w.ParentIndex >= 0 && *w.ParentIndex < i {
				parent := stableID(id, *w.ParentIndex)
				if kept, ok := remap[parent]; ok {
					parent = kept
				}
				s.ParentID = sp(parent)
			} else {
				reason(&s, "invalid_parent")
			}
		}
		partialsNormalize(&s)
		// The model global precision is checked but boundary precision is derived.
		s.Precision = ""
		// missing_time is an omission, never uncertainty by itself.
		reasons := []string{}
		for _, r := range s.ReviewReasons {
			if r == "missing_time" {
				s.Omissions = unique(s.Omissions, r)
			} else {
				reasons = unique(reasons, r)
			}
		}
		s.ReviewReasons = reasons
		s = FromV2(s, doc)
		copy := s
		valid := ValidateSchedule(&copy, doc)
		if !valid {
			fallback := repairV2Boundaries(s, doc)
			check := fallback
			if ValidateSchedule(&check, doc) {
				fallback.ParentID = s.ParentID
				fallback.Corrections = append(fallback.Corrections, model.ScheduleCorrection{Code: "unsupported_datetime", Field: "model_calendar", Original: "see preserved raw_response"})
				for _, r := range s.ReviewReasons {
					if r != "unsupported_precision" {
						reason(&fallback, r)
					}
				}
				s = fallback
			} else {
				// Keep the proposed schedule, evidence and reasons, but materialize no
				// unverifiable calendar values. Raw response preserves the original claim.
				s.StartDate = nil
				s.EndDate = nil
				s.StartTime = nil
				s.EndTime = nil
				s.StartPartial = nil
				s.EndPartial = nil
				s.StartAt = nil
				s.EndAt = nil
				s.Precision = "unknown"
				s.StartPrecision = "unknown"
				s.EndPrecision = "unknown"
				s.ExtractionMethod = "rule"
				s.ConversionMethod = "unverified_calendar_cleared"
				reason(&s, "unsupported_datetime")
			}
		}
		sourceApproximation(&s, doc)
		context := evidenceLineContext(doc, s.EvidenceLocation)
		if strings.Contains(context, "예정") || strings.Contains(context, "추후") || strings.Contains(context, "변경 가능") {
			reason(&s, "tentative_schedule")
		}
		if (s.Type == "application" || s.Type == "selection") && (strings.Contains(doc.Content, "조기 마감") || strings.Contains(doc.Content, "조기마감")) {
			reason(&s, "conditional_deadline")
		}
		if strings.Contains(doc.Title, "겨울") && strings.Contains(doc.Content, "여름학기") {
			reason(&s, "source_conflict")
		}
		if s.Precision == "month_day" {
			reason(&s, "missing_year")
		}
		s.ReviewRequired = len(s.ReviewReasons) > 0
		key := Fingerprint(s)
		if s.Precision == "unknown" {
			key += "|" + NormalizeWhitespace(s.Evidence)
		}
		if kept, ok := seen[key]; ok {
			remap[s.ID] = kept
			out.SetIssues = append(out.SetIssues, "duplicate_schedule_removed:"+s.ID)
			continue
		}
		seen[key] = s.ID
		remap[s.ID] = s.ID
		out.Schedules = append(out.Schedules, s)
	}
	out.RuleSupplements = SupplementSessions(out.Schedules, doc)
	if issues := ValidateSet(append(append([]model.Schedule{}, out.Schedules...), out.RuleSupplements...)); len(issues) > 0 {
		return out, fmt.Errorf("v2 schedule graph invalid")
	}
	if out.SetIssues == nil {
		out.SetIssues = []string{}
	}
	return out, nil
}
