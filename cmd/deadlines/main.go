// Defaults to read-only local Preview, independently of the Atlas .env.
package main

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/lifecycle"
	"flag"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"os"
	"time"
)

func save(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	if path != "" {
		return os.WriteFile(path, append(b, '\n'), 0600)
	}
	fmt.Println(string(b))
	return nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	uri := flag.String("uri", "mongodb://127.0.0.1:27030", "explicit local DB; .env/Atlas is not loaded")
	dbName := flag.String("database", "ssu_recent_20261106", "dataset")
	output := flag.String("output", "", "JSON report path")
	at := flag.String("at", "", "optional fixed RFC3339 evaluation time")
	apply := flag.Bool("apply", false, "explicitly write only deadline/lifecycle fields using snapshot CAS")
	cleanup := flag.Bool("cleanup", false, "preview confirmed expired stored records; dry-run by default")
	deleteFlag := flag.Bool("delete", false, "with cleanup, explicitly delete after verified backup; requires transaction support")
	backup := flag.String("backup-dir", "", "new backup directory for explicit deletion")
	flag.Parse()
	if *apply && *cleanup || *deleteFlag && !*cleanup {
		return fmt.Errorf("choose preview/apply or cleanup; delete requires cleanup")
	}
	if *deleteFlag && *backup == "" {
		return fmt.Errorf("verified backup directory required")
	}
	now := time.Now()
	if *at != "" {
		var e error
		now, e = time.Parse(time.RFC3339, *at)
		if e != nil {
			return e
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, e := mongo.Connect(options.Client().ApplyURI(*uri).SetServerSelectionTimeout(5 * time.Second))
	if e != nil {
		return fmt.Errorf("MongoDB configuration error")
	}
	defer client.Disconnect(context.Background())
	db := client.Database(*dbName)
	if *cleanup {
		plan, e := lifecycle.PlanCleanup(ctx, db, now)
		if e != nil {
			return e
		}
		if !*deleteFlag {
			return save(*output, map[string]any{"mode": "cleanup_dry_run", "database": *dbName, "candidate_count": len(plan.Candidates), "excluded_count": len(plan.Excluded), "deleted": 0, "plan": plan})
		}
		// Persist the complete target preview before any backup or deletion.
		if *output == "" {
			return fmt.Errorf("delete requires an output audit file")
		}
		if e = save(*output, plan); e != nil {
			return e
		}
		audit, e := lifecycle.Delete(ctx, db, plan, *backup)
		_ = save(*output, audit)
		return e
	}
	snapshots, before, e := lifecycle.Read(ctx, db)
	if e != nil {
		return e
	}
	report := lifecycle.Preview(snapshots, now)
	report.SourceSnapshotSHA256 = before
	if *apply {
		r, e := lifecycle.Apply(ctx, db, snapshots, report)
		if e != nil {
			return e
		}
		report.DBChanges = r.Applied
	}
	_, after, e := lifecycle.Read(ctx, db)
	if e != nil {
		return e
	}
	report.ReadAfterSHA256 = after
	report.Unchanged = before == after
	if !*apply && !report.Unchanged {
		return fmt.Errorf("dataset changed during read-only preview; retry")
	}
	if e = save(*output, report); e != nil {
		return e
	}
	fmt.Printf("total=%d confirmed=%d expired=%d active=%d needs_review=%d no_deadline=%d DB_changes=%d\n", report.Total, report.Confirmed, report.Expired, report.Active, report.NeedsReview, report.NoDeadline, report.DBChanges)
	return nil
}
