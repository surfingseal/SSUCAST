package scheduleflash

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/schedulebatch"
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
	s := []string{}
	for i := 0; i < 76; i++ {
		s = append(s, fmt.Sprintf("%024x", i+1))
	}
	return s
}
func req() *http.Request {
	r, _ := http.NewRequest("POST", "https://aiplatform.googleapis.com/v1/projects/p/locations/global/publishers/google/models/"+Model+":generateContent", strings.NewReader(`{"generationConfig":{"maxOutputTokens":16384,"thinkingConfig":{"thinkingLevel":"MEDIUM"}}}`))
	return r
}
func TestFlashGuardReservesBeforeSendingAndEnforcesThreeDollarCap(t *testing.T) {
	dir := t.TempDir()
	b, e := schedulebatch.LoadBudget(dir, ids())
	if e != nil {
		t.Fatal(e)
	}
	b.ExpectedModel = Model
	b.ID = ids()[0]
	b.BeforeSend = func(r *http.Request) error { return Guard(dir, b, r) }
	if e = Guard(dir, b, req()); e != nil {
		t.Fatal(e)
	}
	b.Attempts = []schedulebatch.Attempt{{Number: 1, ID: ids()[0], Finished: true, HTTPStatus: 200}}
	payload, _ := json.Marshal(genai.GenerateContentResponse{UsageMetadata: &genai.GenerateContentResponseUsageMetadata{CandidatesTokenCount: 795000}})
	if scheduleexperiment.Write(filepath.Join(dir, "responses", "001.json"), schedulebatch.SavedHTTP{Status: 200, Body: string(payload)}) != nil {
		t.Fatal("write")
	}
	calls := 0
	b.ID = ids()[1]
	b.Next = roundFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, fmt.Errorf("must not send") })
	if _, e = b.RoundTrip(req()); e == nil || calls != 0 || len(b.Attempts) != 1 {
		t.Fatal("cost cap transmitted/journaled paid request", e, calls)
	}
}
func TestFlashGuardRejectsWrongThinkingAndMissingUsage(t *testing.T) {
	dir := t.TempDir()
	b, _ := schedulebatch.LoadBudget(dir, ids())
	r := req()
	data, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(strings.NewReader(strings.Replace(string(data), "MEDIUM", "HIGH", 1)))
	if Guard(dir, b, r) == nil {
		t.Fatal("thinking configuration drift")
	}
	b.Attempts = []schedulebatch.Attempt{{Number: 1, ID: ids()[0], Finished: true, HTTPStatus: 200}}
	scheduleexperiment.Write(filepath.Join(dir, "responses", "001.json"), schedulebatch.SavedHTTP{Status: 200, Body: `{}`})
	if Guard(dir, b, req()) == nil {
		t.Fatal("unknown paid usage continued")
	}
}
func TestFlashSDKUsesSamePromptSchemaAndMediumWithoutFallback(t *testing.T) {
	calls := 0
	hc := &http.Client{Transport: roundFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		data, _ := io.ReadAll(r.Body)
		var body map[string]json.RawMessage
		json.Unmarshal(data, &body)
		if !strings.HasSuffix(r.URL.Path, "/"+Model+":generateContent") || !strings.Contains(string(data), `"thinkingLevel":"MEDIUM"`) || !strings.Contains(string(data), `"responseMimeType":"application/json"`) {
			t.Error("wire configuration")
		}
		var gc struct {
			ResponseSchema *genai.Schema `json:"responseSchema"`
		}
		json.Unmarshal(body["generationConfig"], &gc)
		if schedulebatch.Hash(gc.ResponseSchema) != schedulebatch.Hash(schedulebatch.Schema()) {
			t.Error("schema drift")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"modelVersion":"gemini-3.8-flash","candidates":[{"content":{"parts":[{"text":"{}"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20,"thoughtsTokenCount":30,"totalTokenCount":150}}`))}, nil
	})}
	sdk, e := genai.NewClient(context.Background(), &genai.ClientConfig{Backend: genai.BackendVertexAI, Project: "test", Location: "global", HTTPClient: hc, HTTPOptions: genai.HTTPOptions{BaseURL: "https://aiplatform.googleapis.com/", APIVersion: "v1"}})
	if e != nil {
		t.Fatal(e)
	}
	resp, e := sdk.Models.GenerateContent(context.Background(), Model, genai.Text("notice"), Config())
	if e != nil || calls != 1 || resp.UsageMetadata.ThoughtsTokenCount != 30 {
		t.Fatal(e, calls)
	}
	if Cost(resp.UsageMetadata) != 0.0002625 {
		t.Fatal("reasoning tokens not costed", Cost(resp.UsageMetadata))
	}
}
func TestFlashWrongModelRejectedBeforeTransport(t *testing.T) {
	b, _ := schedulebatch.LoadBudget(t.TempDir(), ids())
	b.ExpectedModel = Model
	b.ID = ids()[0]
	r := req()
	r.URL.Path = strings.Replace(r.URL.Path, Model, schedulebatch.Model, 1)
	if _, e := b.RoundTrip(r); e == nil || len(b.Attempts) != 0 {
		t.Fatal("fallback allowed")
	}
}
func TestFlashLiteCorpusProvenance(t *testing.T) {
	// Production verifier must reject an incomplete/corrupt original experiment;
	// no synthetic response is ever a substitute for the frozen actual 76.
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "results.json"), []byte(`[]`), 0600)
	if VerifyLite(d, schedulebatch.Selection{}) == nil {
		t.Fatal("incomplete corpus accepted")
	}
}
