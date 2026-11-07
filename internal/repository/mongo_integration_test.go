package repository

import (
	"context"
	"errors"
	"example.com/crawlserver/internal/config"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"example.com/crawlserver/internal/model"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("set TEST_MONGODB_URI to run real MongoDB integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	name := "crawltest_" + bson.NewObjectID().Hex()
	if err := config.ValidateIsolatedTestMongo(uri, name); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, uri, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.Client.Database(name).Drop(ctx); err != nil {
			t.Error(err)
		}
		if err := s.Client.Disconnect(ctx); err != nil {
			t.Error(err)
		}
	})
	return s
}
func TestMongoUpsertFiltersAndIndexes(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Seed(ctx, "https://example.com", "0 */6 * * *"); err != nil {
		t.Fatal(err)
	}
	sources, err := s.Sources(ctx)
	if err != nil || len(sources) != 1 {
		t.Fatal(sources, err)
	}
	id := sources[0].ID
	now := time.Now().UTC().Truncate(time.Millisecond)
	items := []model.Content{{SourceID: id, Title: "first", Content: "text", OriginalURL: "https://example.com/1", Category: "academic", Tags: []string{}, CrawledAt: now, UpdatedAt: now, Metadata: map[string]string{"company": "example"}}, {SourceID: id, Title: "second", Content: "text", OriginalURL: "https://example.com/2", Category: "extracurricular", CrawledAt: now.Add(-time.Hour), UpdatedAt: now}}
	if err := s.Save(ctx, items); err != nil {
		t.Fatal(err)
	}
	all, err := s.Contents(ctx, model.Filter{Page: 1, Limit: 20})
	if err != nil || len(all) != 2 {
		t.Fatal(all, err)
	}
	firstID := all[0].ID
	items[0].Title = "updated"
	if err := s.Save(ctx, items); err != nil {
		t.Fatal(err)
	}
	all, err = s.Contents(ctx, model.Filter{Page: 1, Limit: 20})
	if err != nil || len(all) != 2 || all[0].Title != "updated" || all[0].ID != firstID {
		t.Fatal(all, err)
	}
	filtered, err := s.Contents(ctx, model.Filter{Page: 1, Limit: 1, SourceID: &id, Category: "academic", From: &now})
	if err != nil || len(filtered) != 1 {
		t.Fatal(filtered, err)
	}
	page, err := s.Contents(ctx, model.Filter{Page: 2, Limit: 1})
	if err != nil || len(page) != 1 || page[0].Title != "second" {
		t.Fatal(page, err)
	}
	// Verify the unique index exists by attempting a raw duplicate insert.
	_, err = s.contents.InsertOne(ctx, items[0])
	if err == nil {
		t.Fatal("unique URL index did not reject duplicate")
	}
}
func TestMongoConcurrentJobsAndRecovery(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Seed(ctx, "https://example.com", "* * * * *"); err != nil {
		t.Fatal(err)
	}
	sources, _ := s.Sources(ctx)
	id := sources[0].ID
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.Start(ctx, model.Job{ID: bson.NewObjectID(), SourceID: id, Status: "running", StartedAt: time.Now(), Deadline: time.Now().Add(time.Minute)})
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrBusy) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("%d jobs acquired", success)
	}
	jobs, err := s.Jobs(ctx, model.Filter{Page: 1, Limit: 10})
	if err != nil || len(jobs) != 1 {
		t.Fatal(jobs, err)
	}
	// Simulate an owner that stopped beyond its hard deadline.
	_, err = s.jobs.UpdateOne(ctx, bson.M{"_id": jobs[0].ID}, bson.M{"$set": bson.M{"deadline": time.Now().Add(-time.Minute)}})
	if err != nil {
		t.Fatal(err)
	}
	next := model.Job{ID: bson.NewObjectID(), SourceID: id, Status: "running", StartedAt: time.Now(), Deadline: time.Now().Add(time.Minute)}
	if err = s.Start(ctx, next); err != nil {
		t.Fatal(err)
	}
	old, err := s.Job(ctx, jobs[0].ID)
	if err != nil || old.Status != "failed" {
		t.Fatal(old, err)
	}
	if err = s.Finish(ctx, jobs[0]); err == nil {
		t.Fatal("expired owner finalized")
	}
	now := time.Now()
	next.FinishedAt = &now
	next.Status = "succeeded"
	if err = s.Finish(ctx, next); err != nil {
		t.Fatal(err)
	}
	src, err := s.Source(ctx, id)
	if err != nil || src.LastCrawledAt == nil {
		t.Fatal(src, err)
	}
}
func TestMongoConnectionFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if s, err := Open(ctx, "mongodb://127.0.0.1:1", "unused"); err == nil {
		_ = s.Client.Disconnect(context.Background())
		t.Fatal("unavailable database accepted")
	}
}

func TestMongoConfiguredSourcesAndLegacyData(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	legacyID := bson.NewObjectID()
	now := time.Now().UTC().Truncate(time.Millisecond)
	if _, err := s.sources.InsertOne(ctx, bson.M{"_id": legacyID, "name": "example", "enabled": true, "base_url": "https://legacy.invalid", "schedule": "* * * * *", "created_at": now, "last_crawled_at": now}); err != nil {
		t.Fatal(err)
	}
	legacy, err := s.Source(ctx, legacyID)
	if err != nil || !legacy.Automatic() || legacy.CrawlerKey() != "example" {
		t.Fatal(legacy, err)
	}
	no := false
	id := bson.NewObjectID()
	sources := []model.Source{{ID: id, Name: "first", CrawlerType: "example", BaseURL: "https://first.invalid", ListURL: "/posts", Enabled: true, AutoEnabled: &no, Schedule: "* * * * *", MaxPages: 1, MaxContents: 3, MaxRequests: 10, RequestIntervalMS: 1000, MaxConcurrency: 1}, {Name: "second", CrawlerType: "example", BaseURL: "https://second.invalid", Enabled: true, AutoEnabled: &no, Schedule: "* * * * *"}}
	if err := s.ApplySources(ctx, sources); err != nil {
		t.Fatal(err)
	}
	all, err := s.Sources(ctx)
	if err != nil || len(all) != 3 {
		t.Fatal(all, err)
	}
	first, err := s.Source(ctx, id)
	if err != nil || first.AutoEnabled == nil || first.Automatic() {
		t.Fatal(first, err)
	}
	sources[0].MaxRequests = 20
	if err := s.ApplySources(ctx, sources); err != nil {
		t.Fatal(err)
	}
	all, err = s.Sources(ctx)
	if err != nil || len(all) != 3 {
		t.Fatal(all, err)
	}
	again, err := s.Source(ctx, id)
	if err != nil || !again.CreatedAt.Equal(first.CreatedAt) || again.MaxRequests != 20 {
		t.Fatal(again, err)
	}
	after, err := s.Source(ctx, legacyID)
	if err != nil || after.LastCrawledAt == nil || !after.LastCrawledAt.Equal(now) {
		t.Fatal(after, err)
	}
	conflicting := sources[0]
	conflicting.ID = bson.NewObjectID()
	if err := s.ApplySources(ctx, []model.Source{conflicting}); err == nil {
		t.Fatal("conflicting ID accepted")
	}
	if err := s.Seed(ctx, "https://legacy.invalid/new", "* * * * *"); err != nil {
		t.Fatal(err)
	}
	after, err = s.Source(ctx, legacyID)
	if err != nil || after.ID != legacyID || !after.Automatic() {
		t.Fatal("legacy schedule behavior changed", after, err)
	}
}
