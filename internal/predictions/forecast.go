package predictions

import (
	"example.com/crawlserver/internal/crawler"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const ForecastOrigin = "historical_forecast"
const ForecastPrompt = "historical-calendar-v1"

type ForecastEvidence struct {
	ID            string `json:"id" bson:"id"`
	HistoryRunID  string `json:"history_run_id" bson:"history_run_id"`
	URL           string `json:"url" bson:"url"`
	Title         string `json:"title" bson:"title"`
	PublishedDate string `json:"published_date" bson:"published_date"`
	Deadline      string `json:"deadline" bson:"deadline"`
	Quote         string `json:"quote" bson:"quote"`
	ContentHash   string `json:"content_hash" bson:"content_hash"`
}
type ForecastProposal struct {
	HistoricalID   string `json:"historical_id"`
	HistoricalDate string `json:"historical_date"`
	Quote          string `json:"quote"`
	Title          string `json:"title"`
	Summary        string `json:"summary"`
	Reason         string `json:"reason"`
	Preparation    string `json:"preparation"`
}

// DateEvidence checks the extracted day against a literal source quote. An AI
// supplied day without an explicit month/day in source text is never accepted.
func DateEvidence(quote, day string) bool {
	d, e := ParseDate(day)
	if e != nil {
		return false
	}
	re := regexp.MustCompile(`(?:(20\d{2})\s*[년./-]\s*)?(\d{1,2})\s*[월./-]\s*(\d{1,2})(?:\s*일)?`)
	for _, m := range re.FindAllStringSubmatch(quote, -1) {
		for _, v := range []string{d.Format("1"), d.Format("01")} {
			if m[2] != v {
				continue
			}
			for _, w := range []string{d.Format("2"), d.Format("02")} {
				if m[3] == w && (m[1] == "" || m[1] == d.Format("2006")) {
					return true
				}
			}
		}
	}
	return false
}
func BuildForecasts(run, from, to string, history HistoryRun, source []HistoricalNotice, proposals []ForecastProposal, at time.Time) ([]Prediction, error) {
	a, e := ParseDate(from)
	if e != nil {
		return nil, e
	}
	b, e := ParseDate(to)
	if e != nil || b.Before(a) || b.Sub(a) > 123*24*time.Hour {
		return nil, fmt.Errorf("bounded four-month horizon required")
	}
	if !history.Completed || len(source) != history.Collected {
		return nil, fmt.Errorf("complete history required")
	}
	by := map[string]HistoricalNotice{}
	for _, v := range source {
		canonical, err := HistoryRecord(history, v.Content)
		if err != nil || canonical.ID != v.ID || v.RunID != history.ID || v.DataOrigin != "historical_http" {
			return nil, fmt.Errorf("foreign/synthetic evidence")
		}
		by[v.ID] = v
	}
	seen := map[string]bool{}
	rows := []Prediction{}
	for _, p := range proposals {
		v, ok := by[p.HistoricalID]
		if !ok || seen[p.HistoricalID] {
			return nil, fmt.Errorf("unknown or repeated historical ID")
		}
		seen[p.HistoricalID] = true
		day, e := ParseDate(p.HistoricalDate)
		if e != nil {
			return nil, e
		}
		published := v.Content.PublishedAt
		if published == nil || day.Before(*published) || day.After(published.AddDate(0, 0, 180)) || !strings.Contains(v.Content.Content, p.Quote) || !DateEvidence(p.Quote, p.HistoricalDate) || utf8.RuneCountInString(p.Quote) > 600 || strings.TrimSpace(p.Quote) == "" {
			return nil, fmt.Errorf("unverified historical date/quote for %s", p.HistoricalID)
		}
		date := AddMonths(day, 12)
		if date.Before(a) || date.After(b) {
			return nil, fmt.Errorf("anniversary outside horizon")
		}
		lower, upper := date.AddDate(0, 0, -7), date.AddDate(0, 0, 7)
		if lower.Before(a) {
			lower = a
		}
		if upper.After(b) {
			upper = b
		}
		evidence := &ForecastEvidence{v.ID, history.ID, v.Content.OriginalURL, v.Content.Title, published.In(Seoul).Format("2006-01-02"), p.HistoricalDate, p.Quote, v.Content.ContentHash}
		row := Prediction{ID: Hash([]string{run, v.ID}), RunID: run, Title: p.Title, Category: v.Content.Category, Summary: p.Summary, Reason: p.Reason, Preparation: p.Preparation, Date: date.Format("2006-01-02"), DataOrigin: ForecastOrigin, DateOrigin: "anniversary_estimate", EventType: "deadline", Timezone: "Asia/Seoul", HorizonFrom: from, HorizonTo: to, ModelVersion: Model, PromptVersion: ForecastPrompt, GeneratedAt: at.UTC().Truncate(time.Millisecond), ExpectedFrom: lower.Format("2006-01-02"), ExpectedTo: upper.Format("2006-01-02"), Evidence: evidence}
		if e = Validate(row); e != nil {
			return nil, e
		}
		row.PayloadHash = Digest(row)
		rows = append(rows, row)
	}
	return rows, nil
}
func validateForecast(p Prediction) error {
	d, e := ParseDate(p.Date)
	if e != nil {
		return e
	}
	a, e := ParseDate(p.HorizonFrom)
	if e != nil {
		return e
	}
	b, e := ParseDate(p.HorizonTo)
	if e != nil {
		return e
	}
	lo, e := ParseDate(p.ExpectedFrom)
	if e != nil {
		return e
	}
	hi, e := ParseDate(p.ExpectedTo)
	if e != nil {
		return e
	}
	if b.Before(a) || b.Sub(a) > 123*24*time.Hour || d.Before(a) || d.After(b) || lo.Before(a) || hi.After(b) || lo.After(d) || hi.Before(d) || hi.Sub(lo) > 14*24*time.Hour {
		return fmt.Errorf("invalid forecast window")
	}
	if p.ID == "" || p.RunID == "" || p.DataOrigin != ForecastOrigin || p.DateOrigin != "anniversary_estimate" || p.EventType != "deadline" || p.Timezone != "Asia/Seoul" || p.ModelVersion != Model || p.PromptVersion != ForecastPrompt || p.GeneratedAt.IsZero() || p.Evidence == nil {
		return fmt.Errorf("invalid forecast provenance")
	}
	ev := p.Evidence
	if p.ID != Hash([]string{p.RunID, ev.ID}) {
		return fmt.Errorf("forecast identity mismatch")
	}
	canonicalURL, urlErr := crawler.SSUCatchURL(ev.URL, ev.URL)
	historical, e := ParseDate(ev.Deadline)
	if e != nil || AddMonths(historical, 12).Format("2006-01-02") != p.Date || ev.ID == "" || ev.HistoryRunID == "" || ev.ContentHash == "" || urlErr != nil || canonicalURL != ev.URL || !DateEvidence(ev.Quote, ev.Deadline) {
		return fmt.Errorf("invalid historical reference")
	}
	for _, v := range []string{p.Title, p.Summary, p.Reason, p.Preparation} {
		if strings.TrimSpace(v) == "" || !utf8.ValidString(v) || utf8.RuneCountInString(v) > 500 || strings.ContainsAny(v, "\x00\r\n") || strings.Contains(v, "http://") || strings.Contains(v, "https://") {
			return fmt.Errorf("invalid forecast text")
		}
	}
	if utf8.RuneCountInString(p.Title) > 80 {
		return fmt.Errorf("forecast title too long")
	}
	valid := false
	for _, c := range []string{"academic", "scholarship", "international", "international_student", "extracurricular", "teaching", "volunteer", "other"} {
		if p.Category == c {
			valid = true
		}
	}
	if !valid {
		return fmt.Errorf("invalid forecast category")
	}
	if p.PayloadHash != "" && p.PayloadHash != Digest(p) {
		return fmt.Errorf("forecast payload changed")
	}
	return nil
}

var titleYear = regexp.MustCompile(`20\d{2}`)
var titleDateAnnotation = regexp.MustCompile(`[(（\[][^()（）\[\]]*\d{1,2}\s*[./월]\s*\d{1,2}[^()（）\[\]]*[)）\]]`)
var titleUpdateAnnotation = regexp.MustCompile(`[(（\[]\s*(?:(?:기한|기간|모집|접수|마감)\s*연장|재공지|재안내|수정)\s*[)）\]]`)
var titleCohort = regexp.MustCompile(`제\s*\d+\s*기`)

// Conservative exact comparison after removing year, cohort, parenthetical
// date annotations and punctuation. Meaningful non-date parentheses stay.
func ForecastTitleKey(title string) string {
	title = titleDateAnnotation.ReplaceAllString(strings.ToLower(title), "")
	title = titleUpdateAnnotation.ReplaceAllString(title, "")
	title = titleCohort.ReplaceAllString(title, "")
	title = titleYear.ReplaceAllString(title, "")
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, title)
}
