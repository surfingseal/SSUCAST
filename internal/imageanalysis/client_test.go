package imageanalysis

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/schedulebatch"
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
func TestMockSDKInlineImageAndStructuredOutput(t *testing.T) {
	calls := 0
	hc := &http.Client{Transport: roundFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		if json.Unmarshal(b, &body) != nil || !strings.Contains(string(b), "inlineData") || !strings.Contains(string(b), "application/json") || !strings.Contains(r.URL.Path, Model) {
			t.Fatal("wrong multimodal wire")
		}
		r0 := Synthetic()
		response := genai.GenerateContentResponse{ModelVersion: Model, Candidates: []*genai.Candidate{{FinishReason: genai.FinishReasonStop, Content: &genai.Content{Parts: []*genai.Part{{Text: r0.Raw}}}}}}
		raw, _ := json.Marshal(response)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	sdk, e := genai.NewClient(context.Background(), &genai.ClientConfig{Backend: genai.BackendVertexAI, Project: "synthetic-project", Location: "global", HTTPClient: hc})
	if e != nil {
		t.Fatal(e)
	}
	resp, e := sdk.Models.GenerateContent(context.Background(), Model, []*genai.Content{{Role: "user", Parts: []*genai.Part{genai.NewPartFromText("synthetic"), genai.NewPartFromBytes([]byte("synthetic PNG bytes"), "image/png")}}}, Config())
	if e != nil || calls != 1 || resp.ModelVersion != Model {
		t.Fatal(calls, e)
	}
	if _, e = Decode(resp.Text()); e != nil {
		t.Fatal(e)
	}
}
func TestBudgetTwentyFiveAndNoSuccessReplay(t *testing.T) {
	ids := []string{}
	for i := 0; i < 20; i++ {
		ids = append(ids, fmt.Sprint(i))
	}
	b, e := schedulebatch.LoadBoundedBudget(t.TempDir(), ids, Model, 25)
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	b.Next = roundFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
	for i := 0; i < 25; i++ {
		b.ID = ids[i/3]
		req, _ := http.NewRequest("POST", "https://aiplatform.googleapis.com/v1/projects/p/locations/global/publishers/google/models/"+Model+":generateContent", nil)
		resp, e := b.RoundTrip(req)
		if e != nil {
			t.Fatal(e)
		}
		resp.Body.Close()
	}
	b.ID = ids[19]
	req, _ := http.NewRequest("POST", "https://aiplatform.googleapis.com/v1/projects/p/locations/global/publishers/google/models/"+Model+":generateContent", nil)
	if _, e = b.RoundTrip(req); e == nil || calls != 25 {
		t.Fatal("generation cap escaped")
	}
}

func TestSharedAdditionalBudgetStopsAtTwentyFive(t *testing.T) {
	prior := t.TempDir()
	dir := t.TempDir()
	a := []schedulebatch.Attempt{}
	for i := 0; i < 25; i++ {
		a = append(a, schedulebatch.Attempt{Number: i + 1, ID: "prior", Finished: true, HTTPStatus: 429})
	}
	raw, _ := json.Marshal(a)
	if e := os.WriteFile(filepath.Join(prior, "attempts.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	b := &schedulebatch.Budget{Attempts: []schedulebatch.Attempt{}}
	req, _ := http.NewRequest("POST", "https://aiplatform.googleapis.com", strings.NewReader(`{"contents":[{"parts":[{"inlineData":{"data":"AA==","mimeType":"image/png"}}]}]}`))
	if guard(dir, b, req, prior) == nil {
		t.Fatal("shared cap escaped")
	}
}
