package crawler

import (
	"context"
	"example.com/crawlserver/internal/model"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestRecentExplicitRetryLimit(t *testing.T) {
	for _, retries := range []int{0, 1} {
		t.Run(string(rune('0'+retries)), func(t *testing.T) {
			var attempts atomic.Int32
			_, s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/robots.txt" {
					allow(w)
					return
				}
				attempts.Add(1)
				w.WriteHeader(503)
			}, model.Source{MaxRequests: 10, MaxRetries: &retries})
			_, e := s.Get(context.Background(), "/temporary-failure")
			code(t, e, "http_status")
			if int(attempts.Load()) != 1+retries || s.Requests() != 2+retries {
				t.Fatal(attempts.Load(), s.Requests())
			}
		})
	}
}
