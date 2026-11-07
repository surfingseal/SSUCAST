// Package notices contains the read-only student-facing contract. AI/raw and
// repository credentials are intentionally absent from these response types.
package notices

import (
	"example.com/crawlserver/internal/model"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var Seoul = time.FixedZone("Asia/Seoul", 9*3600)
var CategoryLabels = map[string]string{"academic": "학사", "scholarship": "장학", "international": "국제교류", "international_student": "외국인 유학생", "extracurricular": "비교과·행사", "teaching": "교직", "volunteer": "봉사", "other": "기타", "unknown": "분류 확인 필요"}

type Filter struct {
	Query, Source, Category, Status, Sort string
	HasSchedule, Upcoming                 *bool
	Page, Limit                           int
}

func Parse(q url.Values) (Filter, error) {
	f := Filter{Page: 1, Limit: 12, Sort: "latest"}
	allowed := map[string]bool{"query": true, "source": true, "category": true, "status": true, "sort": true, "has_schedule": true, "upcoming": true, "page": true, "limit": true}
	for k, v := range q {
		if !allowed[k] || len(v) != 1 {
			return f, fmt.Errorf("알 수 없거나 중복된 검색 조건입니다")
		}
	}
	var e error
	for k, p := range map[string]*int{"page": &f.Page, "limit": &f.Limit} {
		if s := q.Get(k); s != "" {
			*p, e = strconv.Atoi(s)
			if e != nil {
				return f, fmt.Errorf("페이지 값이 올바르지 않습니다")
			}
		}
	}
	if f.Page < 1 || f.Page > 10000 || f.Limit < 1 || f.Limit > 50 {
		return f, fmt.Errorf("page는1~10000, limit은1~50입니다")
	}
	f.Query = strings.TrimSpace(q.Get("query"))
	if !utf8.ValidString(f.Query) || utf8.RuneCountInString(f.Query) > 100 {
		return f, fmt.Errorf("검색어는100자 이내로 입력해 주세요")
	}
	for _, r := range f.Query {
		if r < ' ' {
			return f, fmt.Errorf("검색어에 제어 문자를 사용할 수 없습니다")
		}
	}
	f.Source = q.Get("source")
	if len(f.Source) > 80 {
		return f, fmt.Errorf("출처가 올바르지 않습니다")
	}
	for _, r := range f.Source {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return f, fmt.Errorf("출처가 올바르지 않습니다")
		}
	}
	f.Category = q.Get("category")
	if f.Category != "" && CategoryLabels[f.Category] == "" {
		return f, fmt.Errorf("카테고리가 올바르지 않습니다")
	}
	f.Status = q.Get("status")
	if f.Status != "" && f.Status != "진행" && f.Status != "unknown" && f.Status != "완료" {
		return f, fmt.Errorf("공고 상태가 올바르지 않습니다")
	}
	if v := q.Get("sort"); v != "" {
		f.Sort = v
	}
	if f.Sort != "latest" && f.Sort != "deadline" && f.Sort != "event" {
		return f, fmt.Errorf("정렬 방식이 올바르지 않습니다")
	}
	for k, p := range map[string]**bool{"has_schedule": &f.HasSchedule, "upcoming": &f.Upcoming} {
		if s, ok := q[k]; ok {
			if s[0] != "true" && s[0] != "false" {
				return f, fmt.Errorf("일정 조건은true/false입니다")
			}
			v := s[0] == "true"
			*p = &v
		}
	}
	return f, nil
}

type Source struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}
type Metadata struct {
	Sources      []Source   `json:"sources"`
	UpdatedAt    *time.Time `json:"updated_at"`
	StoredCount  int64      `json:"stored_count"`
	VisibleCount int64      `json:"visible_count"`
}
type Schedule struct {
	ID                 string               `json:"id"`
	ParentID           *string              `json:"parent_id"`
	Type               string               `json:"type"`
	Label              string               `json:"label"`
	StartDate, EndDate *string              `json:"-"`
	StartTime, EndTime *string              `json:"-"`
	StartPartial       *model.CalendarParts `json:"start_partial"`
	EndPartial         *model.CalendarParts `json:"end_partial"`
	Timezone           string               `json:"timezone"`
	Precision          string               `json:"precision"`
	Scope              *string              `json:"scope"`
	Evidence           string               `json:"evidence"`
	Spans              []model.SourceSpan   `json:"evidence_spans,omitempty"`
	Review             bool                 `json:"review_required"`
	Reasons            []string             `json:"review_reasons"`
	Quality            string               `json:"quality"`
	State              string               `json:"state"`
	StartLabel         string               `json:"start_label"`
	EndLabel           string               `json:"end_label"`
	Dates              DateFields           `json:"dates"`
}
type DateFields struct {
	StartPrecision string  `json:"start_precision"`
	EndPrecision   string  `json:"end_precision"`
	StartDate      *string `json:"start_date"`
	EndDate        *string `json:"end_date"`
	StartTime      *string `json:"start_time"`
	EndTime        *string `json:"end_time"`
}
type Notice struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	Summary        string     `json:"summary"`
	Content        string     `json:"content,omitempty"`
	Source         string     `json:"source"`
	Category       string     `json:"category"`
	Categories     []string   `json:"categories"`
	CategoryMethod string     `json:"category_method"`
	PublishedAt    *time.Time `json:"published_at"`
	OriginalURL    string     `json:"original_url"`
	Status         string     `json:"status"`
	Analyzed       bool       `json:"analyzed"`
	Review         bool       `json:"review_required"`
	SummaryReview  bool       `json:"summary_review_required"`
	Audience       *string    `json:"target_audience"`
	AudienceReview bool       `json:"audience_review_required"`
	Schedules      []Schedule `json:"schedules"`
}
type Page struct {
	Items []Notice  `json:"items"`
	Total int64     `json:"total"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
	Pages int       `json:"pages"`
	AsOf  time.Time `json:"as_of"`
}

func SafeURL(s string) string {
	u, e := url.Parse(s)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Port() != "" {
		return ""
	}
	return u.String()
}
func dateLabel(d, t *string, p *model.CalendarParts) string {
	v := ""
	if d != nil {
		v = *d
	} else if p != nil {
		if p.Year != nil {
			v = fmt.Sprintf("%d년", *p.Year)
		}
		if p.Month != nil {
			v += fmt.Sprintf(" %d월", *p.Month)
		}
		if p.Day != nil {
			v += fmt.Sprintf(" %d일", *p.Day)
		}
		if p.Year == nil {
			v += " (연도 미상)"
		}
	}
	if t != nil {
		if v != "" {
			v += " "
		}
		v += *t
	}
	return strings.TrimSpace(v)
}
func boundary(d, t *string, end bool) (time.Time, bool) {
	if d == nil {
		return time.Time{}, false
	}
	v, e := time.ParseInLocation("2006-01-02", *d, Seoul)
	if e != nil {
		return v, false
	}
	if t != nil {
		v, e = time.ParseInLocation("2006-01-02 15:04", *d+" "+*t, Seoul)
		return v, e == nil
	}
	if end {
		v = v.AddDate(0, 0, 1)
	}
	return v, true
}
func ScheduleView(s model.Schedule, now time.Time) Schedule {
	state := "일정 미상"
	quality := "uncertain"
	if s.Quality != nil {
		quality = s.Quality.State
	}
	review := s.ReviewRequired || (quality != "verified" && quality != "partial") || s.Timezone != "Asia/Seoul"
	start, hasStart := boundary(s.StartDate, s.StartTime, false)
	end, hasEnd := boundary(s.EndDate, s.EndTime, true)
	if s.StartDate != nil && !hasStart || s.EndDate != nil && !hasEnd || hasStart && hasEnd && start.After(end) {
		review = true
	}
	if review {
		state = "원문 확인 필요"
	} else if hasStart || hasEnd {
		switch s.Type {
		case "application", "submission", "completion":
			if hasEnd && !now.Before(end) {
				state = "신청 마감"
			} else if hasStart && now.Before(start) {
				state = "신청 예정"
			} else if hasEnd {
				state = "신청 중"
			}
		default:
			if hasStart && !hasEnd {
				if now.In(Seoul).Format("2006-01-02") > start.In(Seoul).Format("2006-01-02") {
					state = "행사 종료"
				} else if now.Before(start) {
					state = "행사 예정"
				} else {
					state = "행사 당일"
				}
				break
			}
			if hasEnd && !now.Before(end) {
				state = "행사 종료"
			} else if hasStart && now.Before(start) {
				state = "행사 예정"
			} else if hasStart && hasEnd && !now.Before(start) {
				state = "행사 진행 중"
			} else {
				state = "행사 예정"
			}
		}
	}
	return Schedule{ID: s.ID, ParentID: s.ParentID, Type: s.Type, Label: s.Label, StartPartial: s.StartPartial, EndPartial: s.EndPartial, Timezone: s.Timezone, Precision: s.Precision, Scope: s.Scope, Evidence: s.Evidence, Spans: s.EvidenceSpans, Review: review, Reasons: s.ReviewReasons, Quality: quality, State: state, StartLabel: dateLabel(s.StartDate, s.StartTime, s.StartPartial), EndLabel: dateLabel(s.EndDate, s.EndTime, s.EndPartial), Dates: DateFields{StartPrecision: s.StartPrecision, EndPrecision: s.EndPrecision, StartDate: s.StartDate, EndDate: s.EndDate, StartTime: s.StartTime, EndTime: s.EndTime}}
}
func View(v model.Content, detail bool, now time.Time) Notice {
	analyzed := v.ScheduleAnalysisID != "" && v.ScheduleInputHash != "" && v.ScheduleModelVersion != "" && v.ScheduleExtractionVersion != ""
	n := Notice{ID: v.ID.Hex(), Title: v.Title, Summary: v.Summary, Source: v.SourceName, Category: v.Category, Categories: v.Categories, CategoryMethod: v.ClassificationMethod, PublishedAt: v.PublishedAt, OriginalURL: SafeURL(v.OriginalURL), Status: v.Status, Analyzed: analyzed, Review: v.ScheduleReviewRequired, SummaryReview: v.SummaryReviewRequired, Audience: v.TargetAudience, AudienceReview: v.TargetAudienceReviewRequired, Schedules: []Schedule{}}
	if !analyzed {
		n.Summary = ""
		n.Audience = nil
	}
	if detail {
		n.Content = v.Content
	}
	for _, s := range v.Schedules {
		if analyzed {
			n.Schedules = append(n.Schedules, ScheduleView(s, now))
		}
	}
	return n
}
