package imageanalysis

import (
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/noticepolicy"
	"example.com/crawlserver/internal/schedules"
	"fmt"
	"reflect"
	"strings"
)

type MergeResult struct {
	Schedules    []model.Schedule `json:"schedules"`
	Added        int              `json:"added"`
	Matched      int              `json:"matched"`
	Complemented int              `json:"complemented"`
	Conflicts    int              `json:"conflicts"`
	Issues       []string         `json:"issues"`
}

func normalized(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), "") }
func sameScope(a, b model.Schedule) bool {
	if (a.Scope == nil) != (b.Scope == nil) {
		return false
	}
	if a.Type != b.Type {
		return false
	}
	if a.Scope != nil && b.Scope != nil && normalized(*a.Scope) != normalized(*b.Scope) {
		return false
	}
	// A shared audience/institution alone does not make different events identical.
	return normalized(a.Label) == normalized(b.Label)
}
func mismatch(a, b *string) bool { return a != nil && b != nil && *a != *b }
func partialMismatch(p *model.CalendarParts, date *string) bool {
	if p == nil || date == nil {
		return false
	}
	var y, m, d int
	if _, e := fmt.Sscanf(*date, "%d-%d-%d", &y, &m, &d); e != nil {
		return true
	}
	return p.Year != nil && *p.Year != y || p.Month != nil && *p.Month != m || p.Day != nil && *p.Day != d
}
func partialConflict(a, b *model.CalendarParts) bool {
	if a == nil || b == nil {
		return false
	}
	for i, x := range []*int{a.Year, a.Month, a.Day} {
		y := []*int{b.Year, b.Month, b.Day}[i]
		if x != nil && y != nil && *x != *y {
			return true
		}
	}
	return false
}
func conflicts(a, b model.Schedule) bool {
	return partialConflict(a.StartPartial, b.StartPartial) || partialConflict(a.EndPartial, b.EndPartial) || mismatch(a.StartDate, b.StartDate) || mismatch(a.EndDate, b.EndDate) || mismatch(a.StartTime, b.StartTime) || mismatch(a.EndTime, b.EndTime) || partialMismatch(a.StartPartial, b.StartDate) || partialMismatch(a.EndPartial, b.EndDate) || partialMismatch(b.StartPartial, a.StartDate) || partialMismatch(b.EndPartial, a.EndDate)
}
func sameCalendar(a, b model.Schedule) bool {
	return reflect.DeepEqual(a.StartDate, b.StartDate) && reflect.DeepEqual(a.EndDate, b.EndDate) && reflect.DeepEqual(a.StartTime, b.StartTime) && reflect.DeepEqual(a.EndTime, b.EndTime) && reflect.DeepEqual(a.StartPartial, b.StartPartial) && reflect.DeepEqual(a.EndPartial, b.EndPartial)
}
func flag(s *model.Schedule, why string) {
	s.ReviewRequired = true
	s.ReviewReasons = appendUnique(s.ReviewReasons, why)
	s.Quality = schedules.DeriveQuality(*s, nil)
}
func Merge(text, images []model.Schedule) MergeResult {
	b, _ := json.Marshal(text)
	var originals []model.Schedule
	json.Unmarshal(b, &originals)
	r := MergeResult{Schedules: originals, Issues: []string{}}
	if r.Schedules == nil {
		r.Schedules = []model.Schedule{}
	}
	aliases := map[string]string{}
	for _, im := range images {
		matched := false
		for i := range r.Schedules {
			old := &r.Schedules[i]
			if !sameScope(*old, im) {
				continue
			}
			if conflicts(*old, im) {
				flag(old, "text_image_conflict")
				flag(&im, "text_image_conflict")
				r.Conflicts++
				r.Issues = appendUnique(r.Issues, "text_image_conflict")
				break
			}
			// Matching identity and compatible calendar: add evidence, never overwrite
			// an already-known text value, and retain review on OCR-derived additions.
			before := *old
			for j, p := range []**string{&old.StartDate, &old.EndDate, &old.StartTime, &old.EndTime} {
				q := []*string{im.StartDate, im.EndDate, im.StartTime, im.EndTime}[j]
				if *p == nil && q != nil {
					*p = q
				}
			}
			if old.StartDate != nil {
				old.StartPartial = nil
			}
			if old.EndDate != nil {
				old.EndPartial = nil
			}
			if old.StartPartial == nil && old.StartDate == nil {
				old.StartPartial = im.StartPartial
			}
			if old.EndPartial == nil && old.EndDate == nil {
				old.EndPartial = im.EndPartial
			}
			old.ImageEvidence = append(old.ImageEvidence, im.ImageEvidence...)
			old.Origin = "merged_text_image"
			old.ExtractionMethod = "ai_text_plus_image"
			if before.Origin == "image_ai" {
				old.Origin = "image_ai"
				old.ExtractionMethod = "ai_multimodal"
			}
			aliases[im.ID] = old.ID
			old.ConversionMethod = Version
			if sameCalendar(before, *old) {
				r.Matched++
			} else {
				r.Complemented++
			}
			for _, reason := range im.ReviewReasons {
				old.ReviewReasons = appendUnique(old.ReviewReasons, reason)
			}
			for _, q := range im.Qualifiers {
				old.Qualifiers = appendUnique(old.Qualifiers, q)
			}
			flag(old, "image_evidence_not_independently_verified")
			noticepolicy.Refresh(old)
			matched = true
			break
		}
		if !matched {
			exists := false
			for _, s := range r.Schedules {
				if s.ID == im.ID {
					exists = true
				}
			}
			if !exists {
				r.Schedules = append(r.Schedules, im)
				r.Added++
			}
		}
	}
	ids := map[string]bool{}
	for _, s := range r.Schedules {
		ids[s.ID] = true
	}
	for i := range r.Schedules {
		if parent := r.Schedules[i].ParentID; parent != nil {
			if replacement, ok := aliases[*parent]; ok {
				r.Schedules[i].ParentID = &replacement
			}
			if !ids[*r.Schedules[i].ParentID] {
				flag(&r.Schedules[i], "invalid_image_parent")
			}
		}
	}
	return r
}
