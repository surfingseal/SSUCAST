package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/classification"

	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type mockClassifier struct {
	calls atomic.Int32
	fail  bool
}

func (m *mockClassifier) Classify(context.Context, model.Content) (classification.Result, error) {
	m.calls.Add(1)
	if m.fail {
		return classification.Result{ModelRequests: 1}, errors.New("mock API failure")
	}
	return classification.Result{ModelRequests: 1, UsageKnown: true, Labels: model.ClassificationLabels{PrimaryCategory: "academic", Categories: []string{"academic"}, ActivityTypes: []string{}, Topics: []string{}, TargetAudience: []string{}, ClassificationStatus: "classified", ReasoningSummary: "MOCK ONLY: 합성 학과 학사 안내", Evidence: []model.ClassificationEvidence{}}, ResolvedModel: "mock-response-version", InputTokens: 10, OutputTokens: 20}, nil
}
func TestDepartmentMockClassificationMongoAPI(t *testing.T) {
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("set TEST_MONGODB_URI for classification MongoDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := "crawltest_" + bson.NewObjectID().Hex()
	store, err := repository.Open(ctx, uri, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := store.Client.Database(db).Drop(c); err != nil {
			t.Error(err)
		}
		if err := store.Client.Disconnect(c); err != nil {
			t.Error(err)
		}
	}()
	source := model.Source{ID: bson.NewObjectID(), Name: "department_synthetic", CrawlerType: "example", BaseURL: "https://department.example.invalid", ClassificationStrategy: "ai"}
	if err := store.ApplySources(ctx, []model.Source{source}); err != nil {
		t.Fatal(err)
	}
	item := departmentFixture(t)
	item.SourceID = source.ID
	if err := store.Save(ctx, []model.Content{item}); err != nil {
		t.Fatal(err)
	}
	items, err := store.Contents(ctx, model.Filter{Page: 1, Limit: 20})
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	item = items[0]
	mock := &mockClassifier{}
	var wg sync.WaitGroup
	var successes atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.ClassifyOne(ctx, item, mock, "mock-gemini"); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if mock.calls.Load() != 1 || successes.Load() < 1 {
		t.Fatal("duplicate generation", mock.calls.Load(), successes.Load())
	}
	rows, err := store.Classifications(ctx, item.ID)
	if err != nil || len(rows) != 1 || rows[0].ClassifiedAt == nil || rows[0].PrimaryCategory != "academic" || rows[0].InputHash != classification.PolicyInputHash(source, item) || rows[0].ModelVersion != "mock-gemini" || rows[0].Quality == nil || rows[0].RawLabels == nil || rows[0].ModelRequests != 1 || !rows[0].UsageKnown {
		t.Fatal(rows, err)
	}
	failing := &mockClassifier{fail: true}
	if _, err := store.ClassifyOne(ctx, item, failing, "mock-failing"); err == nil {
		t.Fatal("failed classification accepted")
	}
	if _, err := store.ClassifyOne(ctx, item, failing, "mock-failing"); err == nil || failing.calls.Load() != 1 {
		t.Fatal("failure retried")
	}
	rows, err = store.Classifications(ctx, item.ID)
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	failed := 0
	for _, r := range rows {
		if r.ModelVersion == "mock-failing" {
			if r.ClassificationStatus != "failed" || r.ClassifiedAt != nil || r.PrimaryCategory != "" || r.ModelRequests != 1 || r.UsageKnown {
				t.Fatal("fabricated successful result", r)
			}
			failed++
		}
	}
	if failed != 1 {
		t.Fatal("failed attempt absent")
	}
	const token = "mock-classification-api-test-token"
	web := httptest.NewServer((api.API{Store: store, Token: token}).Handler())
	defer web.Close()
	path := "/api/v1/contents/" + item.ID.Hex() + "/classifications"
	for _, auth := range []bool{true, false} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, web.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if auth {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := web.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if !auth {
			resp.Body.Close()
			if resp.StatusCode != 401 {
				t.Fatal(resp.StatusCode)
			}
			continue
		}
		var got struct {
			Items []model.Classification `json:"items"`
		}
		err = json.NewDecoder(resp.Body).Decode(&got)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 || len(got.Items) != 2 {
			t.Fatal(resp.StatusCode, got, err)
		}
	}
	t.Log("MOCK GEMINI + SYNTHETIC DEPARTMENT: calls=1 across 8 contenders, API=200/401, failure retained without success labels, real Vertex generations=0")
}

func departmentFixture(t *testing.T) model.Content {
	t.Helper()
	raw, err := os.ReadFile("../fixture/department/notice.synthetic.json")
	if err != nil {
		t.Fatal(err)
	}
	var item model.Content
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	return item
}
