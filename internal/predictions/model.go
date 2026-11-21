// Package predictions contains synthetic calendar scenarios, never real notices.
package predictions

import (
	"crypto/sha256"
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const Model = "gemini-3.8-flash"
const PromptVersion = "synthetic-calendar-v1"

var Seoul = time.FixedZone("Asia/Seoul", 9*3600)

type Slot struct {
	ID   string `json:"id"`
	Date string `json:"date"`
}
type Item struct {
	SlotID   string `json:"slot_id"`
	Title    string `json:"title"`
	Category string `json:"category"`
	Summary  string `json:"summary"`
	Reason   string `json:"reason"`
}
type Prediction struct {
	ExpectedFrom  string            `json:"expected_from,omitempty" bson:"expected_from,omitempty"`
	ExpectedTo    string            `json:"expected_to,omitempty" bson:"expected_to,omitempty"`
	Preparation   string            `json:"preparation,omitempty" bson:"preparation,omitempty"`
	Evidence      *ForecastEvidence `json:"evidence,omitempty" bson:"evidence,omitempty"`
	ID            string            `json:"id" bson:"_id"`
	RunID         string            `json:"run_id" bson:"run_id"`
	Title         string            `json:"title" bson:"title"`
	Category      string            `json:"category" bson:"category"`
	Summary       string            `json:"summary" bson:"summary"`
	Reason        string            `json:"reason" bson:"reason"`
	Date          string            `json:"date" bson:"date"`
	DataOrigin    string            `json:"data_origin" bson:"data_origin"`
	DateOrigin    string            `json:"date_origin" bson:"date_origin"`
	EventType     string            `json:"event_type" bson:"event_type"`
	Timezone      string            `json:"timezone" bson:"timezone"`
	HorizonFrom   string            `json:"horizon_from" bson:"horizon_from"`
	HorizonTo     string            `json:"horizon_to" bson:"horizon_to"`
	ModelVersion  string            `json:"model_version" bson:"model_version"`
	PromptVersion string            `json:"prompt_version" bson:"prompt_version"`
	GeneratedAt   time.Time         `json:"generated_at" bson:"generated_at"`
	PayloadHash   string            `json:"-" bson:"payload_hash"`
}

func Hash(v any) string { b, _ := json.Marshal(v); return fmt.Sprintf("%x", sha256.Sum256(b)) }
func ParseDate(s string) (time.Time, error) {
	d, e := time.ParseInLocation("2006-01-02", s, Seoul)
	if e != nil || d.Format("2006-01-02") != s {
		return time.Time{}, fmt.Errorf("invalid date")
	}
	return d, nil
}
func AddMonths(d time.Time, n int) time.Time {
	first := time.Date(d.Year(), d.Month()+time.Month(n), 1, 0, 0, 0, 0, Seoul)
	last := first.AddDate(0, 1, -1).Day()
	day := d.Day()
	if day > last {
		day = last
	}
	return first.AddDate(0, 0, day-1)
}

// Slots are explicitly synthetic display dates, not inferred or verified deadlines.
func Slots(from time.Time) []Slot {
	slots := []Slot{}
	for batch := 0; batch < 3; batch++ {
		start := AddMonths(from, batch)
		end := AddMonths(from, batch+1)
		if batch < 2 {
			end = end.AddDate(0, 0, -1)
		}
		days := int(end.Sub(start).Hours() / 24)
		for i := 0; i < 12; i++ {
			slots = append(slots, Slot{fmt.Sprintf("scenario-%02d", len(slots)+1), start.AddDate(0, 0, i*days/11).Format("2006-01-02")})
		}
	}
	return slots
}
func Build(run string, from, to string, slots []Slot, items []Item, at time.Time) ([]Prediction, error) {
	if len(slots) != len(items) || len(items) == 0 {
		return nil, fmt.Errorf("wrong scenario count")
	}
	by := map[string]Slot{}
	for _, s := range slots {
		by[s.ID] = s
	}
	seen := map[string]bool{}
	titles := map[string]bool{}
	out := []Prediction{}
	for _, i := range items {
		s, ok := by[i.SlotID]
		if !ok || seen[i.SlotID] {
			return nil, fmt.Errorf("unknown/duplicate slot")
		}
		seen[i.SlotID] = true
		p := Prediction{ID: Hash([]string{run, i.SlotID}), RunID: run, Title: strings.TrimSpace(i.Title), Category: i.Category, Summary: strings.TrimSpace(i.Summary), Reason: strings.TrimSpace(i.Reason), Date: s.Date, DataOrigin: "synthetic", DateOrigin: "simulation_slot", EventType: "deadline", Timezone: "Asia/Seoul", HorizonFrom: from, HorizonTo: to, ModelVersion: Model, PromptVersion: PromptVersion, GeneratedAt: at.UTC().Truncate(time.Millisecond)}
		if e := Validate(p); e != nil {
			return nil, e
		}
		title := strings.Join(strings.Fields(p.Title), "")
		if titles[title] {
			return nil, fmt.Errorf("duplicate title")
		}
		titles[title] = true
		p.PayloadHash = Digest(p)
		out = append(out, p)
	}
	return out, nil
}
func Digest(p Prediction) string { p.PayloadHash = ""; return Hash(p) }
func Validate(p Prediction) error {
	if p.DataOrigin == ForecastOrigin {
		return validateForecast(p)
	}
	date, e := ParseDate(p.Date)
	if e != nil {
		return e
	}
	from, e := ParseDate(p.HorizonFrom)
	if e != nil {
		return e
	}
	to, e := ParseDate(p.HorizonTo)
	if e != nil {
		return e
	}
	if date.Before(from) || date.After(to) || to.Before(from) || to.Sub(from) > 100*24*time.Hour {
		return fmt.Errorf("date outside synthetic horizon")
	}
	if p.ID == "" || p.RunID == "" || p.DataOrigin != "synthetic" || p.DateOrigin != "simulation_slot" || p.EventType != "deadline" || p.Timezone != "Asia/Seoul" || p.ModelVersion != Model || p.PromptVersion != PromptVersion || p.GeneratedAt.IsZero() {
		return fmt.Errorf("invalid synthetic provenance")
	}
	valid := false
	for _, c := range model.OfficialCategories {
		if p.Category == c {
			valid = true
		}
	}
	if !valid {
		return fmt.Errorf("invalid category")
	}
	for _, f := range []struct {
		v   string
		max int
	}{{p.Title, 80}, {p.Summary, 200}, {p.Reason, 200}} {
		if !utf8.ValidString(f.v) || strings.TrimSpace(f.v) == "" || utf8.RuneCountInString(f.v) > f.max || strings.ContainsAny(f.v, "\x00\r\n") || strings.Contains(f.v, "http://") || strings.Contains(f.v, "https://") {
			return fmt.Errorf("invalid generated text")
		}
	}
	if p.PayloadHash != "" && p.PayloadHash != Digest(p) {
		return fmt.Errorf("synthetic payload changed")
	}
	return nil
}
