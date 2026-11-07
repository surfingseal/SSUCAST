package scheduleexperiment

import (
	"context"
	"fmt"
	"google.golang.org/genai"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func p(s string) *string { return &s }
func TestDecodeStrictSchema(t *testing.T) {
	good := `{"summary":"안내입니다.","target_audience":null,"summary_review_required":false,"schedules":[]}`
	if _, e := Decode(good); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{strings.Replace(good, `"schedules":[]`, `"schedules":null`, 1), strings.Replace(good, `"summary_review_required":false`, `"summary_review_required":null`, 1), strings.Replace(good, `"summary":"안내입니다.",`, "", 1), strings.Replace(good, `"schedules":[]`, `"extra":1,"schedules":[]`, 1), good + `{}`} {
		if _, e := Decode(bad); e == nil {
			t.Errorf("accepted invalid schema: %s", bad)
		}
	}
	bad := `{"summary":"안내", "target_audience":null,"summary_review_required":false,"schedules":[{"type":"other","label":"일정","start_at":null,"end_at":null,"evidence":"원문","scope":null,"review_required":true,"review_reason":"불명확"}]}`
	if _, e := Decode(bad); e == nil {
		t.Fatal("unknown type accepted")
	}
}
func TestValidationGroundingAndTime(t *testing.T) {
	source := "행사: 2026.11.12 12:00~13:00"
	out := Output{Summary: "행사 안내", Schedules: []Schedule{{Type: "event", Label: "행사", StartAt: p("2026-11-12T12:00:00+09:00"), EndAt: p("2026-11-12T13:00:00+09:00"), Evidence: source}}}
	v := Validate(out, "", source)
	if v.EvidencePassed != 1 || len(v.Issues) != 0 {
		t.Fatal(v)
	}
	out.Schedules[0].EndAt = p("2026-11-12T11:00:00+09:00")
	out.Summary = "장학금 999999원 지급"
	v = Validate(out, "", source)
	if !v.ReviewRequired || len(v.Issues) < 2 {
		t.Fatal(v)
	}
	out.Schedules[0].Evidence = "신청 11.12까지"
	v = Validate(out, "", source)
	if v.EvidencePassed != 0 {
		t.Fatal(v)
	}
	out.Schedules[0].Evidence = "11.12까지"
	v = Validate(out, "", "11.12까지")
	if !strings.Contains(strings.Join(v.Issues, ","), "clock_not_supported") {
		t.Fatal(v)
	}
	out.Schedules = append(out.Schedules, out.Schedules[0])
	v = Validate(out, "", "11.12까지")
	if !strings.Contains(strings.Join(v.Issues, ","), "duplicate") {
		t.Fatal(v)
	}
}

type roundFunc func(*http.Request) (*http.Response, error)

func (f roundFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestBudgetHardLimitsAndNoFallback(t *testing.T) {
	calls := 0
	b := &Budget{Path: filepath.Join(t.TempDir(), "attempts.json"), Next: roundFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	})}
	req, _ := http.NewRequest("POST", "https://aiplatform.googleapis.com/v1/projects/p/locations/global/publishers/google/models/"+Model+":generateContent", nil)
	for i := 0; i < 12; i++ {
		b.ID = fmt.Sprint(i / 3)
		r, e := b.RoundTrip(req)
		if e != nil {
			t.Fatal(e)
		}
		r.Body.Close()
	}
	if _, e := b.RoundTrip(req); e == nil || calls != 12 {
		t.Fatalf("budget escaped: %d %v", calls, e)
	}
	req.URL.Path = strings.ReplaceAll(req.URL.Path, Model, "gemini-other")
	if _, e := b.RoundTrip(req); e == nil {
		t.Fatal("fallback allowed")
	}
}
func TestBudgetTenDistinctIDsAndNoSuccessfulRetry(t *testing.T) {
	b := &Budget{Path: filepath.Join(t.TempDir(), "attempts.json"), Next: roundFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	})}
	req, _ := http.NewRequest("POST", "https://aiplatform.googleapis.com/v1/publishers/google/models/"+Model+":generateContent", nil)
	b.ID = "0"
	r, e := b.RoundTrip(req)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if _, e = b.RoundTrip(req); e == nil {
		t.Fatal("successful request repeated")
	}
	for i := 1; i < 10; i++ {
		b.ID = fmt.Sprint(i)
		r, e = b.RoundTrip(req)
		if e != nil {
			t.Fatal(e)
		}
		r.Body.Close()
	}
	b.ID = "11"
	if _, e = b.RoundTrip(req); e == nil {
		t.Fatal("eleventh notice accepted")
	}
}
func TestSDKMockStructuredOutputSingleRequest(t *testing.T) {
	calls := 0
	hc := &http.Client{Transport: roundFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"responseMimeType":"application/json"`) || !strings.Contains(string(body), `"summary"`) {
			t.Error("schema missing")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"{\"summary\":\"일반 안내\",\"target_audience\":null,\"summary_review_required\":false,\"schedules\":[]}"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":20,"totalTokenCount":32}}`))}, nil
	})}
	sdk, e := genai.NewClient(context.Background(), &genai.ClientConfig{Backend: genai.BackendVertexAI, Project: "test", Location: "global", HTTPClient: hc, HTTPOptions: genai.HTTPOptions{BaseURL: "http://localhost/", APIVersion: "v1", RetryOptions: &genai.HTTPRetryOptions{Attempts: genai.Ptr[int32](1)}}})
	if e != nil {
		t.Fatal(e)
	}
	c := Client{SDK: sdk, Budget: &Budget{}}
	r, e := c.Generate(context.Background(), "one", map[string]string{"title": "안내"})
	if e != nil {
		t.Fatal(e)
	}
	out, e := Decode(r.Text())
	if e != nil || calls != 1 || out.Summary != "일반 안내" || r.UsageMetadata.TotalTokenCount != 32 {
		t.Fatalf("%+v %v calls=%d", out, e, calls)
	}
}
func TestConservativeOutputRejectsInventedClocks(t *testing.T) {
	out := Output{Summary: "안내", Schedules: []Schedule{{Type: "application", Evidence: "11월 23일까지", EndAt: p("2026-11-23T23:59:59+09:00")}}}
	c := ConservativeOutput(out, "", "11월 23일까지")
	if c.Schedules[0].EndAt != nil || !c.Schedules[0].ReviewRequired || out.Schedules[0].EndAt == nil {
		t.Fatal("raw response mutated or unsupported time retained")
	}
	out.Schedules[0].Evidence = "11월 23일 12:00"
	out.Schedules[0].EndAt = p("2026-11-23T18:00:00+09:00")
	if c = ConservativeOutput(out, "", out.Schedules[0].Evidence); c.Schedules[0].EndAt != nil {
		t.Fatal("different clock accepted")
	}
	out.Schedules[0].Evidence = "2026년 11월 23일 오후 1~4시"
	out.Schedules[0].StartAt = p("2026-11-23T13:00:00+09:00")
	out.Schedules[0].EndAt = p("2026-11-23T16:00:00+09:00")
	if c = ConservativeOutput(out, "", out.Schedules[0].Evidence); c.Schedules[0].EndAt == nil || c.Schedules[0].StartAt == nil {
		t.Fatal("explicit afternoon range rejected")
	}
}

func TestDateGroundingAndReversedBoundaries(t *testing.T) {
	out := Output{Summary: "안내", Schedules: []Schedule{{Type: "event", Evidence: "2026.11.12.(목) 12:00~13:00", StartAt: p("2026-11-13T12:00:00+09:00")}}}
	v := Validate(out, "", out.Schedules[0].Evidence)
	if !strings.Contains(strings.Join(v.Issues, ","), "date_not_supported") {
		t.Fatal(v)
	}
	c := ConservativeOutput(out, "", out.Schedules[0].Evidence)
	if c.Schedules[0].StartAt != nil {
		t.Fatal("invented calendar day retained")
	}
}
