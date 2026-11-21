package web

import (
	"context"
	"example.com/crawlserver/internal/predictions"
	"testing"
	"time"
)

type predictionFake struct {
	fake
	from, to string
}

func (f *predictionFake) NoticePredictions(_ context.Context, from, to string) ([]predictions.Prediction, error) {
	f.from, f.to = from, to
	return []predictions.Prediction{}, nil
}
func TestSyntheticPredictionsAPIIsReadOnlyAndBounded(t *testing.T) {
	f := &predictionFake{}
	h := (&Server{Store: f, Token: testToken, Now: func() time.Time { return time.Date(2026, 11, 17, 0, 0, 0, 0, predictions.Seoul) }}).Handler()
	if w := call(h, "GET", "/api/notice-predictions", "", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := call(h, "GET", "/api/notice-predictions", "", true); w.Code != 200 || f.from != "2026-11-17" || f.to != "2027-02-17" {
		t.Fatal(w.Code, f.from, f.to)
	}
	if w := call(h, "GET", "/api/notice-predictions?from=2026-11-01&to=2027-02-28", "", true); w.Code != 200 || f.from != "2026-11-01" || f.to != "2027-02-28" {
		t.Fatal("four-month horizon", w.Code)
	}
	for _, q := range []string{"?from=2026-11-17", "?from=2026-11-17&to=2027-11-17", "?from=2026-11-17&to=2026-11-16", "?from=2026-02-30&to=2026-03-01", "?category=scholarship"} {
		if w := call(h, "GET", "/api/notice-predictions"+q, "", true); w.Code != 400 {
			t.Fatal(q, w.Code)
		}
	}
	if w := call(h, "POST", "/api/notice-predictions", "{}", true); w.Code != 405 {
		t.Fatal(w.Code)
	}
}
