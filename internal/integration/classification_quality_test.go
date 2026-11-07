package integration_test

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/classification"

	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"go.mongodb.org/mongo-driver/v2/bson"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

type unsupportedMock struct{}

func (unsupportedMock) Classify(context.Context, model.Content) (classification.Result, error) {
	return classification.Result{ModelRequests: 1, UsageKnown: true, InputTokens: 12, OutputTokens: 8, Labels: model.ClassificationLabels{PrimaryCategory: "academic", Categories: []string{"academic"}, ActivityTypes: []string{"internship"}, Topics: []string{}, TargetAudience: []string{"평점 3.5 이상"}, ClassificationStatus: "classified", ReasoningSummary: "MOCK adversarial unsupported claims", Evidence: []model.ClassificationEvidence{}}}, nil
}
func TestQualityGuardPreservesRawAndOriginalLabelsInMongoAPI(t *testing.T) {
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("set TEST_MONGODB_URI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := "crawltest_" + bson.NewObjectID().Hex()
	s, err := repository.Open(ctx, uri, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Client.Database(db).Drop(c); err != nil {
			t.Error(err)
		}
		if err := s.Client.Disconnect(c); err != nil {
			t.Error(err)
		}
	}()
	source := model.Source{ID: bson.NewObjectID(), Name: "department_synthetic", CrawlerType: "example", BaseURL: "https://department.example.invalid", ClassificationStrategy: "ai"}
	if err := s.ApplySources(ctx, []model.Source{source}); err != nil {
		t.Fatal(err)
	}
	input := departmentFixture(t)
	input.SourceID = source.ID
	input.Metadata["has_images"] = "true"
	input.Attachments = []model.Attachment{{Name: "synthetic.pdf", URL: "https://department.example.invalid/synthetic.pdf"}}
	if err := s.Save(ctx, []model.Content{input}); err != nil {
		t.Fatal(err)
	}
	items, err := s.Contents(ctx, model.Filter{Page: 1, Limit: 20})
	if err != nil || len(items) != 1 {
		t.Fatal(err)
	}
	input = items[0]
	record, err := s.ClassifyOne(ctx, input, unsupportedMock{}, "mock-quality-v2")
	if err != nil {
		t.Fatal(err)
	}
	if len(record.ActivityTypes) != 0 || len(record.TargetAudience) != 0 || record.RawLabels == nil || len(record.RawLabels.ActivityTypes) != 1 || record.Quality == nil || !record.Quality.ImagesUnanalysed || !record.Quality.AttachmentsUnanalysed || len(record.Quality.RejectedClaims) != 2 || record.ClassificationStatus != "needs_review" {
		t.Fatal(record)
	}
	original, err := s.Content(ctx, input.ID)
	if err != nil || original.Category != "unknown" || original.OriginalCategory != "" {
		t.Fatal("school category overwritten", original, err)
	}
	const token = "quality-test-bearer-token-123456789"
	web := httptest.NewServer((api.API{Store: s, Token: token}).Handler())
	defer web.Close()
	req, _ := http.NewRequestWithContext(ctx, "GET", web.URL+"/api/v1/contents/"+input.ID.Hex()+"/classifications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := web.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var got struct {
		Items []model.Classification `json:"items"`
	}
	err = json.NewDecoder(response.Body).Decode(&got)
	if err != nil || response.StatusCode != 200 || len(got.Items) != 1 || got.Items[0].RawLabels == nil || got.Items[0].Quality == nil || len(got.Items[0].ActivityTypes) != 0 || got.Items[0].InputTokens != 12 || got.Items[0].OutputTokens != 8 || got.Items[0].ModelRequests != 1 {
		t.Fatal(got, err)
	}
}
