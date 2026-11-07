package scheduleexperiment

import (
	"cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/auth/httptransport"
	"context"
	"encoding/json"
	"errors"
	"example.com/crawlserver/internal/classification"
	"fmt"
	"google.golang.org/genai"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Attempt struct {
	Number          int       `json:"number"`
	ID              string    `json:"id"`
	StartedAt       time.Time `json:"started_at"`
	HTTPStatus      int       `json:"http_status"`
	TransportFailed bool      `json:"transport_failed"`
}

// Budget sits below the SDK: hidden SDK/auth retries cannot exceed twelve.
// Every outbound generation is durably recorded before the HTTP request.
type Budget struct {
	mu       sync.Mutex
	Next     http.RoundTripper
	Path     string
	ID       string
	Attempts []Attempt
	perID    map[string]int
}

func (b *Budget) RoundTrip(r *http.Request) (*http.Response, error) {
	if !strings.HasSuffix(r.URL.Path, ":generateContent") {
		return b.Next.RoundTrip(r)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/publishers/google/models/"+Model+":generateContent") || len(b.Attempts) >= 12 || b.ID == "" {
		return nil, fmt.Errorf("fixed model/call budget rejected")
	}
	if b.perID == nil {
		b.perID = map[string]int{}
	}
	if b.perID[b.ID] > 0 {
		last := b.Attempts[len(b.Attempts)-1]
		if last.ID != b.ID || (last.HTTPStatus != 429 && last.HTTPStatus < 500) || last.HTTPStatus > 599 {
			return nil, fmt.Errorf("only explicit 429/5xx retries allowed")
		}
	}
	if _, ok := b.perID[b.ID]; !ok && len(b.perID) >= 10 {
		return nil, fmt.Errorf("notice budget exhausted")
	}
	b.perID[b.ID]++
	b.Attempts = append(b.Attempts, Attempt{Number: len(b.Attempts) + 1, ID: b.ID, StartedAt: time.Now().UTC()})
	if e := Write(b.Path, b.Attempts); e != nil {
		return nil, e
	}
	resp, err := b.Next.RoundTrip(r)
	last := &b.Attempts[len(b.Attempts)-1]
	last.TransportFailed = err != nil
	if resp != nil {
		last.HTTPStatus = resp.StatusCode
	}
	if e := Write(b.Path, b.Attempts); e != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, e
	}
	return resp, err
}

type Client struct {
	SDK    *genai.Client
	HTTP   *http.Client
	Budget *Budget
	Config classification.Config
}

func NewClient(ctx context.Context, c classification.Config, dir string) (*Client, error) {
	if c.Provider != "vertexai" || c.AutoEnabled || c.Location != "global" {
		return nil, fmt.Errorf("manual Vertex global configuration required")
	}
	if e := classification.ValidateServiceAccount(c.CredentialsFile, c.Project); e != nil {
		return nil, e
	}
	cred, e := credentials.DetectDefault(&credentials.DetectOptions{Scopes: []string{"https://www.googleapis.com/auth/cloud-platform"}, CredentialsFile: c.CredentialsFile})
	if e != nil {
		return nil, fmt.Errorf("ADC configuration failed")
	}
	b := &Budget{Next: http.DefaultTransport, Path: filepath.Join(dir, "attempts.json")}
	hc, e := httptransport.NewClient(&httptransport.Options{Credentials: cred, BaseRoundTripper: b, DisableTelemetry: true})
	if e != nil {
		return nil, fmt.Errorf("ADC transport failed")
	}
	hc.Timeout = c.Timeout
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	sdk, e := genai.NewClient(ctx, &genai.ClientConfig{Backend: genai.BackendVertexAI, Project: c.Project, Location: c.Location, Credentials: cred, HTTPClient: hc, HTTPOptions: genai.HTTPOptions{BaseURL: "https://aiplatform.googleapis.com/", APIVersion: "v1", RetryOptions: &genai.HTTPRetryOptions{Attempts: genai.Ptr[int32](1)}}})
	if e != nil {
		return nil, fmt.Errorf("Vertex SDK initialization failed")
	}
	return &Client{sdk, hc, b, c}, nil
}
func Status(err error) int {
	var e genai.APIError
	if errors.As(err, &e) {
		return e.Code
	}
	return 0
}

// Model metadata GET is read-only and does not invoke prediction or generation.
func (c *Client) Preflight(ctx context.Context) map[string]any {
	out := map[string]any{"checked_at": time.Now().UTC(), "project": c.Config.Project, "location": c.Config.Location, "requested_model": Model, "sdk": "google.golang.org/genai@v1.77.0", "authentication": "validated service account; ADC", "generation_calls": 0}
	m, e := c.SDK.Models.Get(ctx, Model, nil)
	if e != nil {
		out["model_metadata_http_status"] = Status(e)
		out["model_metadata_error"] = classification.ErrorCode(e)
		out["model_metadata_accessible"] = false
	} else {
		out["model_metadata_accessible"] = true
		out["model_name"] = m.Name
	}
	// Management permissions are distinct from model prediction permission.
	for key, url := range map[string]string{"vertex_api": "https://serviceusage.googleapis.com/v1/projects/" + c.Config.Project + "/services/aiplatform.googleapis.com", "project_identity": "https://cloudresourcemanager.googleapis.com/v1/projects/" + c.Config.Project} {
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			out[key+"_read"] = "authentication_or_network_error"
			continue
		}
		out[key+"_http_status"] = resp.StatusCode
		var data map[string]any
		json.NewDecoder(resp.Body).Decode(&data)
		resp.Body.Close()
		if resp.StatusCode == 200 {
			if key == "vertex_api" {
				out["vertex_api_state"] = data["state"]
			} else {
				out["verified_project_id"] = data["projectId"]
			}
		}
	}
	out["management_permission_note"] = "관리 API 403은 모델 호출 권한과 별개다. 서비스 계정 project 일치와 모델 메타데이터 200을 확인하고 첫 표본 요청으로 실제 생성 권한/API 활성화를 확인한다. 추가 유료 probe 없음."
	return out
}
func (c *Client) Generate(ctx context.Context, id string, input any) (*genai.GenerateContentResponse, error) {
	c.Budget.ID = id
	raw, e := json.Marshal(input)
	if e != nil {
		return nil, e
	}
	return c.SDK.Models.GenerateContent(ctx, Model, genai.Text(string(raw)), &genai.GenerateContentConfig{SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: Instruction}}}, ResponseMIMEType: "application/json", ResponseSchema: Schema(), MaxOutputTokens: 8192, CandidateCount: 1, AutomaticContinuation: genai.Ptr(false), HTTPOptions: &genai.HTTPOptions{RetryOptions: &genai.HTTPRetryOptions{Attempts: genai.Ptr[int32](1)}}})
}
func Write(path string, v any) error {
	raw, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".experiment-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(append(raw, '\n'))
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, path)
}
