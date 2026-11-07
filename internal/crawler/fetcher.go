package crawler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/temoto/robotstxt"

	"example.com/crawlserver/internal/model"
)

const agent = "CareerResearchBot"
const maxBody = 2 << 20

// FetchError is safe to persist: URL excludes credentials, query and fragment.
// Unwrap preserves context cancellation and timeout identity.
type FetchError struct {
	Code, Stage, URL string
	Status, Attempt  int
	Err              error
	// Reason is a parser-supplied, sanitized diagnostic, never a response excerpt.
	Reason string
}

func (e *FetchError) Error() string {
	text := fmt.Sprintf("%s: %s (%s, HTTP %d, attempt %d)", e.Stage, e.Code, e.URL, e.Status, e.Attempt)
	if e.Reason != "" {
		text += ": " + e.Reason
	}
	return text
}
func (e *FetchError) Unwrap() error { return e.Err }
func failure(code, stage, raw string, status, attempt int, err error) *FetchError {
	u, _ := url.Parse(raw)
	safe := ""
	if u != nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		u.User = nil
		u.RawQuery = ""
		u.Fragment = ""
		safe = u.String()
	}
	return &FetchError{Code: code, Stage: stage, URL: safe, Status: status, Attempt: attempt, Err: err}
}
func contextFailure(stage, raw string, err error) *FetchError {
	code := "network"
	if errors.Is(err, context.Canceled) {
		code = "cancelled"
	} else if errors.Is(err, context.DeadlineExceeded) {
		code = "timeout"
	}
	return failure(code, stage, raw, 0, 0, err)
}

// Fetcher owns origin-wide pacing/concurrency; Session owns one job's HTTP budget.
// Configure before sharing between goroutines. FixtureOrigin is only for a
// trusted in-process fixture, never loaded from a source config or an API input.
type Fetcher struct {
	MaxResponseBytes int64 // Configure before use; zero keeps the normal 2MiB cap, at most10MiB.
	Timeout          time.Duration
	FixtureOrigin    string
	mu               sync.Mutex
	origins          map[string]*originGate
}
type originGate struct {
	mu            sync.Mutex
	active, limit int
	interval      time.Duration
	next          time.Time
	changed       chan struct{}
}

func NewFetcher(timeout time.Duration) *Fetcher {
	return &Fetcher{Timeout: timeout, origins: make(map[string]*originGate)}
}
func (f *Fetcher) gate(origin string, interval time.Duration, limit int) *originGate {
	f.mu.Lock()
	defer f.mu.Unlock()
	g := f.origins[origin]
	if g == nil {
		g = &originGate{limit: limit, interval: interval, changed: make(chan struct{})}
		f.origins[origin] = g
	} else {
		g.tighten(interval, limit)
	}
	return g
}
func (g *originGate) tighten(interval time.Duration, limit int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if interval > g.interval {
		g.next = g.next.Add(interval - g.interval)
		g.interval = interval
	}
	if limit < g.limit {
		g.limit = limit
	}
}
func (g *originGate) cooldown(until time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if until.After(g.next) {
		g.next = until
	}
}
func (g *originGate) acquire(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		g.mu.Lock()
		wait := time.Until(g.next)
		changed := g.changed
		if g.active < g.limit && wait <= 0 {
			g.active++
			g.next = time.Now().Add(g.interval)
			g.mu.Unlock()
			return nil
		}
		full := g.active >= g.limit
		g.mu.Unlock()
		if full {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-changed:
			}
		} else {
			if err := pause(ctx, wait); err != nil {
				return err
			}
		}
	}
}
func (g *originGate) release() {
	g.mu.Lock()
	g.active--
	close(g.changed)
	g.changed = make(chan struct{})
	g.mu.Unlock()
}
func pause(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

type Page struct {
	Body   []byte
	URL    string
	Status int
}

// RequestRecord accounts for every network attempt, including robots, retries
// and redirects. It is diagnostic only; it never bypasses request validation.
type RequestRecord struct {
	URL       string    `json:"url"`
	Stage     string    `json:"stage"`
	Attempt   int       `json:"attempt"`
	Status    int       `json:"status"`
	StartedAt time.Time `json:"started_at"`
}
type Session struct {
	source           model.Source
	origin           *url.URL
	client           *http.Client
	gate             *originGate
	mu               sync.Mutex
	requests         int
	robotsReady      chan struct{}
	robots           *robotstxt.Group
	robotsErr        error
	robotsPage       Page
	requestLog       []RequestRecord
	maxResponseBytes int64
}

func ValidateSource(s model.Source) error {
	session, err := NewFetcher(time.Second).NewSession(s)
	if err == nil {
		session.Close()
	}
	return err
}
func (f *Fetcher) NewSession(s model.Source) (*Session, error) {
	return f.newSession(s, f.FixtureOrigin)
}
func (f *Fetcher) newSession(s model.Source, fixture string) (*Session, error) {
	s = s.WithDefaults()
	if f.MaxResponseBytes < 0 || f.MaxResponseBytes > 10<<20 {
		return nil, fmt.Errorf("response size cap outside0..10MiB")
	}
	if s.RequestJitterMS < 0 || s.RequestJitterMS > 60000 || s.MaxRetries != nil && (*s.MaxRetries < 0 || *s.MaxRetries > 2) {
		return nil, fmt.Errorf("invalid jitter/retry limits")
	}
	if f.Timeout <= 0 || s.RequestIntervalMS < 1 || s.RequestIntervalMS > 86400000 || s.MaxPages < 1 || s.MaxPages > 10000 || s.MaxContents < 1 || s.MaxContents > 10000 || s.MaxRequests < 1 || s.MaxRequests > 100000 || s.MaxConcurrency < 1 || s.MaxConcurrency > 16 {
		return nil, failure("config", "validate", s.BaseURL, 0, 0, errors.New("invalid timeout or source limits"))
	}
	base, err := safeURL(s.BaseURL)
	parsed, parseErr := url.Parse(s.BaseURL)
	isFixture := parseErr == nil && fixture != "" && parsed.Scheme+"://"+parsed.Host == fixture && parsed.User == nil
	if err != nil && isFixture {
		base = parsed
		err = nil
	}
	if err != nil {
		return nil, failure("url", "validate", s.BaseURL, 0, 0, err)
	}
	if ip := net.ParseIP(base.Hostname()); ip != nil && !publicIP(ip) && !isFixture {
		return nil, failure("ssrf", "validate", s.BaseURL, 0, 0, errors.New("nonpublic address"))
	}
	origin := &url.URL{Scheme: base.Scheme, Host: base.Host}
	session := &Session{source: s, origin: origin, maxResponseBytes: f.MaxResponseBytes, gate: f.gate(origin.String(), time.Duration(s.RequestIntervalMS)*time.Millisecond, s.MaxConcurrency)}
	listURL, err := session.Resolve(base.String(), s.ListURL)
	if err != nil {
		return nil, err
	}
	session.source.ListURL = listURL
	tr := &http.Transport{MaxIdleConns: 16, MaxIdleConnsPerHost: s.MaxConcurrency, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: f.Timeout, ResponseHeaderTimeout: f.Timeout}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if host != origin.Hostname() {
			return nil, failure("ssrf", "dial", origin.String(), 0, 0, errors.New("off-origin dial"))
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, errors.New("empty DNS response")
		}
		for _, ip := range ips {
			if !publicIP(ip) && !isFixture {
				return nil, failure("ssrf", "dial", origin.String(), 0, 0, errors.New("nonpublic destination"))
			}
		}
		return (&net.Dialer{Timeout: f.Timeout}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	session.client = &http.Client{Timeout: f.Timeout, Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return session, nil
}
func (s *Session) ListURL() string { return s.source.ListURL }
func (s *Session) Close()          { s.client.CloseIdleConnections() }
func (s *Session) Requests() int   { s.mu.Lock(); defer s.mu.Unlock(); return s.requests }
func (s *Session) RequestLog() []RequestRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]RequestRecord{}, s.requestLog...)
}

// Robots returns the same robots response used by Get, without another request.
func (s *Session) Robots(ctx context.Context) (Page, error) {
	err := s.ensureRobots(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.robotsPage
	p.Body = append([]byte(nil), p.Body...)
	return p, err
}
func (s *Session) Resolve(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", failure("url", "resolve", base, 0, 0, err)
	}
	u, err := b.Parse(ref)
	if err != nil {
		return "", failure("url", "resolve", base, 0, 0, err)
	}
	normalizeURL(u)
	if u.Scheme != s.origin.Scheme || u.Host != s.origin.Host || u.User != nil {
		return "", failure("ssrf", "resolve", u.String(), 0, 0, errors.New("off-origin URL blocked"))
	}
	u.Fragment = ""
	return u.String(), nil
}
func (s *Session) ensureRobots(ctx context.Context) error {
	s.mu.Lock()
	if ready := s.robotsReady; ready != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return contextFailure("robots", s.origin.String(), ctx.Err())
		case <-ready:
		}
		s.mu.Lock()
		err := s.robotsErr
		s.mu.Unlock()
		return err
	}
	s.robotsReady = make(chan struct{})
	s.mu.Unlock()
	raw := s.origin.String() + "/robots.txt"
	page, err := s.request(ctx, raw, "robots")
	var robotsHTTPError *FetchError
	if page.Status == 404 && errors.As(err, &robotsHTTPError) && robotsHTTPError.Code == "http_status" {
		page.Body = []byte("User-agent: *\nAllow: /\n")
		err = nil
	}
	var group *robotstxt.Group
	if err == nil {
		if strings.HasPrefix(strings.TrimSpace(string(page.Body)), "<") {
			err = failure("robots", "robots", raw, page.Status, 0, errors.New("HTML/challenge response is not robots.txt"))
		} else if page.Status >= 300 && page.Status < 400 {
			err = failure("robots", "robots", raw, page.Status, 0, errors.New("robots redirects are not followed"))
		} else {
			var data *robotstxt.RobotsData
			data, err = robotstxt.FromBytes(page.Body)
			if err == nil {
				group = data.FindGroup(agent)
				s.gate.tighten(group.CrawlDelay, s.source.MaxConcurrency)
			} else {
				err = failure("robots", "robots", raw, page.Status, 0, err)
			}
		}
	}
	s.mu.Lock()
	s.robots = group
	s.robotsPage = page
	s.robotsErr = err
	close(s.robotsReady)
	s.mu.Unlock()
	return err
}
func (s *Session) Get(ctx context.Context, raw string) (Page, error) {
	target, err := s.Resolve(s.origin.String(), raw)
	if err != nil {
		return Page{}, err
	}
	if err = s.ensureRobots(ctx); err != nil {
		return Page{}, err
	}
	for hops := 0; hops <= 5; hops++ {
		u, _ := url.Parse(target)
		if !s.robots.Test(u.RequestURI()) {
			return Page{}, failure("robots_denied", "robots", target, 0, 0, errors.New("robots disallows path"))
		}
		page, err := s.request(ctx, target, "fetch")
		if err != nil {
			return page, err
		}
		if page.Status < 300 || page.Status >= 400 {
			return page, nil
		}
		// The redirect target passes the same origin, robots, DNS and budget checks.
		if page.URL == "" {
			return page, failure("redirect", "redirect", target, page.Status, 0, errors.New("missing Location"))
		}
		target, err = s.Resolve(target, page.URL)
		if err != nil {
			return Page{}, err
		}
	}
	return Page{}, failure("redirect_limit", "redirect", target, 0, 0, errors.New("too many redirects"))
}
func retryAfter(raw string, now time.Time) time.Duration {
	if n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && n >= 0 {
		if n > int64((time.Duration(1<<63-1))/time.Second) {
			return time.Duration(1<<63 - 1)
		}
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(raw); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}
func (s *Session) request(ctx context.Context, raw, stage string) (Page, error) {
	attempts := 3
	if s.source.MaxRetries != nil {
		attempts = 1 + *s.source.MaxRetries
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return Page{}, contextFailure(stage, raw, err)
		}
		if s.Requests() >= s.source.MaxRequests {
			return Page{}, failure("request_limit", stage, raw, 0, attempt, errors.New("HTTP request budget exhausted"))
		}
		if err := s.gate.acquire(ctx); err != nil {
			return Page{}, contextFailure(stage, raw, err)
		}
		if s.source.RequestJitterMS > 0 {
			s.gate.mu.Lock()
			s.gate.next = s.gate.next.Add(time.Duration(rand.Int64N(s.source.RequestJitterMS+1)) * time.Millisecond)
			s.gate.mu.Unlock()
		}
		s.mu.Lock()
		if s.requests >= s.source.MaxRequests {
			s.mu.Unlock()
			s.gate.release()
			return Page{}, failure("request_limit", stage, raw, 0, attempt, errors.New("HTTP request budget exhausted"))
		}
		s.requests++
		s.mu.Unlock()
		started := time.Now().UTC()
		page, after, err := s.once(ctx, raw, stage, attempt)
		s.mu.Lock()
		s.requestLog = append(s.requestLog, RequestRecord{raw, stage, attempt, page.Status, started})
		s.mu.Unlock()
		if after > 0 {
			s.gate.cooldown(time.Now().Add(after))
		}
		s.gate.release()
		if err == nil {
			return page, nil
		}
		var fe *FetchError
		if !errors.As(err, &fe) {
			return page, err
		}
		var ne net.Error
		transient := errors.As(err, &ne) && (ne.Timeout() || ne.Temporary())
		retry := fe.Code != "body_limit" && fe.Status != 401 && fe.Status != 403 && (fe.Status == 429 || fe.Status >= 500 || transient || errors.Is(err, io.ErrUnexpectedEOF))
		if !retry || attempt == attempts || ctx.Err() != nil {
			return page, err
		}
		if s.Requests() >= s.source.MaxRequests {
			return page, failure("request_limit", stage, raw, page.Status, attempt, errors.New("HTTP request budget exhausted"))
		}
		delay := max(time.Duration(1<<(attempt-1))*time.Second, after)
		if err := pause(ctx, delay); err != nil {
			return page, contextFailure(stage, raw, err)
		}
	}
	return Page{}, failure("network", stage, raw, 0, 3, errors.New("retry exhausted"))
}
func (s *Session) once(ctx context.Context, raw, stage string, attempt int) (Page, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return Page{}, 0, failure("url", stage, raw, 0, attempt, err)
	}
	req.Header.Set("User-Agent", agent+"/1.0")
	req.Header.Set("Accept", "text/html,text/plain,application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		var policy *FetchError
		if errors.As(err, &policy) {
			policy.Attempt = attempt
			return Page{}, 0, policy
		}
		fe := contextFailure(stage, raw, err)
		fe.Attempt = attempt
		return Page{}, 0, fe
	}
	limit := s.maxResponseBytes
	if limit == 0 || stage == "robots" {
		limit = maxBody
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	closeErr := resp.Body.Close()
	if readErr == nil {
		readErr = closeErr
	}
	page := Page{Body: body, URL: raw, Status: resp.StatusCode}
	if int64(len(body)) > limit {
		return page, 0, failure("body_limit", stage, raw, resp.StatusCode, attempt, errors.New("response exceeds size limit"))
	}
	if readErr != nil {
		fe := contextFailure(stage, raw, readErr)
		fe.Status = resp.StatusCode
		fe.Attempt = attempt
		return page, 0, fe
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		page.URL = resp.Header.Get("Location")
		return page, 0, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		after := time.Duration(0)
		if resp.StatusCode == 429 || resp.StatusCode == 503 {
			after = retryAfter(resp.Header.Get("Retry-After"), time.Now())
		}
		return page, after, failure("http_status", stage, raw, resp.StatusCode, attempt, errors.New("unsuccessful HTTP response"))
	}
	return page, 0, nil
}
