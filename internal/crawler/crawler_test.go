package crawler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"example.com/crawlserver/internal/fixture"
	"example.com/crawlserver/internal/model"
)

func TestExamplePaginationNormalization(t *testing.T) {
	server := httptest.NewServer(fixture.Handler())
	defer server.Close()
	c := New(time.Second)
	c.DemoOrigin = server.URL
	items, err := c.Crawl(context.Background(), model.Source{ID: bson.NewObjectID(), BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Title != "Go 백엔드 합격 후기" || items[0].OriginalURL != server.URL+"/posts/1" || len(items[0].ContentHash) != 64 || items[0].PublishedAt == nil || items[0].Metadata["job_role"] != "backend" {
		t.Fatalf("unexpected result: %+v", items)
	}
}
func TestRobotsDenied(t *testing.T) {
	var visits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
			return
		}
		visits.Add(1)
	}))
	defer s.Close()
	c := New(time.Second)
	c.DemoOrigin = s.URL
	_, err := c.Crawl(context.Background(), model.Source{BaseURL: s.URL})
	if err == nil || visits.Load() != 0 {
		t.Fatalf("robots bypass: %v", err)
	}
}
func TestSSRFPolicy(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "::ffff:127.0.0.1", "100.64.0.1", "224.0.0.1"} {
		if publicIP(net.ParseIP(ip)) {
			t.Errorf("allowed %s", ip)
		}
	}
	if !publicIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public rejected")
	}
	for _, raw := range []string{"file:///etc/passwd", "http://user:pass@example.com", "http://example.com:2375"} {
		if _, err := safeURL(raw); err == nil {
			t.Errorf("allowed %s", raw)
		}
	}
	f := NewFetcher(time.Second)
	if _, err := f.NewSession(model.Source{BaseURL: "http://127.0.0.1"}); err == nil {
		t.Fatal("private destination allowed")
	}

}
func TestRetryAndCancellation(t *testing.T) {
	var n atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
			return
		}
		if n.Add(1) < 3 {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	f := NewFetcher(time.Second)
	f.FixtureOrigin = server.URL
	session, err := f.NewSession(model.Source{BaseURL: server.URL, RequestIntervalMS: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	page, err := session.Get(context.Background(), server.URL)
	if err != nil || string(page.Body) != "ok" || n.Load() != 3 {
		t.Fatalf("retry: %v %s %d", err, page.Body, n.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = session.Get(ctx, server.URL)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestRobotsUnavailableAndExternalLink(t *testing.T) {
	for _, mode := range []string{"robots", "external", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/robots.txt" {
					if mode == "robots" {
						w.WriteHeader(403)
					} else {
						_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
					}
					return
				}
				if mode == "redirect" {
					http.Redirect(w, r, "http://127.0.0.1/private", 302)
					return
				}
				_, _ = w.Write([]byte(`<a class="post" href="http://evil.example/post">post</a>`))
			}))
			defer s.Close()
			c := New(time.Second)
			c.DemoOrigin = s.URL
			_, err := c.Crawl(context.Background(), model.Source{BaseURL: s.URL})
			if err == nil {
				t.Fatal("unsafe crawl succeeded")
			}
			if mode == "external" {
				code(t, err, "ssrf")
			}
		})
	}
}
