package schedulebatch

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/scheduleexperiment"
	"fmt"
	"google.golang.org/genai"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundFunc func(*http.Request) (*http.Response, error)

func (f roundFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func ids() []string {
	x := []string{}
	for i := 0; i < 76; i++ {
		x = append(x, fmt.Sprintf("id-%02d", i))
	}
	return x
}
func request() *http.Request {
	r, _ := http.NewRequest("POST", "https://aiplatform.googleapis.com/v1/projects/p/locations/global/publishers/google/models/"+Model+":generateContent", nil)
	return r
}
func TestBatchBudgetNinetyAndNoModelFallback(t *testing.T) {
	b, e := LoadBudget(t.TempDir(), ids())
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	b.Next = roundFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"private":"redact"}`))}, nil
	})
	for i := 0; i < 90; i++ {
		b.ID = ids()[i/3]
		resp, e := b.RoundTrip(request())
		if e != nil {
			t.Fatal(e)
		}
		resp.Body.Close()
	}
	if _, e = b.RoundTrip(request()); e == nil || calls != 90 {
		t.Fatal("request budget escaped", calls, e)
	}
	raw, _ := os.ReadFile(b.responsePath(1))
	if strings.Contains(string(raw), "private") {
		t.Fatal("API error body leaked")
	}
	b.ID = "excluded-prior-ten"
	if _, e = b.RoundTrip(request()); e == nil {
		t.Fatal("excluded ID allowed")
	}
}
func TestSuccessfulHTTPNeverRetriedAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	b, _ := LoadBudget(dir, ids())
	b.ID = ids()[0]
	b.Next = roundFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"candidates":[]}`))}, nil
	})
	resp, e := b.RoundTrip(request())
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	again, e := LoadBudget(dir, ids())
	if e != nil {
		t.Fatal(e)
	}
	again.ID = ids()[0]
	if _, e = again.RoundTrip(request()); e == nil {
		t.Fatal("paid success replayed")
	}
	r := request()
	r.URL.Path = strings.Replace(r.URL.Path, Model, "another-paid-model", 1)
	if _, e = again.RoundTrip(r); e == nil {
		t.Fatal("model fallback permitted")
	}
}
func TestRecoveryOfPaidResponseWithoutAnotherCall(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "checkpoints"), 0700)
	b, _ := LoadBudget(dir, ids())
	id := ids()[0]
	b.Attempts = []Attempt{{Number: 1, ID: id, HTTPStatus: 200, Finished: true}}
	r := genai.GenerateContentResponse{ModelVersion: Model, Candidates: []*genai.Candidate{{FinishReason: genai.FinishReasonStop, Content: &genai.Content{Parts: []*genai.Part{{Text: `{"summary":"안내","target_audience":null,"summary_review_required":false,"schedules":[]}`}}}}}}
	raw, _ := json.Marshal(r)
	scheduleexperiment.Write(b.responsePath(1), SavedHTTP{200, string(raw)})
	v := model.Content{Title: "안내"}
	v.ID = [12]byte{} // use the actual notice ID as the journal key
	b.Attempts[0].ID = v.ID.Hex()
	cp, e := Recover(dir, b, v)
	if e != nil || cp == nil || cp.RawResponse == "" {
		t.Fatal(cp, e)
	}
	cp, e = Recover(dir, b, v)
	if e != nil || cp == nil {
		t.Fatal("checkpoint not reused", e)
	}
}
func TestUnknownInFlightJournalStopsResume(t *testing.T) {
	dir := t.TempDir()
	scheduleexperiment.Write(filepath.Join(dir, "attempts.json"), []Attempt{{Number: 1, ID: ids()[0]}})
	if _, e := LoadBudget(dir, ids()); e == nil {
		t.Fatal("ambiguous paid request replay allowed")
	}
}
func TestSDKUsesSchemaAndSingleGeneration(t *testing.T) {
	calls := 0
	hc := &http.Client{Transport: roundFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"responseMimeType":"application/json"`) || !strings.Contains(string(body), "start_partial") {
			t.Error("v2 schema missing")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"{}"}],"role":"model"},"finishReason":"STOP"}]}`))}, nil
	})}
	sdk, e := genai.NewClient(context.Background(), &genai.ClientConfig{Backend: genai.BackendVertexAI, Project: "test", Location: "global", HTTPClient: hc, HTTPOptions: genai.HTTPOptions{BaseURL: "http://localhost/", APIVersion: "v1", RetryOptions: &genai.HTTPRetryOptions{Attempts: genai.Ptr[int32](1)}}})
	if e != nil {
		t.Fatal(e)
	}
	client := Client{SDK: sdk, Budget: &Budget{}}
	if _, e = client.Generate(context.Background(), "one", map[string]string{"title": "안내"}); e != nil || calls != 1 {
		t.Fatal(calls, e)
	}
}
