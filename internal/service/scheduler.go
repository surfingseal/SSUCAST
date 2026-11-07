package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"example.com/crawlserver/internal/model"
)

// Schedules are loaded once at startup. Restart after changing a source schedule.
func Schedule(sources []model.Source, r *Runner) (*cron.Cron, error) {
	c := cron.New(cron.WithLocation(time.UTC), cron.WithChain(cron.Recover(cron.DefaultLogger)))
	for _, s := range sources {
		if !s.Automatic() {
			continue
		}
		id := s.ID
		if _, err := c.AddFunc(s.Schedule, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := r.StartScheduled(ctx, id); err != nil {
				slog.Warn("scheduled job skipped", "source_id", id.Hex(), "error", err)
			}
		}); err != nil {
			return nil, fmt.Errorf("source %s schedule: %w", s.Name, err)
		}
	}
	c.Start()
	return c, nil
}
