package schedules

import (
	"example.com/crawlserver/internal/model"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var fullKorean = regexp.MustCompile(`((?:19|20)[0-9]{2})\s*년\s*([0-9]{1,2})\s*월\s*([0-9]{1,2})\s*일`)
var fullNumeric = regexp.MustCompile(`((?:19|20)[0-9]{2})\s*[./-]\s*([0-9]{1,2})\s*[./-]\s*([0-9]{1,2})(?:\s*\.)?`)
var monthDayKorean = regexp.MustCompile(`([0-9]{1,2})\s*월\s*([0-9]{1,2})\s*일`)
var monthDayNumeric = regexp.MustCompile(`(?:^|[^0-9])([0-9]{1,2})\s*[./]\s*([0-9]{1,2})(?:\s*\.)?`)
var yearMonth = regexp.MustCompile(`((?:19|20)[0-9]{2})\s*년\s*([0-9]{1,2})\s*월`)
var yearMonthNumeric = regexp.MustCompile(`((?:19|20)[0-9]{2})\s*[./]\s*([0-9]{1,2})\s*월`)
var shortYearDate = regexp.MustCompile(`['’‘]([0-9]{2})\s*[./]\s*([0-9]{1,2})\s*[./]\s*([0-9]{1,2})(?:\s*\.)?`)
var shortYearMonth = regexp.MustCompile(`['’‘]([0-9]{2})\s*[./]\s*([0-9]{1,2})\s*\.`)
var onlyMonth = regexp.MustCompile(`([0-9]{1,2})\s*월`)
var onlyYear = regexp.MustCompile(`((?:19|20)[0-9]{2})\s*년`)
var colonClock = regexp.MustCompile(`([0-9]{1,2})\s*:\s*([0-9]{2})(?::([0-9]{2}))?`)
var hourClock = regexp.MustCompile(`([0-9]{1,2})\s*시`)
var hourRange = regexp.MustCompile(`([0-9]{1,2})\s*[~～〜–-]\s*([0-9]{1,2})\s*시`)

type dateToken struct {
	parts      model.CalendarParts
	start, end int
	context    bool
}
type clockToken struct {
	value      string
	start, end int
}

func ip(v int) *int               { return &v }
func sp(v string) *string         { return &v }
func num(s string) int            { n, _ := strconv.Atoi(s); return n }
func overlap(a, b, c, d int) bool { return a < d && c < b }
func dates(s string) []dateToken {
	found := []dateToken{}
	add := func(re *regexp.Regexp, kind string) {
		for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
			start, end := m[0], m[1]
			a := num(s[m[2]:m[3]])
			parts := model.CalendarParts{}
			switch kind {
			case "full":
				parts.Year = ip(a)
				parts.Month = ip(num(s[m[4]:m[5]]))
				parts.Day = ip(num(s[m[6]:m[7]]))
			case "month_day":
				start = m[2]
				parts.Month = ip(a)
				parts.Day = ip(num(s[m[4]:m[5]]))
			case "month":
				parts.Year = ip(a)
				parts.Month = ip(num(s[m[4]:m[5]]))
			case "year":
				parts.Year = ip(a)
			case "short_full":
				parts.Month = ip(num(s[m[4]:m[5]]))
				parts.Day = ip(num(s[m[6]:m[7]]))
			case "short_month":
				parts.Month = ip(num(s[m[4]:m[5]]))
			case "only_month":
				parts.Month = ip(a)
			}
			used := false
			for _, t := range found {
				if overlap(start, end, t.start, t.end) {
					used = true
					break
				}
			}
			if used {
				continue
			}
			if parts.Month != nil && (*parts.Month < 1 || *parts.Month > 12) {
				continue
			}
			if parts.Day != nil && (*parts.Day < 1 || *parts.Day > 31) {
				continue
			}
			if parts.Year != nil && parts.Month != nil && parts.Day != nil {
				if !validDate(*parts.Year, *parts.Month, *parts.Day) {
					continue
				}
			}
			found = append(found, dateToken{parts, start, end, false})
		}
	}
	add(fullKorean, "full")
	add(fullNumeric, "full")
	add(shortYearDate, "short_full")
	add(yearMonthNumeric, "month")
	add(shortYearMonth, "short_month")
	add(monthDayKorean, "month_day")
	add(monthDayNumeric, "month_day")
	add(yearMonth, "month")
	add(onlyYear, "year")
	add(onlyMonth, "only_month")
	for i := 0; i < len(found); i++ {
		for j := i + 1; j < len(found); j++ {
			if found[j].start < found[i].start {
				found[i], found[j] = found[j], found[i]
			}
		}
	}
	// An explicitly abbreviated day ending in a weekday/day marker shares
	// the preceding range's month, never an invented year from publication.
	if len(found) == 1 && found[0].parts.Month != nil && found[0].parts.Day != nil {
		tail := s[found[0].end:]
		re := regexp.MustCompile(`^[^~～〜–]*[~～〜–]\s*([0-9]{1,2})\s*(?:일|\([월화수목금토일]\))`)
		if m := re.FindStringSubmatchIndex(tail); m != nil {
			day := num(tail[m[2]:m[3]])
			if day >= 1 && day <= 31 {
				found = append(found, dateToken{parts: model.CalendarParts{Month: ip(*found[0].parts.Month), Day: ip(day)}, start: found[0].end + m[2], end: found[0].end + m[1]})
			}
		}
	}
	// Only inherit within an explicit, adjacent range whose first date has a year.
	// Never infer a year from publication/academic-year titles or the current date.
	if len(found) == 2 && found[0].parts.Year != nil && found[0].parts.Month != nil && found[0].parts.Day != nil && found[1].parts.Year == nil && hasRange(s[found[0].end:found[1].start]) {
		y := *found[0].parts.Year
		if found[0].parts.Month != nil && found[1].parts.Month != nil && *found[1].parts.Month < *found[0].parts.Month {
			return found
		}
		found[1].parts.Year = ip(y)
		found[1].context = true
	}
	return found
}
func validDate(y, m, d int) bool {
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	return t.Year() == y && int(t.Month()) == m && t.Day() == d
}
func hasRange(s string) bool {
	return strings.ContainsAny(s, "~～〜–") || strings.Contains(s, "부터") || strings.TrimSpace(s) == "-"
}
func meridiem(h int, prefix string) int {
	am := strings.LastIndex(prefix, "오전")
	pm := strings.LastIndex(prefix, "오후")
	night := strings.LastIndex(prefix, "저녁")
	if night > pm {
		pm = night
	}
	if pm > am && h < 12 {
		return h + 12
	}
	if am > pm && am >= 0 && h == 12 {
		return 0
	}
	return h
}
func clocks(s string, ds []dateToken) []clockToken {
	out := []clockToken{}
	dateOverlap := func(start, end int) bool {
		for _, d := range ds {
			if overlap(start, end, d.start, d.end) {
				return true
			}
		}
		return false
	}
	add := func(start, end, h, min int) {
		if h < 0 || h > 23 || min < 0 || min > 59 || dateOverlap(start, end) {
			return
		}
		out = append(out, clockToken{fmt.Sprintf("%02d:%02d", h, min), start, end})
	}
	for _, m := range colonClock.FindAllStringSubmatchIndex(s, -1) {
		if m[6] >= 0 && num(s[m[6]:m[7]]) != 0 {
			continue
		}
		add(m[0], m[1], meridiem(num(s[m[2]:m[3]]), s[:m[0]]), num(s[m[4]:m[5]]))
	}
	for _, m := range hourRange.FindAllStringSubmatchIndex(s, -1) {
		for _, idx := range []int{2, 4} {
			add(m[idx], m[idx+1], meridiem(num(s[m[idx]:m[idx+1]]), s[:m[0]]), 0)
		}
	}
	for _, m := range hourClock.FindAllStringSubmatchIndex(s, -1) {
		if strings.HasPrefix(s[m[1]:], "간") {
			continue
		}
		used := false
		for _, c := range out {
			if overlap(m[2], m[3], c.start, c.end) {
				used = true
			}
		}
		if !used {
			add(m[0], m[1], meridiem(num(s[m[2]:m[3]]), s[:m[0]]), 0)
		}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].start < out[i].start {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
func setBoundary(s *model.Schedule, start bool, d dateToken, clock *string) {
	var date *string
	partial := &d.parts
	precision := "unknown"
	if d.parts.Year != nil && d.parts.Month != nil && d.parts.Day != nil {
		if validDate(*d.parts.Year, *d.parts.Month, *d.parts.Day) {
			date = sp(fmt.Sprintf("%04d-%02d-%02d", *d.parts.Year, *d.parts.Month, *d.parts.Day))
			partial = nil
			precision = "date"
		}
	} else if d.parts.Year != nil && d.parts.Month != nil {
		precision = "month"
	} else if d.parts.Month != nil && d.parts.Day == nil {
		precision = "month"
	} else if d.parts.Year != nil {
		precision = "year"
	} else if d.parts.Month != nil && d.parts.Day != nil {
		precision = "month_day"
	}
	if date != nil && clock != nil {
		precision = "minute"
	}
	if d.parts.Year == nil {
		reason(s, "missing_year")
	}
	if clock == nil {
		s.Omissions = unique(s.Omissions, "missing_time")
	}
	if d.context {
		s.DateContext = "explicit_range_year"
	}
	if start {
		s.StartDate = date
		s.StartPartial = partial
		s.StartTime = clock
		s.StartPrecision = precision
	} else {
		s.EndDate = date
		s.EndPartial = partial
		s.EndTime = clock
		s.EndPrecision = precision
	}
}
func unique(xs []string, x string) []string {
	for _, v := range xs {
		if v == x {
			return xs
		}
	}
	return append(xs, x)
}
func reason(s *model.Schedule, r string) {
	s.ReviewReasons = unique(s.ReviewReasons, r)
	s.ReviewRequired = true
}

// Invalid explicit clocks are a source problem, not mere time omission.
func unsupportedClockInSource(s string) bool {
	for _, m := range colonClock.FindAllStringSubmatchIndex(s, -1) {
		h := meridiem(num(s[m[2]:m[3]]), s[:m[0]])
		minute := num(s[m[4]:m[5]])
		if h > 23 || minute > 59 || (m[6] >= 0 && num(s[m[6]:m[7]]) != 0) {
			return true
		}
	}
	for _, m := range hourClock.FindAllStringSubmatchIndex(s, -1) {
		if strings.HasPrefix(s[m[1]:], "간") {
			continue
		}
		if meridiem(num(s[m[2]:m[3]]), s[:m[0]]) > 23 {
			return true
		}
	}
	return false
}

// Do not downgrade an invalid explicit full date to a seemingly valid year.
func unsupportedCalendarInSource(s string) bool {
	for _, re := range []*regexp.Regexp{fullKorean, fullNumeric} {
		for _, m := range re.FindAllStringSubmatch(s, -1) {
			if !validDate(num(m[1]), num(m[2]), num(m[3])) {
				return true
			}
		}
	}
	for _, m := range yearMonth.FindAllStringSubmatch(s, -1) {
		if num(m[2]) < 1 || num(m[2]) > 12 {
			return true
		}
	}
	return false
}
