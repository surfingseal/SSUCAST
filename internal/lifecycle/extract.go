// Package lifecycle uses deterministic local evidence, never AI/OCR/network I/O.
package lifecycle

import (
	"crypto/sha256"
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const Version = "deadline-evidence-v1"

var Seoul = func() *time.Location {
	l, e := time.LoadLocation("Asia/Seoul")
	if e != nil {
		return time.FixedZone("Asia/Seoul", 9*3600)
	}
	return l
}()
var dates = regexp.MustCompile(`(^|[^0-9])(?:(\d{4})\s*(?:[./-]|년)\s*)?(\d{1,2})\s*(?:[./-]|월)\s*(\d{1,2})\s*(?:일|\.)?`)
var clocks = regexp.MustCompile(`(오전|오후|저녁)?\s*(\d{1,2})\s*(?::\s*(\d{2})|시(?:\s*(\d{1,2})\s*분)?)(?:까지|마감|종료|정각)?(?:$|[^가-힣0-9])`)
var due = regexp.MustCompile(`(신청|접수|지원|모집|설문|이수|제출|등록|참여|수강신청).{0,12}(기간|마감|기한|종료|까지)`)
var action = regexp.MustCompile(`신청|접수|모집|설문|이수|제출|참가자|지원기간|지원마감|장학생.{0,8}선발`)
var specific = regexp.MustCompile(`마감|기한|까지|[~∼～]`)
var event = regexp.MustCompile(`^(?:[0-9가-힣.○□★▶\s-]{0,6})(?:행사일|행사기간|개최일|개최기간|발표일|발표일자|선발결과|수업일|교육일|교육기간|실습기간|활동기간|운영기간|지급일|사업기간)`)
var uncertain = regexp.MustCompile(`조기.{0,6}마감|선착순|상시|예산.{0,8}소진|추후.{0,8}안내|별도.{0,8}공지|기한.{0,5}미정`)
var reference = regexp.MustCompile(`마감일.{0,35}기준|유효기간|신청.{0,12}완료한.{0,4}자|신청기간.{0,10}내에만`)
var changed = regexp.MustCompile(`변경\s*(후|마감|기한|기간)?\s*[:：]|연장\s*(후|마감|기한|기간)?\s*[:：]|→|=>`)

func InputHash(v model.Content) string {
	images := append([]string{}, v.ImageURLs...)
	sort.Strings(images)
	b, _ := json.Marshal([]any{Version, v.Title, v.Content, v.PublishedAt, images})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func EffectiveStatus(v model.Content, now time.Time) string {
	if v.Lifecycle != nil && v.Lifecycle.Confirmed && v.Deadline != nil && v.Lifecycle.Version == Version && v.Lifecycle.InputHash == InputHash(v) && v.Lifecycle.Deadline != nil && v.Deadline.Equal(*v.Lifecycle.Deadline) {
		if now.After(*v.Deadline) {
			return "expired"
		}
		return "active"
	}
	if v.LifecycleStatus == "active" || v.LifecycleStatus == "expired" {
		return "needs_review"
	}
	return v.LifecycleStatus
}

type token struct{ year, month, day, start, end int }

func number(s string) int { n, _ := strconv.Atoi(s); return n }
func tokens(s string) []token {
	out := []token{}
	for _, m := range dates.FindAllStringSubmatchIndex(s, -1) {
		value := func(i int) string {
			if m[i] < 0 {
				return ""
			}
			return s[m[i]:m[i+1]]
		}
		out = append(out, token{number(value(4)), number(value(6)), number(value(8)), m[0] + len(value(2)), m[1]})
	}
	return out
}
func day(y, m, d int) (time.Time, bool) {
	if y < 1900 || y > 2200 || m < 1 || m > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, Seoul)
	return t, t.Year() == y && int(t.Month()) == m && t.Day() == d
}
func candidate(text, source string, published *time.Time) (model.DeadlineEvidence, string) {
	e := model.DeadlineEvidence{Source: source, Sentence: text}
	if reference.MatchString(text) {
		return e, "reference_date_not_deadline"
	}
	if uncertain.MatchString(text) {
		return e, "conditional_or_rolling_deadline"
	}
	work := text
	eventLabels := regexp.MustCompile(`행사일시|행사일|개최일|발표일|수업일|교육일|교육기간|실습기간|활동기간|운영기간|지급일`)
	if loc := due.FindStringIndex(work); loc != nil {
		if eventLabels.MatchString(work[:loc[0]]) {
			work = work[loc[0]:]
		}
	}
	if loc := eventLabels.FindStringIndex(work); loc != nil && len(tokens(work[:loc[0]])) > 0 {
		work = work[:loc[0]]
	}
	if m := changed.FindStringIndex(text); m != nil {
		work = text[m[1]:]
		e.Extension = true
	}
	ts := tokens(work)
	if len(ts) == 0 {
		return e, "no_parseable_deadline_date"
	}
	if len(ts) > 2 {
		return e, "multiple_dates_in_one_clause"
	}
	if len(ts) == 2 && !strings.ContainsAny(work[ts[0].end:ts[1].start], "~∼～–—-") {
		return e, "multiple_dates_without_range"
	}
	if len(ts) == 1 && !specific.MatchString(text) {
		return e, "period_end_not_explicit"
	}
	last := ts[len(ts)-1]
	year := last.year
	e.YearPolicy = "explicit_year"
	if year == 0 && len(ts) == 2 && ts[0].year != 0 {
		year = ts[0].year
		if last.month < ts[0].month {
			year++
		}
		e.YearPolicy = "range_start_year"
	}
	if year == 0 {
		if published == nil {
			return e, "missing_year_and_publication"
		}
		pub := published.In(Seoul)
		year = pub.Year()
		e.YearPolicy = "published_year_nearby_same_year"
		inferred, ok := day(year, last.month, last.day)
		pubDay := time.Date(pub.Year(), pub.Month(), pub.Day(), 0, 0, 0, 0, Seoul)
		if !ok || inferred.Before(pubDay) || inferred.After(pubDay.AddDate(0, 3, 0)) {
			return e, "ambiguous_omitted_year"
		}
		if len(ts) == 2 && ts[0].month > last.month {
			return e, "ambiguous_year_rollover"
		}
	}
	end, ok := day(year, last.month, last.day)
	if !ok {
		return e, "invalid_calendar_date"
	}
	if len(ts) == 2 {
		first := ts[0]
		startYear := first.year
		if startYear == 0 {
			startYear = year
			if first.month > last.month {
				startYear--
			}
		}
		start, ok := day(startYear, first.month, first.day)
		if !ok || start.After(end) {
			return e, "invalid_date_range"
		}
	}
	tail := work[last.end:]
	if regexp.MustCompile(`:[0-9]{2}:[0-9]{2}`).MatchString(tail) {
		return e, "unsupported_second_precision"
	}
	times := clocks.FindAllStringSubmatch(tail, -1)
	if len(times) == 0 && regexp.MustCompile(`[0-9]{1,2}\s*[:시]`).MatchString(tail) {
		return e, "unsupported_or_ambiguous_clock"
	}
	if len(times) > 0 {
		if len(times) > 1 && !strings.ContainsAny(tail, "~∼～–—-") {
			return e, "multiple_end_times"
		}
		clock := times[len(times)-1]
		hour := number(clock[2])
		minute := number(clock[3])
		if clock[3] == "" {
			minute = number(clock[4])
		}
		if clock[1] == "오후" || clock[1] == "저녁" {
			if hour < 12 {
				hour += 12
			}
		} else if clock[1] == "오전" && hour == 12 {
			hour = 0
		}
		if clock[1] != "" && number(clock[2]) > 12 {
			return e, "invalid_am_pm_clock"
		}
		if clock[1] == "" && clock[3] == "" && hour > 0 && hour < 12 {
			return e, "ambiguous_am_pm"
		}
		if hour > 24 || minute > 59 || hour == 24 && minute != 0 {
			return e, "invalid_clock_time"
		}
		end = end.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
		e.TimePolicy = "explicit_kst_time_inclusive"
	} else {
		end = end.AddDate(0, 0, 1).Add(-time.Millisecond)
		e.TimePolicy = "date_only_end_of_day_kst_23_59_59_999"
	}
	if published != nil {
		pub := published.In(Seoul)
		pubDay := time.Date(pub.Year(), pub.Month(), pub.Day(), 0, 0, 0, 0, Seoul)
		if end.Before(pubDay) {
			return e, "deadline_before_publication"
		}
	}
	utc := end.UTC()
	e.Deadline = &utc
	return e, ""
}
func Extract(v model.Content, now time.Time) model.LifecycleEvaluation {
	r := model.LifecycleEvaluation{Version: Version, InputHash: InputHash(v), ContentHash: v.ContentHash, CheckedAt: now.UTC(), Status: "needs_review", Evidence: []model.DeadlineEvidence{}, Reasons: []string{}}
	add := func(s string) {
		for _, x := range r.Reasons {
			if x == s {
				return
			}
		}
		r.Reasons = append(r.Reasons, s)
	}
	if strings.TrimSpace(v.Content) == "" {
		add("body_unavailable_or_image_only")
		r.Evidence = append(r.Evidence, model.DeadlineEvidence{Source: "title", Sentence: v.Title})
		return r
	}
	sources := []struct{ name, text string }{{"title", v.Title}, {"content", v.Content}}
	foundContext := false
	blocking := false
	for _, source := range sources {
		lines := strings.Split(strings.ReplaceAll(source.text, "\r\n", "\n"), "\n")
		for i, line := range lines {
			line = model.NormalizeText(line)
			if line == "" {
				continue
			}
			relevant := due.MatchString(line) || (action.MatchString(line) && strings.Contains(line, "까지")) || (source.name == "title" && action.MatchString(line) && strings.ContainsAny(line, "~∼～"))
			if !relevant || event.MatchString(line) && !due.MatchString(line) {
				continue
			}
			foundContext = true
			if regexp.MustCompile(`대학별|기관별|차수별|단계별|대상별`).MatchString(line) {
				blocking = true
				add("multiple_application_schedules")
			}

			if len(tokens(line)) == 0 && i+1 < len(lines) {
				next := model.NormalizeText(lines[i+1])
				if len(tokens(next)) > 0 && !event.MatchString(next) && !due.MatchString(next) {
					line += "\n" + next
				}
			}
			e, reason := candidate(line, source.name, v.PublishedAt)
			r.Evidence = append(r.Evidence, e)
			if e.Deadline == nil && reason != "no_parseable_deadline_date" {
				blocking = true
				add(reason)
			}
		}
	}
	if uncertain.MatchString(v.Title+"\n"+v.Content) && action.MatchString(v.Title+"\n"+v.Content) {
		blocking = true
		add("conditional_or_rolling_deadline")
	}
	valid := map[string]model.DeadlineEvidence{}
	extensions := map[string]model.DeadlineEvidence{}
	for _, e := range r.Evidence {
		if e.Deadline != nil {
			key := e.Deadline.Format(time.RFC3339Nano)
			valid[key] = e
			if e.Extension {
				extensions[key] = e
			}
		}
	}
	if len(valid) > 1 {
		days := map[string]bool{}
		precise := map[string]model.DeadlineEvidence{}
		for k, e := range valid {
			days[e.Deadline.In(Seoul).Format("2006-01-02")] = true
			if e.TimePolicy == "explicit_kst_time_inclusive" {
				precise[k] = e
			}
		}
		if len(days) == 1 && len(precise) == 1 {
			valid = precise
		}
	}
	var chosen *time.Time
	if len(extensions) == 1 && !blocking {
		for _, e := range extensions {
			x := *e.Deadline
			chosen = &x
		}
		for _, e := range valid {
			if !e.Deadline.Equal(*chosen) && (!regexp.MustCompile(`기존|변경\s*전|종전`).MatchString(e.Sentence) || e.Deadline.After(*chosen)) {
				chosen = nil
				add("extension_conflicts_with_other_deadline")
				break
			}
		}
	} else if len(valid) == 1 && !blocking {
		for _, e := range valid {
			x := *e.Deadline
			chosen = &x
		}
	} else if len(valid) > 1 {
		add("multiple_distinct_deadlines")
	}
	if chosen != nil {
		r.Deadline = chosen
		r.Confirmed = true
		r.Status = "active"
		if now.After(*chosen) {
			r.Status = "expired"
		}
		return r
	}
	if len(valid) == 0 && !blocking && !foundContext && !action.MatchString(v.Title+"\n"+v.Content) {
		r.Status = "no_deadline"
		add("no_application_or_completion_deadline_context")
	} else {
		add("deadline_not_uniquely_confirmed")
	}
	if len(r.Evidence) == 0 {
		r.Evidence = append(r.Evidence, model.DeadlineEvidence{Source: "title", Sentence: v.Title})
	}
	sort.Strings(r.Reasons)
	return r
}
