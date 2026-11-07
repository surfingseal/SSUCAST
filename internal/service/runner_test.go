package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
)

type fakeStore struct {
	mu       sync.Mutex
	running  bool
	finished chan model.Job
	saveErr  error
}

func (f *fakeStore) Source(_ context.Context, id bson.ObjectID) (model.Source, error) {
	return model.Source{ID: id, Name: "example", Enabled: true}, nil
}
func (f *fakeStore) Start(_ context.Context, j model.Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.running {
		return repository.ErrBusy
	}
	f.running = true
	return nil
}
func (f *fakeStore) Finish(_ context.Context, j model.Job) error {
	f.mu.Lock()
	f.running = false
	f.mu.Unlock()
	f.finished <- j
	return nil
}
func (f *fakeStore) Save(context.Context, []model.Content) error { return f.saveErr }

type blockCrawler struct{}

func (blockCrawler) Crawl(ctx context.Context, s model.Source) ([]model.Content, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestRunnerDuplicateAndShutdown(t *testing.T) {
	f := &fakeStore{finished: make(chan model.Job, 1)}
	r := New(f, map[string]crawler.Crawler{"example": blockCrawler{}}, 2, time.Minute)
	id := bson.NewObjectID()
	if _, err := r.Start(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Start(context.Background(), id); !errors.Is(err, repository.ErrBusy) {
		t.Fatal(err)
	}
	r.Stop()
	j := <-f.finished
	if j.Status != "cancelled" || j.FinishedAt == nil {
		t.Fatalf("%+v", j)
	}
	if _, err := r.Start(context.Background(), id); !errors.Is(err, ErrCapacity) {
		t.Fatal(err)
	}
}
func TestRunnerCapacityAndDeadline(t *testing.T) {
	f := &fakeStore{finished: make(chan model.Job, 1)}
	r := New(f, map[string]crawler.Crawler{"example": blockCrawler{}}, 1, 30*time.Millisecond)
	defer r.Stop()
	if _, err := r.Start(context.Background(), bson.NewObjectID()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Start(context.Background(), bson.NewObjectID()); !errors.Is(err, ErrCapacity) {
		t.Fatal(err)
	}
	select {
	case j := <-f.finished:
		if j.Status != "failed" {
			t.Fatal(j.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("deadline not enforced")
	}
}

type instantCrawler struct{}

func (instantCrawler) Crawl(context.Context, model.Source) ([]model.Content, error) {
	return []model.Content{}, nil
}
func TestSchedulerRegistrationAndTrigger(t *testing.T) {
	f := &fakeStore{finished: make(chan model.Job, 1)}
	r := New(f, map[string]crawler.Crawler{"example": instantCrawler{}}, 1, time.Second)
	defer r.Stop()
	source := model.Source{ID: bson.NewObjectID(), Name: "example", Enabled: true, Schedule: "0 0 1 1 *"}
	c, err := Schedule([]model.Source{source}, r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { <-c.Stop().Done() }()
	entries := c.Entries()
	if len(entries) != 1 {
		t.Fatal(entries)
	}
	// Invoke the registered cron callback without waiting for wall-clock minutes.
	entries[0].Job.Run()
	select {
	case j := <-f.finished:
		if j.Status != "succeeded" || j.SourceID != source.ID {
			t.Fatal(j)
		}
	case <-time.After(time.Second):
		t.Fatal("registered job did not run")
	}
	source.Schedule = "not cron"
	if bad, err := Schedule([]model.Source{source}, r); err == nil {
		bad.Stop()
		t.Fatal("invalid cron accepted")
	}
}
func TestPersistenceFailureIsRecorded(t *testing.T) {
	f := &fakeStore{finished: make(chan model.Job, 1), saveErr: errors.New("database unavailable")}
	r := New(f, map[string]crawler.Crawler{"example": instantCrawler{}}, 1, time.Second)
	defer r.Stop()
	if _, err := r.Start(context.Background(), bson.NewObjectID()); err != nil {
		t.Fatal(err)
	}
	select {
	case j := <-f.finished:
		if j.Status != "failed" || j.ErrorCount != 1 || j.ErrorCode != "storage" || j.ErrorStage != "save" {
			t.Fatal(j)
		}
	case <-time.After(time.Second):
		t.Fatal("failure not finalized")
	}
}

type configuredStore struct {
	*fakeStore
	source model.Source
}

func (s *configuredStore) Source(context.Context, bson.ObjectID) (model.Source, error) {
	return s.source, nil
}
func TestManualAndAutomaticExecutionAreIndependent(t *testing.T) {
	no := false
	f := &configuredStore{fakeStore: &fakeStore{finished: make(chan model.Job, 1)}, source: model.Source{ID: bson.NewObjectID(), Name: "real-site", CrawlerType: "example", Enabled: true, AutoEnabled: &no, Schedule: "* * * * *"}}
	registry := crawler.NewRegistry()
	if err := registry.Register("example", func(model.Source) (crawler.Crawler, error) { return instantCrawler{}, nil }); err != nil {
		t.Fatal(err)
	}
	r := NewWithRegistry(f, registry, 1, time.Second)
	defer r.Stop()
	c, err := Schedule([]model.Source{f.source}, r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { <-c.Stop().Done() }()
	if len(c.Entries()) != 0 {
		t.Fatal("manual-only source scheduled")
	}
	if _, err := r.StartScheduled(context.Background(), f.source.ID); !errors.Is(err, ErrDisabled) {
		t.Fatal("automatic execution allowed", err)
	}
	if _, err := r.Start(context.Background(), f.source.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case j := <-f.finished:
		if j.Status != "succeeded" {
			t.Fatal(j)
		}
	case <-time.After(time.Second):
		t.Fatal("manual job blocked")
	}
}
func TestUnavailableSourceCannotRunManually(t *testing.T) {
	yes := true
	f := &configuredStore{fakeStore: &fakeStore{finished: make(chan model.Job, 1)}, source: model.Source{ID: bson.NewObjectID(), Name: "example", Enabled: false, AutoEnabled: &yes}}
	r := New(f, map[string]crawler.Crawler{"example": instantCrawler{}}, 1, time.Second)
	defer r.Stop()
	if _, err := r.Start(context.Background(), f.source.ID); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
}
