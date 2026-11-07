package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var strategyDBNames sync.Map

type strategyMock struct {
	status           string
	calls            atomic.Int32
	category         string
	started, release chan struct{}
}

func (m *strategyMock) Classify(ctx context.Context, _ model.Content) (classification.Result, error) {
	m.calls.Add(1)
	if m.started != nil {
		close(m.started)
		select {
		case <-m.release:
		case <-ctx.Done():
			return classification.Result{}, ctx.Err()
		}
	}
	category := m.category
	if category == "" {
		category = "academic"
	}
	return classification.Result{ModelRequests: 1, UsageKnown: true, InputTokens: 7, OutputTokens: 3, Labels: model.ClassificationLabels{Evidence: []model.ClassificationEvidence{}, PrimaryCategory: category, Categories: []string{category}, ActivityTypes: []string{}, Topics: []string{}, TargetAudience: []string{}, ClassificationStatus: func() string {
		if m.status != "" {
			return m.status
		}
		return "classified"
	}(), ReasoningSummary: "MOCK only: explicit strategy test"}}, nil
}
func strategyStore(t *testing.T) *repository.Store {
	t.Helper()
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("set TEST_MONGODB_URI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db := "crawltest_" + bson.NewObjectID().Hex()
	s, err := repository.Open(ctx, uri, db)
	if err != nil {
		t.Fatal(err)
	}
	strategyDBNames.Store(s, db)
	t.Cleanup(func() {
		strategyDBNames.Delete(s)
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Client.Database(db).Drop(c); err != nil {
			t.Error(err)
		}
		if err := s.Client.Disconnect(c); err != nil {
			t.Error(err)
		}
	})
	return s
}
func saveStrategyNotice(t *testing.T, s *repository.Store, source model.Source, item model.Content) model.Content {
	t.Helper()
	ctx := context.Background()
	if err := s.ApplySources(ctx, []model.Source{source}); err != nil {
		t.Fatal(err)
	}
	item.SourceID = source.ID
	normalized, err := model.NormalizeContentURL(item.OriginalURL)
	if err != nil {
		t.Fatal(err)
	}
	item.OriginalURL = normalized
	if err := s.Save(ctx, []model.Content{item}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Contents(ctx, model.Filter{SourceID: &source.ID, Page: 1, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.OriginalURL == item.OriginalURL {
			return row
		}
	}
	t.Fatal("saved notice absent")
	return item
}
func TestActualSSUSourceStrategyNeverInvokesAI(t *testing.T) {
	s := strategyStore(t)
	ctx := context.Background()
	source := model.Source{ID: bson.NewObjectID(), Name: "official-board-arbitrary-name", CrawlerType: "ssu_catch", BaseURL: crawler.SSUCatchOrigin, ClassificationStrategy: "source"}
	item, err := crawler.ReadSSULocalFixture("../crawler/testdata/ssu_catch")
	if err != nil {
		t.Fatal(err)
	}
	item = saveStrategyNotice(t, s, source, item)
	mock := &strategyMock{}
	result, err := s.ClassifyOne(ctx, item, mock, "must-not-be-used")
	if err != nil || mock.calls.Load() != 0 || result.ClassificationMethod != "source" {
		t.Fatal(result, err, mock.calls.Load())
	}
	got, err := s.Content(ctx, item.ID)
	if err != nil || got.Category != "scholarship" || got.OriginalCategory != "장학" || got.ClassificationMethod != "source" || got.ModelVersion != "" {
		t.Fatal(got, err)
	}
	history, err := s.Classifications(ctx, item.ID)
	if err != nil || len(history) != 0 {
		t.Fatal(history, err)
	}
	// A synthetic unrecognized category outside the requested eight is retained as raw data,
	// queued for review, and still never sent to AI under source strategy.
	item.OriginalURL = "https://scatch.ssu.ac.kr/공지사항/?slug=synthetic-unknown"
	item.OriginalCategory = "미등록분류"
	item.OriginalCategories = []string{"미등록분류"}
	item = saveStrategyNotice(t, s, source, item)
	result, err = s.ClassifyOne(ctx, item, mock, "unused")
	if err != nil || result.PrimaryCategory != "unknown" || result.ClassificationStatus != "needs_review" || mock.calls.Load() != 0 {
		t.Fatal(result, err)
	}
	t.Log("actual SSU HTML source strategy: official scholarship; unsupported raw category retained; AI calls=0")
}
func TestDepartmentStrategiesCacheRevisionAndCategoryAPI(t *testing.T) {
	s := strategyStore(t)
	ctx := context.Background()
	source := model.Source{ID: bson.NewObjectID(), Name: "uncategorized-board", CrawlerType: "example", BaseURL: "https://department.example.invalid", ClassificationStrategy: "ai"}
	item := saveStrategyNotice(t, s, source, departmentFixture(t))
	mock := &strategyMock{}
	first, err := s.ClassifyOne(ctx, item, mock, "mock-model")
	if err != nil || !first.Applied {
		t.Fatal(first, err)
	}
	cached, err := s.ClassifyOne(ctx, item, mock, "mock-model")
	if err != nil || !cached.Reused || mock.calls.Load() != 1 {
		t.Fatal(cached, err, mock.calls.Load())
	}
	before, err := s.Content(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Same recrawl keeps the AI result and content revision timestamps.
	if err = s.Save(ctx, []model.Content{before}); err != nil {
		t.Fatal(err)
	}
	same, _ := s.Content(ctx, item.ID)
	if same.Category != "academic" || same.ModelVersion != "mock-model" || !same.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal(same)
	}
	modified := same
	modified.Content += " 추가된 학사 일정 안내입니다."
	modified.UpdatedAt = time.Now().UTC()
	if err = s.Save(ctx, []model.Content{modified}); err != nil {
		t.Fatal(err)
	}
	pending, _ := s.Content(ctx, item.ID)
	if pending.Category != "unknown" || pending.ModelVersion != "" || pending.ContentHash == before.ContentHash {
		t.Fatal("modified notice retained stale AI", pending)
	}
	second, err := s.ClassifyOne(ctx, pending, mock, "mock-model")
	if err != nil || !second.Applied || mock.calls.Load() != 2 {
		t.Fatal(second, err)
	}
	source.ClassificationStrategy = "source"
	if err = s.ApplySources(ctx, []model.Source{source}); err != nil {
		t.Fatal(err)
	}
	official, _ := s.Content(ctx, item.ID)
	if official.Category != "unknown" || official.ClassificationMethod != "source" || official.ModelVersion != "" {
		t.Fatal(official)
	}
	if _, err = s.ClassifyOne(ctx, official, mock, "unused"); err != nil || mock.calls.Load() != 2 {
		t.Fatal(err)
	}
	source.ClassificationStrategy = "ai"
	if err = s.ApplySources(ctx, []model.Source{source}); err != nil {
		t.Fatal(err)
	}
	third, err := s.ClassifyOne(ctx, official, mock, "mock-model")
	if err != nil || third.InputHash == second.InputHash || mock.calls.Load() != 3 {
		t.Fatal(third, err)
	}
	history, _ := s.Classifications(ctx, item.ID)
	if len(history) != 3 {
		t.Fatal(history)
	}
	for _, r := range history {
		if r.ID != third.ID && r.Applied {
			t.Fatal("old history still active", r)
		}
	}
	const token = "strategy-api-local-token-0123456789"
	web := httptest.NewServer((api.API{Store: s, Token: token}).Handler())
	defer web.Close()
	req, _ := http.NewRequest("GET", web.URL+"/api/v1/contents?category=academic", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := web.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got struct {
		Items []model.Content `json:"items"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&got); err != nil || resp.StatusCode != 200 || len(got.Items) != 1 || got.Items[0].ClassificationMethod != "ai" {
		t.Fatal(got, err)
	}
	t.Log("department synthetic: same input cached, modified body reclassified, source blocks AI, strategy cycle causes new policy reservation, API=200")
}
func TestHybridUsesValidCategoryAndAIOnlyForGenericLabels(t *testing.T) {
	s := strategyStore(t)
	ctx := context.Background()
	source := model.Source{ID: bson.NewObjectID(), Name: "hybrid-board", BaseURL: "https://department.example.invalid", ClassificationStrategy: "hybrid"}
	raw := departmentFixture(t)
	raw.OriginalCategory = "교직"
	item := saveStrategyNotice(t, s, source, raw)
	mock := &strategyMock{category: "teaching"}
	result, err := s.ClassifyOne(ctx, item, mock, "mock")
	if err != nil || result.PrimaryCategory != "teaching" || mock.calls.Load() != 0 {
		t.Fatal(result, err)
	}
	raw.OriginalURL += "-generic"
	raw.OriginalCategory = "일반"
	item = saveStrategyNotice(t, s, source, raw)
	result, err = s.ClassifyOne(ctx, item, mock, "mock")
	if err != nil || result.PrimaryCategory != "teaching" || mock.calls.Load() != 1 || !result.Applied {
		t.Fatal(result, err)
	}
}
func TestStrategyChangeDuringGenerationPreventsStaleProjection(t *testing.T) {
	s := strategyStore(t)
	ctx := context.Background()
	source := model.Source{ID: bson.NewObjectID(), Name: "race-board", BaseURL: "https://department.example.invalid", ClassificationStrategy: "ai"}
	input := departmentFixture(t)
	input.OriginalCategory = "학사"
	item := saveStrategyNotice(t, s, source, input)
	mock := &strategyMock{started: make(chan struct{}), release: make(chan struct{})}
	done := make(chan model.Classification, 1)
	errs := make(chan error, 1)
	go func() { r, err := s.ClassifyOne(ctx, item, mock, "mock"); done <- r; errs <- err }()
	select {
	case <-mock.started:
	case <-time.After(5 * time.Second):
		t.Fatal("mock did not start")
	}
	source.ClassificationStrategy = "source"
	if err := s.ApplySources(ctx, []model.Source{source}); err != nil {
		t.Fatal(err)
	}
	close(mock.release)
	result := <-done
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if result.Applied || result.ApplicationStatus != "superseded" {
		t.Fatal(result)
	}
	final, err := s.Content(ctx, item.ID)
	if err != nil || final.Category != "academic" || final.ClassificationMethod != "source" || final.ModelVersion != "" {
		t.Fatal(final, err)
	}
}

func TestEveryOfficialCategoryAndUnknownCanBeQueried(t *testing.T) {
	s := strategyStore(t)
	ctx := context.Background()
	source := model.Source{ID: bson.NewObjectID(), Name: "all-eight", BaseURL: "https://department.example.invalid", ClassificationStrategy: "ai"}
	for _, category := range append(append([]string{}, model.OfficialCategories...), "unknown") {
		input := departmentFixture(t)
		input.OriginalURL = "https://department.example.invalid/" + category
		item := saveStrategyNotice(t, s, source, input)
		mock := &strategyMock{category: category}
		if category == "unknown" {
			mock.status = "needs_review"
		}
		result, err := s.ClassifyOne(ctx, item, mock, "mock-eight")
		if err != nil || !result.Applied || result.PrimaryCategory != category || mock.calls.Load() != 1 {
			t.Fatal(category, result, err)
		}
		rows, err := s.Contents(ctx, model.Filter{SourceID: &source.ID, Category: category, Page: 1, Limit: 100})
		if err != nil || len(rows) != 1 || rows[0].Category != category {
			t.Fatal(category, rows, err)
		}
	}
}

func TestExcludedCategoriesNotSavedClassifiedOrReturned(t *testing.T) {
	s := strategyStore(t)
	ctx := context.Background()
	source := model.Source{ID: bson.NewObjectID(), Name: "exclusion-test", BaseURL: "https://department.example.invalid", ClassificationStrategy: "ai"}
	if err := s.ApplySources(ctx, []model.Source{source}); err != nil {
		t.Fatal(err)
	}
	for _, category := range []string{"채용", "교원채용"} {
		item := departmentFixture(t)
		item.SourceID = source.ID
		item.OriginalCategory = category
		if err := s.Save(ctx, []model.Content{item}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.Contents(ctx, model.Filter{Page: 1, Limit: 100})
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	// A legacy excluded row is hidden rather than destructively deleted.
	item := departmentFixture(t)
	item.ID = bson.NewObjectID()
	item.SourceID = source.ID
	item.OriginalCategory = "교원채용"
	item.Category = "unknown"
	if _, err = s.Client.Database(sourcesDatabaseName(t, s)).Collection("crawled_contents").InsertOne(ctx, item); err != nil {
		t.Fatal(err)
	}
	rows, err = s.Contents(ctx, model.Filter{Page: 1, Limit: 100})
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	mock := &strategyMock{}
	if _, err = s.ClassifyOne(ctx, item, mock, "unused"); err == nil || mock.calls.Load() != 0 {
		t.Fatal("excluded legacy row classified", err, mock.calls.Load())
	}
	if _, err = s.Content(ctx, item.ID); err == nil {
		t.Fatal("excluded detail exposed")
	}
}

func sourcesDatabaseName(t *testing.T, s *repository.Store) string {
	t.Helper()
	v, ok := strategyDBNames.Load(s)
	if !ok {
		t.Fatal("test database missing")
	}
	return v.(string)
}
