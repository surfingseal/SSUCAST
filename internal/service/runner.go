package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
)

var ErrCapacity = errors.New("worker capacity reached")
var ErrDisabled = errors.New("source disabled or unsupported")

type JobStore interface {
	Source(context.Context, bson.ObjectID) (model.Source, error)
	Start(context.Context, model.Job) error
	Finish(context.Context, model.Job) error
	Save(context.Context, []model.Content) error
}
type Runner struct {
	store    JobStore
	registry *crawler.Registry
	ctx      context.Context
	cancel   context.CancelFunc
	slots    chan struct{}
	timeout  time.Duration
	mu       sync.Mutex
	stopped  bool
	wg       sync.WaitGroup
}

// New retains the original constructor for existing embedders and tests.
func New(store JobStore, crawlers map[string]crawler.Crawler, workers int, timeout time.Duration) *Runner {
	registry := crawler.NewRegistry()
	for name, instance := range crawlers {
		c := instance
		if err := registry.Register(name, func(model.Source) (crawler.Crawler, error) { return c, nil }); err != nil {
			panic(err)
		}
	}
	return NewWithRegistry(store, registry, workers, timeout)
}
func NewWithRegistry(store JobStore, registry *crawler.Registry, workers int, timeout time.Duration) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	return &Runner{store: store, registry: registry, ctx: ctx, cancel: cancel, slots: make(chan struct{}, workers), timeout: timeout}
}
func (r *Runner) Start(ctx context.Context, id bson.ObjectID) (model.Job, error) {
	return r.start(ctx, id, false)
}
func (r *Runner) StartScheduled(ctx context.Context, id bson.ObjectID) (model.Job, error) {
	return r.start(ctx, id, true)
}
func (r *Runner) start(ctx context.Context, id bson.ObjectID, automatic bool) (model.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return model.Job{}, ErrCapacity
	}
	select {
	case r.slots <- struct{}{}:
	default:
		return model.Job{}, ErrCapacity
	}
	release := true
	defer func() {
		if release {
			<-r.slots
		}
	}()
	source, err := r.store.Source(ctx, id)
	if err != nil {
		return model.Job{}, err
	}
	if !source.Enabled || (automatic && !source.Automatic()) {
		return model.Job{}, ErrDisabled
	}
	c, err := r.registry.Resolve(source)
	if err != nil {
		return model.Job{}, fmt.Errorf("%w: %v", ErrDisabled, err)
	}
	started := time.Now().UTC()
	job := model.Job{Deadline: started.Add(r.timeout + 30*time.Second), ID: bson.NewObjectID(), SourceID: id, Status: "running", StartedAt: started}
	if err = r.store.Start(ctx, job); err != nil {
		return model.Job{}, err
	}
	r.wg.Add(1)
	release = false
	go r.run(c, source, job)
	return job, nil
}
func (r *Runner) run(c crawler.Crawler, s model.Source, j model.Job) {
	defer r.wg.Done()
	defer func() { <-r.slots }()
	ctx, cancel := context.WithDeadline(r.ctx, j.StartedAt.Add(r.timeout))
	defer cancel()
	var items []model.Content
	var err error
	stage := "crawl"
	func() {
		defer func() {
			if recover() != nil {
				err = fmt.Errorf("crawler panic")
			}
		}()
		items, err = c.Crawl(ctx, s)
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			stage = "save"
			eligible := items[:0]
			for _, item := range items {
				if !model.IsExcludedNotice(item) {
					eligible = append(eligible, item)
				}
			}
			items = eligible
			for i := range items {
				if err = model.PrepareClassification(s, &items[i]); err != nil {
					return
				}
			}
			err = r.store.Save(ctx, items)
		}
	}()
	j.Status = "succeeded"
	j.CollectedCount = len(items)
	if err != nil {
		j.Status = "failed"
		j.ErrorCount = 1
		j.ErrorMessage = "crawl or persistence failed; inspect server logs"
		j.ErrorStage = stage
		j.ErrorCode = "crawl"
		if stage == "save" {
			j.ErrorCode = "storage"
		}
		var fetchErr *crawler.FetchError
		if errors.As(err, &fetchErr) {
			j.ErrorCode = fetchErr.Code
			j.ErrorStage = fetchErr.Stage
			j.ErrorMessage = fetchErr.Error()
		}
		j.CollectedCount = 0
		if errors.Is(err, context.Canceled) {
			j.Status = "cancelled"
		}
		slog.Error("crawl failed", "job_id", j.ID.Hex(), "error", err)
	}
	now := time.Now().UTC()
	j.FinishedAt = &now
	// Persist terminal status independently of the cancelled job context.
	finalCtx, finalCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer finalCancel()
	if err := r.store.Finish(finalCtx, j); err != nil {
		slog.Error("job finalization failed; inspect state or retry after deadline", "job_id", j.ID.Hex(), "error", err)
	}
	slog.Info("job finished", "job_id", j.ID.Hex(), "status", j.Status, "collected_count", j.CollectedCount)
}
func (r *Runner) Stop() { r.mu.Lock(); r.stopped = true; r.cancel(); r.mu.Unlock(); r.wg.Wait() }
