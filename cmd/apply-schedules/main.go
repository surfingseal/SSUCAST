// Default: read-only preview. Database writes exist only behind explicit --apply.
package main

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/lifecycle"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/scheduleapply"
	"flag"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"time"
)

type report struct {
	ApplyConflicts        int                              `json:"apply_conflicts"`
	ApplyBlocked          bool                             `json:"apply_blocked"`
	ApplyErrors           int                              `json:"apply_errors"`
	Mode                  string                           `json:"mode"`
	Database              string                           `json:"database"`
	Targets               int                              `json:"targets"`
	Counts                map[string]int                   `json:"outcomes"`
	ReviewNotices         int                              `json:"review_notices"`
	Schedules             int                              `json:"planned_schedules"`
	OriginalSchedules     int                              `json:"original_schedules"`
	RuleSessions          int                              `json:"rule_sessions"`
	BeforeCount           int                              `json:"before_count"`
	AfterCount            int                              `json:"after_count"`
	BeforeHash            string                           `json:"before_sha256"`
	AfterHash             string                           `json:"after_sha256"`
	Unchanged             bool                             `json:"database_unchanged"`
	HistoryBefore         int64                            `json:"history_before"`
	HistoryAfter          int64                            `json:"history_after"`
	ClassificationsBefore int64                            `json:"classifications_before"`
	ClassificationsAfter  int64                            `json:"classifications_after"`
	Writes                int64                            `json:"content_writes"`
	HistoryWrites         int                              `json:"history_inserts"`
	Plans                 []repository.SchedulePlan        `json:"items"`
	Applied               []repository.ScheduleApplyResult `json:"apply_results,omitempty"`
	GeminiCalls           int                              `json:"gemini_calls"`
	SchoolRequests        int                              `json:"school_requests"`
}

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "schedule command failed:", e)
		os.Exit(1)
	}
}
func run() error {
	input := flag.String("input", "", "validated frozen apply-input.json")
	out := flag.String("output", "", "safe preview report JSON")
	uri := flag.String("uri", "", "explicit Mongo URI; never printed")
	database := flag.String("database", "", "database override")
	apply := flag.Bool("apply", false, "explicitly write eligible analysis projections; default false")
	flag.Parse()
	raw, e := os.ReadFile(*input)
	if e != nil {
		return fmt.Errorf("apply input unavailable")
	}
	var bundle scheduleapply.Bundle
	if json.Unmarshal(raw, &bundle) != nil {
		return fmt.Errorf("invalid apply input JSON")
	}
	if e = scheduleapply.ValidateBundle(bundle); e != nil {
		return e
	}
	if *apply && bundle.Version == scheduleapply.BatchBundleVersion {
		return fmt.Errorf("batch apply disabled: this experiment supports read-only preview only")
	}
	cfg, e := config.LoadMongoConnection(*uri, *database)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// No Open(): no index or collection creation in preview.
	store, e := repository.OpenReadOnly(ctx, cfg.URI, cfg.Database)
	if e != nil {
		return fmt.Errorf("MongoDB connection failed (credentials omitted)")
	}
	defer store.Client.Disconnect(context.Background())
	db := store.Client.Database(cfg.Database)
	repo := repository.NewScheduleAnalyses(db)
	before, hash, e := lifecycle.Read(ctx, db)
	if e != nil {
		return fmt.Errorf("snapshot read failed")
	}
	count := func(name string) (int64, error) { return db.Collection(name).CountDocuments(ctx, bson.M{}) }
	history, e := count("schedule_analyses")
	if e != nil {
		return fmt.Errorf("history count read failed")
	}
	classes, e := count("content_classifications")
	if e != nil {
		return fmt.Errorf("classification count read failed")
	}
	mode := "preview"
	if *apply {
		mode = "apply"
	}
	r := report{Mode: mode, Database: cfg.Database, Targets: len(bundle.Items), Counts: map[string]int{}, BeforeCount: len(before), BeforeHash: hash, HistoryBefore: history, ClassificationsBefore: classes, Plans: []repository.SchedulePlan{}}
	for _, v := range bundle.Items {
		p, e := repo.Preview(ctx, v)
		if e != nil {
			return e
		}
		r.Plans = append(r.Plans, p)
		r.Counts[p.Outcome]++
		r.Schedules += p.Schedules
		r.OriginalSchedules += p.OriginalSchedules
		r.RuleSessions += p.RuleSessions
		if p.ReviewRequired {
			r.ReviewNotices++
		}
	}
	// Complete preview is produced before any --apply mutation. On any conflict
	// abort the whole batch without inserting histories or changing contents.
	if *apply {
		for _, p := range r.Plans {
			if p.Outcome != "eligible" && p.Outcome != "duplicate" {
				r.ApplyBlocked = true
			}
		}
		if !r.ApplyBlocked {
			for i, item := range bundle.Items {
				a, e := repo.Apply(ctx, item, r.Plans[i])
				if e != nil {
					r.ApplyErrors++
					a.Outcome = "write_error"
					r.Applied = append(r.Applied, a)
					if a.HistoryInserted {
						r.HistoryWrites++
					}
					break
				}
				r.Applied = append(r.Applied, a)
				r.Writes += a.Modified
				if a.HistoryInserted {
					r.HistoryWrites++
				}
				if a.Outcome != "updated" && a.Outcome != "duplicate" {
					r.ApplyConflicts++
					break
				}
			}
		}
	}
	after, afterHash, e := lifecycle.Read(ctx, db)
	if e != nil {
		return fmt.Errorf("post-run snapshot failed")
	}
	r.AfterCount = len(after)
	r.AfterHash = afterHash
	r.Unchanged = hash == afterHash
	r.HistoryAfter, e = count("schedule_analyses")
	if e != nil {
		return fmt.Errorf("post-run history count failed")
	}
	r.ClassificationsAfter, e = count("content_classifications")
	if e != nil {
		return fmt.Errorf("post-run classification count failed")
	}
	if !*apply && (!r.Unchanged || r.HistoryBefore != r.HistoryAfter || r.ClassificationsBefore != r.ClassificationsAfter) {
		return fmt.Errorf("database changed during read-only preview")
	}
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	if *out != "" {
		if e = os.WriteFile(*out, append(b, '\n'), 0600); e != nil {
			return e
		}
	}
	fmt.Printf("mode=%s targets=%d eligible=%d duplicates=%d content_hash_conflicts=%d input_conflicts=%d version_conflicts=%d state_conflicts=%d review=%d schedules=%d (%d+%d) documents=%d→%d content_writes=%d history_inserts=%d unchanged=%t\n", r.Mode, r.Targets, r.Counts["eligible"], r.Counts["duplicate"], r.Counts["content_hash_conflict"], r.Counts["input_hash_conflict"], r.Counts["analysis_version_conflict"], r.Counts["analysis_state_conflict"], r.ReviewNotices, r.Schedules, r.OriginalSchedules, r.RuleSessions, r.BeforeCount, r.AfterCount, r.Writes, r.HistoryWrites, r.Unchanged)
	if r.ApplyBlocked {
		return fmt.Errorf("apply not executed: preview conflicts; report preserved")
	}
	if r.ApplyConflicts > 0 {
		return fmt.Errorf("apply stopped by CAS conflict; inspect partial report")
	}
	if r.ApplyErrors > 0 {
		return fmt.Errorf("apply stopped after write error; inspect partial report")
	}
	return nil
}
