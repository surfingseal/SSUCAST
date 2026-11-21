package crawler

import (
	"fmt"
	"time"
)

// SSUCollectionRange selects publication dates, not inferred deadline dates.
// Bounds are inclusive KST calendar days; crawl timestamps always remain real.
type SSUCollectionRange struct{ From, To time.Time }

func ParseSSUCollectionRange(from, to string) (*SSUCollectionRange, error) {
	loc := time.FixedZone("Asia/Seoul", 9*3600)
	a, e := time.ParseInLocation("2006-01-02", from, loc)
	if e != nil || a.Format("2006-01-02") != from {
		return nil, fmt.Errorf("invalid from date")
	}
	b, e := time.ParseInLocation("2006-01-02", to, loc)
	if e != nil || b.Format("2006-01-02") != to {
		return nil, fmt.Errorf("invalid to date")
	}
	r := &SSUCollectionRange{a, b}
	return r, r.Validate(time.Now())
}

func (r SSUCollectionRange) Validate(now time.Time) error {
	loc := time.FixedZone("Asia/Seoul", 9*3600)
	today := now.In(loc)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)
	for _, d := range []time.Time{r.From, r.To} {
		_, offset := d.Zone()
		if d.IsZero() || offset != 9*3600 || d.Hour() != 0 || d.Minute() != 0 || d.Second() != 0 || d.Nanosecond() != 0 {
			return fmt.Errorf("KST calendar days required")
		}
	}
	if r.To.Before(r.From) || r.To.After(today) || r.To.Sub(r.From) > 366*24*time.Hour {
		return fmt.Errorf("historical range must be ordered, at most 367 days, and not future")
	}
	return nil
}
