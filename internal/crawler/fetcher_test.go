package crawler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/crawlserver/internal/model"
)

func fixtureSession(t *testing.T, h http.HandlerFunc, source model.Source) (*httptest.Server, *Session) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	f := NewFetcher(time.Second)
	f.FixtureOrigin = srv.URL
	source.BaseURL = srv.URL
	if source.RequestIntervalMS == 0 {
		source.RequestIntervalMS = 1
	}
	s, err := f.NewSession(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return srv, s
}
func allow(w http.ResponseWriter) {
	_, _ = fmt.Fprint(w, "User-agent: *\nAllow: /\nDisallow: /private\n")
}
func code(t *testing.T, err error, want string) {
	t.Helper()
	var e *FetchError
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("got %v, want FetchError %s", err, want)
	}
}
func TestFetcherRedirectPolicyAndBudget(t *testing.T) {
	var forbidden atomic.Int32
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forbidden.Add(1) }))
	defer external.Close()
	srv, s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			allow(w)
		case "/redirect":
			http.Redirect(w, r, "/ok", 302)
		case "/blocked":
			http.Redirect(w, r, "/private", 302)
		case "/external":
			http.Redirect(w, r, external.URL, 302)
		case "/loop":
			http.Redirect(w, r, "/loop", 302)
		default:
			_, _ = fmt.Fprint(w, "ok")
		}
	}, model.Source{MaxRequests: 30})
	page, err := s.Get(context.Background(), srv.URL+"/redirect")
	if err != nil || page.URL != srv.URL+"/ok" || s.Requests() != 3 {
		t.Fatalf("redirect %v %+v %d", err, page, s.Requests())
	}
	_, err = s.Get(context.Background(), srv.URL+"/blocked")
	code(t, err, "robots_denied")
	_, err = s.Get(context.Background(), srv.URL+"/external")
	code(t, err, "ssrf")
	if forbidden.Load() != 0 {
		t.Fatal("off-origin request escaped")
	}
	_, err = s.Get(context.Background(), srv.URL+"/loop")
	code(t, err, "redirect_limit")
}
func TestFetcherRequestBudgetIncludesRobotsRetries(t *testing.T) {
	var hits atomic.Int32
	srv, s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/robots.txt" {
			allow(w)
			return
		}
		w.WriteHeader(503)
	}, model.Source{MaxRequests: 2})
	_, err := s.Get(context.Background(), srv.URL)
	code(t, err, "request_limit")
	if hits.Load() != 2 || s.Requests() != 2 {
		t.Fatal(hits.Load(), s.Requests())
	}
	_, err = s.Get(context.Background(), srv.URL)
	code(t, err, "request_limit")
	if hits.Load() != 2 {
		t.Fatal("budget exceeded")
	}
}
func TestFetcherRetryAfterAndCancellation(t *testing.T) {
	var attempts atomic.Int32
	var first time.Time
	var spacing time.Duration
	var timing sync.Mutex
	srv, s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			allow(w)
			return
		}
		timing.Lock()
		defer timing.Unlock()
		if attempts.Add(1) == 1 {
			first = time.Now()
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			return
		}
		spacing = time.Since(first)
		_, _ = fmt.Fprint(w, "ok")
	}, model.Source{})
	if _, err := s.Get(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	timing.Lock()
	defer timing.Unlock()
	if attempts.Load() != 2 || spacing < 1900*time.Millisecond {
		t.Fatal(attempts.Load(), spacing)
	}
	if got := retryAfter("3", time.Now()); got != 3*time.Second {
		t.Fatal(got)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if got := retryAfter(now.Add(5*time.Second).Format(http.TimeFormat), now); got != 5*time.Second {
		t.Fatal(got)
	}
	if got := retryAfter("invalid", now); got != 0 {
		t.Fatal(got)
	}
}
func TestFetcherRetryAfterWaitIsCancellable(t *testing.T) {
	srv, s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			allow(w)
			return
		}
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
	}, model.Source{})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := s.Get(ctx, srv.URL)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	code(t, err, "timeout")
}
func TestFetcherTimeoutAndErrorSanitization(t *testing.T) {
	srv, s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			allow(w)
			return
		}
		<-r.Context().Done()
	}, model.Source{})
	s.client.Timeout = 20 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, err := s.Get(ctx, srv.URL+"/?token=secret")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatal("secret leaked")
	}
	code(t, err, "timeout")
}
func TestFetcherRobots404AndRedirectFailClosed(t *testing.T) {
	for _, status := range []int{404, 403, 302} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var posts atomic.Int32
			srv, s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/robots.txt" {
					w.Header().Set("Location", "/rules")
					w.WriteHeader(status)
					return
				}
				posts.Add(1)
				_, _ = fmt.Fprint(w, "ok")
			}, model.Source{})
			_, err := s.Get(context.Background(), srv.URL)
			if status == 404 {
				if err != nil || posts.Load() != 1 {
					t.Fatal(err)
				}
			} else if err == nil || posts.Load() != 0 {
				t.Fatal("robots fail-open", err)
			}
		})
	}
}
func TestFetcherBodyLimit(t *testing.T) {
	srv, s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			allow(w)
			return
		}
		_, _ = fmt.Fprint(w, strings.Repeat("x", maxBody+1))
	}, model.Source{})
	_, err := s.Get(context.Background(), srv.URL)
	code(t, err, "body_limit")
}
func TestFetcherSharedConcurrencyAndRobotsOnce(t *testing.T) {
	var active, peak, robots atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			robots.Add(1)
			allow(w)
			return
		}
		n := active.Add(1)
		defer active.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	f := NewFetcher(time.Second)
	f.FixtureOrigin = srv.URL
	source := model.Source{BaseURL: srv.URL, RequestIntervalMS: 1, MaxConcurrency: 2, MaxRequests: 50}
	a, err := f.NewSession(source)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := f.NewSession(source)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := a
			if i%2 == 1 {
				s = b
			}
			if _, err := s.Get(context.Background(), srv.URL); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if peak.Load() > 2 || peak.Load() < 2 {
		t.Fatal("unexpected concurrency", peak.Load())
	}
	if robots.Load() != 2 {
		t.Fatal("robots fetched more than once per session", robots.Load())
	}
}
func TestFetcherSharedRateLimitAndCrawlDelay(t *testing.T) {
	var mu sync.Mutex
	starts := []time.Time{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		if r.URL.Path == "/robots.txt" {
			_, _ = fmt.Fprint(w, "User-agent: *\nAllow: /\nCrawl-delay: 0.05\n")
			return
		}
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	f := NewFetcher(time.Second)
	f.FixtureOrigin = srv.URL
	a, err := f.NewSession(model.Source{BaseURL: srv.URL, RequestIntervalMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := f.NewSession(model.Source{BaseURL: srv.URL, RequestIntervalMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var wg sync.WaitGroup
	for _, s := range []*Session{a, b} {
		wg.Add(1)
		go func(s *Session) {
			defer wg.Done()
			if _, err := s.Get(context.Background(), srv.URL); err != nil {
				t.Error(err)
			}
		}(s)
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	for i := 1; i < len(starts); i++ {
		if starts[i].Sub(starts[i-1]) < 40*time.Millisecond {
			t.Fatalf("pacing not shared: %v", starts[i].Sub(starts[i-1]))
		}
	}
}
func TestFetcherDNSPrivateAddress(t *testing.T) {
	f := NewFetcher(time.Second)
	s, err := f.NewSession(model.Source{BaseURL: "http://localhost"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.Get(context.Background(), "http://localhost")
	code(t, err, "ssrf")
}

func TestFetcherOriginNormalization(t *testing.T) {
	f := NewFetcher(time.Second)
	a, err := f.NewSession(model.Source{BaseURL: "https://EXAMPLE.invalid:443", RequestIntervalMS: 3, MaxConcurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := f.NewSession(model.Source{BaseURL: "https://example.invalid", RequestIntervalMS: 5, MaxConcurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if a.gate != b.gate || a.gate.interval != 5*time.Millisecond || a.gate.limit != 1 {
		t.Fatal("origin policies not shared")
	}
}
func TestFetcherCancelledRobotsWaiter(t *testing.T) {
	started := make(chan struct{})
	srv, s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }, model.Source{})
	leader, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()
	done := make(chan struct{})
	go func() { defer close(done); _, _ = s.Get(leader, srv.URL) }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := s.Get(ctx, srv.URL)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	cancelLeader()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("robots request did not cancel")
	}
}
func TestExampleSmallLimitsAndListURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			allow(w)
		case "/list":
			_, _ = fmt.Fprint(w, `<a class="post" href="/one">one</a><a class="post" href="/two">two</a><a class="next" href="/next">next</a>`)
		case "/one":
			_, _ = fmt.Fprint(w, `<article><h1>One</h1><div class="body">Body</div></article>`)
		default:
			t.Errorf("request beyond limit: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c := New(time.Second)
	c.DemoOrigin = server.URL
	items, err := c.Crawl(context.Background(), model.Source{BaseURL: server.URL, ListURL: "/list", MaxPages: 1, MaxContents: 1, MaxRequests: 3, RequestIntervalMS: 1})
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
}

func TestFetcherRelativeListURL(t *testing.T) {
	f := NewFetcher(time.Second)
	s, err := f.NewSession(model.Source{BaseURL: "https://site.invalid/section/", ListURL: "posts"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.ListURL() != "https://site.invalid/section/posts" {
		t.Fatal(s.ListURL())
	}
}

func TestFetcherForbiddenResponseNeverRetries(t *testing.T) {
	var hits atomic.Int32
	srv, s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			allow(w)
			return
		}
		hits.Add(1)
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(403)
		_, _ = fmt.Fprint(w, "x")
	}, model.Source{})
	_, err := s.Get(context.Background(), srv.URL)
	var e *FetchError
	if !errors.As(err, &e) || e.Status != 403 || hits.Load() != 1 {
		t.Fatal(err, hits.Load())
	}
}

func TestRobotsCaptureLedgerAndHTMLFailClosed(t *testing.T) {
	for _, html := range []bool{false, true} {
		t.Run(fmt.Sprint(html), func(t *testing.T) {
			var hits atomic.Int32
			srv, s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				if r.URL.Path == "/robots.txt" {
					if html {
						fmt.Fprint(w, "<html><title>CAPTCHA</title></html>")
					} else {
						allow(w)
					}
					return
				}
				fmt.Fprint(w, "body")
			}, model.Source{MaxRequests: 2})
			_, err := s.Robots(context.Background())
			if html {
				if err == nil {
					t.Fatal("HTML robots accepted")
				}
				_, _ = s.Get(context.Background(), srv.URL)
				if hits.Load() != 1 {
					t.Fatal(hits.Load())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Get(context.Background(), srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = s.Robots(context.Background())
			log := s.RequestLog()
			if hits.Load() != 2 || len(log) != 2 || log[0].Stage != "robots" || log[1].Status != 200 {
				t.Fatal(log, hits.Load())
			}
			log[0].URL = "changed"
			if s.RequestLog()[0].URL == "changed" {
				t.Fatal("ledger alias")
			}
		})
	}
}
