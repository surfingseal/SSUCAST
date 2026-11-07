package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/fixture"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/service"
)

func TestMultiSourceAPIWithMongo(t *testing.T) {
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("set TEST_MONGODB_URI for MongoDB end-to-end test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db := "crawltest_" + bson.NewObjectID().Hex()
	store, err := repository.Open(ctx, uri, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := store.Client.Database(db).Drop(ctx); err != nil {
			t.Error(err)
		}
		if err := store.Client.Disconnect(ctx); err != nil {
			t.Error(err)
		}
	}()
	web := httptest.NewServer(fixture.Handler())
	defer web.Close()
	no := false
	sources := []model.Source{{ID: bson.NewObjectID(), Name: "first", CrawlerType: "example", BaseURL: web.URL, ListURL: "/", Enabled: true, AutoEnabled: &no, Schedule: "* * * * *", RequestIntervalMS: 1, MaxPages: 1, MaxContents: 1, MaxRequests: 5}, {ID: bson.NewObjectID(), Name: "second", CrawlerType: "example", BaseURL: web.URL, ListURL: "/?page=2", Enabled: true, AutoEnabled: &no, Schedule: "* * * * *", RequestIntervalMS: 1, MaxPages: 1, MaxContents: 1, MaxRequests: 5}, {ID: bson.NewObjectID(), Name: "budget", CrawlerType: "example", BaseURL: web.URL, Enabled: true, AutoEnabled: &no, RequestIntervalMS: 1, MaxRequests: 1}}
	if err := store.ApplySources(ctx, sources); err != nil {
		t.Fatal(err)
	}
	fetcher := crawler.NewFetcher(time.Second)
	fetcher.FixtureOrigin = web.URL
	registry, err := crawler.DefaultRegistry(fetcher)
	if err != nil {
		t.Fatal(err)
	}
	runner := service.NewWithRegistry(store, registry, 2, 3*time.Second)
	defer runner.Stop()
	server := httptest.NewServer((api.API{Store: store, Runner: runner, Token: "integration-token"}).Handler())
	defer server.Close()
	request := func(method, path string, body any, target any) int {
		t.Helper()
		var data bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&data).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, server.URL+path, &data)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer integration-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode
	}
	for i, source := range sources {
		var job model.Job
		if status := request("POST", "/api/v1/jobs", map[string]string{"source_id": source.ID.Hex()}, &job); status != 202 {
			t.Fatalf("start %d %+v", status, job)
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			if request("GET", "/api/v1/jobs/"+job.ID.Hex(), nil, &job) != 200 {
				t.Fatal("job not found")
			}
			if job.Status != "running" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("job did not finish")
			}
			time.Sleep(5 * time.Millisecond)
		}
		if i < 2 {
			if job.Status != "succeeded" || job.CollectedCount != 1 {
				t.Fatal(job)
			}
		} else if job.Status != "failed" || job.ErrorCode != "request_limit" || job.ErrorStage != "fetch" {
			t.Fatal("structured failure lost", job)
		}
	}
	for _, source := range sources[:2] {
		var result struct {
			Items []model.Content `json:"items"`
		}
		if request("GET", "/api/v1/contents?source_id="+source.ID.Hex(), nil, &result) != 200 || len(result.Items) != 1 || result.Items[0].SourceID != source.ID {
			t.Fatal(result)
		}
	}
}
