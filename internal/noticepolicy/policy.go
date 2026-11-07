// Package noticepolicy applies an explicit user year assumption while keeping
// immutable AI evidence and analysis fingerprints unchanged.
package noticepolicy

import (
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/schedules"
	"fmt"
	"strings"
	"time"
)

const Version = "current-year-user-policy-v1"

var Seoul = time.FixedZone("Asia/Seoul", 9*3600)

func Valid(v model.Content) bool {
	p := v.NoticePolicy
	return p != nil && p.Version == Version && p.ContentHash == v.ContentHash && p.InputHash == v.ScheduleInputHash && p.AnalysisID == v.ScheduleAnalysisID
}
func Effective(v model.Content) []model.Schedule {
	if Valid(v) {
		return v.NoticePolicy.Schedules
	}
	return v.Schedules
}
func Refresh(s *model.Schedule) {
	precision := func(d, t *string, p *model.CalendarParts) string {
		if d != nil {
			if t != nil {
				return "datetime"
			}
			return "date"
		}
		if p != nil {
			if p.Day != nil {
				return "month_day"
			}
			if p.Month != nil {
				return "month"
			}
			return "year"
		}
		return "unknown"
	}
	s.StartPrecision = precision(s.StartDate, s.StartTime, s.StartPartial)
	s.EndPrecision = precision(s.EndDate, s.EndTime, s.EndPartial)
	s.Precision = s.StartPrecision
	if s.Precision == "unknown" {
		s.Precision = s.EndPrecision
	} else if s.EndPrecision != "unknown" && s.Precision != s.EndPrecision {
		s.Precision = "mixed"
	}
	compat := func(d, t *string) *string {
		if d == nil || t == nil {
			return nil
		}
		x := *d + "T" + *t + ":00+09:00"
		return &x
	}
	s.StartAt = compat(s.StartDate, s.StartTime)
	s.EndAt = compat(s.EndDate, s.EndTime)
}
func Project(v model.Content, year int, now time.Time) (model.NoticePolicy, error) {
	if year < 2000 || year > 2100 {
		return model.NoticePolicy{}, fmt.Errorf("invalid policy year")
	}
	b, _ := json.Marshal(v.Schedules)
	var rows []model.Schedule
	if json.Unmarshal(b, &rows) != nil {
		return model.NoticePolicy{}, fmt.Errorf("invalid schedules")
	}
	p := model.NoticePolicy{Version: Version, Year: year, YearBasis: "explicit_user_current_year_assumption", ContentHash: v.ContentHash, InputHash: v.ScheduleInputHash, AnalysisID: v.ScheduleAnalysisID, AppliedAt: now.UTC(), Schedules: rows, ExpiryReason: "no_confirmed_application_deadline"}
	for i := range rows {
		s := &rows[i]
		filled := 0
		for j, partial := range []*model.CalendarParts{s.StartPartial, s.EndPartial} {
			if partial == nil || partial.Year != nil {
				continue
			}
			copy := *partial
			y := year
			copy.Year = &y
			filled++
			if j == 0 {
				s.StartPartial = &copy
			} else {
				s.EndPartial = &copy
			}
			if copy.Month != nil && copy.Day != nil {
				date := fmt.Sprintf("%04d-%02d-%02d", year, *copy.Month, *copy.Day)
				if _, e := time.Parse("2006-01-02", date); e != nil {
					if j == 0 {
						s.StartPartial = partial
					} else {
						s.EndPartial = partial
					}
					filled--
					s.ReviewReasons = append(s.ReviewReasons, "invalid_calendar_value")
					s.ReviewRequired = true
					s.Quality = schedules.DeriveQuality(*s, nil)
					continue
				}
				if j == 0 && s.StartDate == nil {
					s.StartDate = &date
					s.StartPartial = nil

				} else if j == 1 && s.EndDate == nil {
					s.EndDate = &date
					s.EndPartial = nil

				}
			}
		}
		if filled > 0 {
			p.BoundariesFilled += filled
			s.Corrections = append(s.Corrections, model.ScheduleCorrection{Code: "year_assumed_user_policy", Field: "year", Original: "missing_year"})
			s.ConversionMethod = Version
			hadYearReason := false
			kept := []string{}
			for _, r := range s.ReviewReasons {
				if r == "missing_year" {
					hadYearReason = true
				}
				if r != "missing_year" {
					kept = append(kept, r)
				}
			}
			s.ReviewReasons = kept
			s.ReviewRequired = len(kept) > 0 || (s.ReviewRequired && !hadYearReason)
			if s.StartDate != nil && s.EndDate != nil && *s.StartDate > *s.EndDate {
				s.ReviewRequired = true
				s.ReviewReasons = append(s.ReviewReasons, "reversed_range")
			}
			Refresh(s)
			s.Quality = schedules.DeriveQuality(*s, nil)
		}
	}
	p.Schedules = rows
	// Every application/submission/completion scope must have a reliable end.
	// A known early round cannot hide another unresolved or future round.
	count := 0
	blocked := false
	var latest time.Time
	for _, s := range rows {
		if s.Type != "application" && s.Type != "submission" && s.Type != "completion" {
			continue
		}
		count++
		if s.ReviewRequired || s.Quality == nil || (s.Quality.State != "verified" && s.Quality.State != "partial") || s.EndDate == nil || s.Timezone != "Asia/Seoul" {
			blocked = true
			continue
		}
		end, e := time.ParseInLocation("2006-01-02", *s.EndDate, Seoul)
		if e != nil {
			blocked = true
			continue
		}
		if s.EndTime == nil {
			end = end.AddDate(0, 0, 1)
		} else {
			end, e = time.ParseInLocation("2006-01-02 15:04", *s.EndDate+" "+*s.EndTime, Seoul)
			if e != nil {
				blocked = true
				continue
			}
		}
		if end.After(latest) {
			latest = end
		}
	}
	if count > 0 && !blocked && !latest.IsZero() {
		utc := latest.UTC()
		p.ExpiresAt = &utc
		p.ExpiryReason = "all_application_scopes_confirmed; date_only_valid_until_next_day_Asia_Seoul"
	} else if blocked {
		p.ExpiryReason = "unresolved_application_scope_kept_visible"
	}
	return p, nil
}
func IsYearAssumed(s model.Schedule) bool {
	for _, c := range s.Corrections {
		if strings.HasPrefix(c.Code, "year_assumed_user_policy") {
			return true
		}
	}
	return false
}
