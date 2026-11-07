package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"os"
	"path/filepath"
	"time"
)

type CleanupItem struct {
	ID                       bson.ObjectID `json:"id"`
	URL                      string        `json:"url"`
	Reason                   string        `json:"reason"`
	ClassificationReferences int64         `json:"classification_references"`
	SourceExists             bool          `json:"source_exists"`
}
type CleanupPlan struct {
	CheckedAt  time.Time     `json:"checked_at"`
	Candidates []CleanupItem `json:"candidates"`
	Excluded   []CleanupItem `json:"excluded"`
	Snapshots  []Snapshot    `json:"-"`
	Database   string        `json:"database"`
	Sources    []bson.Raw    `json:"-"`
	Histories  []bson.Raw    `json:"-"`
}

func PlanCleanup(ctx context.Context, db *mongo.Database, now time.Time) (CleanupPlan, error) {
	plan := CleanupPlan{Database: db.Name(), CheckedAt: now.UTC(), Candidates: []CleanupItem{}, Excluded: []CleanupItem{}, Sources: []bson.Raw{}, Histories: []bson.Raw{}}
	snapshots, _, e := Read(ctx, db)
	if e != nil {
		return plan, e
	}
	for _, s := range snapshots {
		v := s.Content
		entry := CleanupItem{ID: v.ID, URL: v.OriginalURL}
		if !ConfirmedExpired(v, now) {
			entry.Reason = "deadline_not_confirmed_expired"
			plan.Excluded = append(plan.Excluded, entry)
			continue
		}
		var source model.Source
		raw, e := db.Collection("crawl_sources").FindOne(ctx, bson.M{"_id": v.SourceID}).Raw()
		if e != nil && e != mongo.ErrNoDocuments {
			return plan, e
		}
		if e == mongo.ErrNoDocuments {
			entry.Reason = "source_reference_missing"
			plan.Excluded = append(plan.Excluded, entry)
			continue
		}
		entry.SourceExists = true
		plan.Sources = append(plan.Sources, raw)
		if e = bson.Unmarshal(raw, &source); e != nil {
			return plan, e
		}
		histories, e := db.Collection("content_classifications").Find(ctx, bson.M{"content_id": v.ID})
		if e != nil {
			return plan, e
		}
		for histories.Next(ctx) {
			plan.Histories = append(plan.Histories, append(bson.Raw{}, histories.Current...))
			entry.ClassificationReferences++
		}
		if e = histories.Err(); e != nil {
			histories.Close(ctx)
			return plan, e
		}
		histories.Close(ctx)
		activeJobs, e := db.Collection("crawl_jobs").CountDocuments(ctx, bson.M{"source_id": v.SourceID, "status": "running"})
		if e != nil {
			return plan, e
		}
		if entry.ClassificationReferences > 0 {
			entry.Reason = "classification_history_retained"
		} else if source.ClassificationMode() != "source" {
			entry.Reason = "ai_source_requires_manual_retention_review"
		} else if activeJobs > 0 {
			entry.Reason = "source_job_running"
		}
		if entry.Reason != "" {
			plan.Excluded = append(plan.Excluded, entry)
			continue
		}
		entry.Reason = "confirmed_expired_no_history"
		plan.Candidates = append(plan.Candidates, entry)
		plan.Snapshots = append(plan.Snapshots, s)
	}
	return plan, nil
}

type BackupManifest struct {
	Database     string            `json:"database"`
	CheckedAt    time.Time         `json:"checked_at"`
	CandidateIDs []bson.ObjectID   `json:"candidate_ids"`
	Files        map[string]string `json:"sha256"`
}

func Backup(plan CleanupPlan, dir string) (BackupManifest, error) {
	m := BackupManifest{Database: plan.Database, CheckedAt: plan.CheckedAt, CandidateIDs: []bson.ObjectID{}, Files: map[string]string{}}
	if e := os.Mkdir(dir, 0700); e != nil {
		return m, e
	}
	docs := []bson.Raw{}
	for _, s := range plan.Snapshots {
		docs = append(docs, s.Raw)
		m.CandidateIDs = append(m.CandidateIDs, s.Content.ID)
	}
	for name, values := range map[string][]bson.Raw{"contents.extjsonl": docs, "sources.extjsonl": plan.Sources, "classifications.extjsonl": plan.Histories} {
		f, e := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return m, e
		}
		hash := sha256.New()
		for _, raw := range values {
			b, e := bson.MarshalExtJSON(raw, true, false)
			if e != nil {
				f.Close()
				return m, e
			}
			b = append(b, '\n')
			if _, e = f.Write(b); e != nil {
				f.Close()
				return m, e
			}
			hash.Write(b)
		}
		if e = f.Sync(); e != nil {
			f.Close()
			return m, e
		}
		if e = f.Close(); e != nil {
			return m, e
		}
		m.Files[name] = fmt.Sprintf("%x", hash.Sum(nil))
	}
	raw, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return m, e
	}
	f, e := os.OpenFile(filepath.Join(dir, "manifest.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return m, e
	}
	if _, e = f.Write(append(raw, '\n')); e != nil {
		f.Close()
		return m, e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return m, e
	}
	if e = f.Close(); e != nil {
		return m, e
	}
	// Verify on-disk bytes before making them eligible for a delete operation.
	for name, expected := range m.Files {
		b, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil || fmt.Sprintf("%x", sha256.Sum256(b)) != expected {
			return m, fmt.Errorf("backup verification failed")
		}
	}
	return m, nil
}

type CleanupAudit struct {
	Database       string          `json:"database"`
	DryRun         bool            `json:"dry_run"`
	Candidates     int             `json:"candidates"`
	Deleted        int             `json:"deleted"`
	Excluded       int             `json:"excluded"`
	ChangedSkipped int             `json:"changed_skipped"`
	BackupDir      string          `json:"backup_directory"`
	IDs            []bson.ObjectID `json:"ids"`
	Error          string          `json:"error,omitempty"`
}

// Delete requires a new verified backup and transactions (replica set/Atlas).
// Standalone MongoDB fails closed; no TTL indexes or cascade history deletes.
func Delete(ctx context.Context, db *mongo.Database, plan CleanupPlan, backupDir string) (audit CleanupAudit, err error) {
	audit = CleanupAudit{Database: db.Name(), Candidates: len(plan.Candidates), Excluded: len(plan.Excluded), BackupDir: backupDir, IDs: []bson.ObjectID{}}
	backupVerified := false
	defer func() {
		if err != nil {
			audit.Error = err.Error()
		}
		if backupVerified {
			_ = writeAudit(filepath.Join(backupDir, "deletion-audit.json"), audit)
		}
	}()
	if db.Name() != plan.Database {
		return audit, fmt.Errorf("backup database mismatch")
	}
	if _, err = Backup(plan, backupDir); err != nil {
		return audit, err
	}
	backupVerified = true
	for _, snapshot := range plan.Snapshots {
		session, e := db.Client().StartSession()
		if e != nil {
			return audit, e
		}
		result, e := session.WithTransaction(ctx, func(tx context.Context) (any, error) {
			var current model.Content
			if e := db.Collection("crawled_contents").FindOne(tx, SnapshotFilter(snapshot)).Decode(&current); e == mongo.ErrNoDocuments {
				return false, nil
			} else if e != nil {
				return nil, e
			}
			if !ConfirmedExpired(current, time.Now()) {
				return false, nil
			}
			var source model.Source
			if e := db.Collection("crawl_sources").FindOne(tx, bson.M{"_id": current.SourceID}).Decode(&source); e != nil {
				return false, e
			}
			if source.ClassificationMode() != "source" {
				return false, nil
			}
			n, e := db.Collection("content_classifications").CountDocuments(tx, bson.M{"content_id": current.ID})
			if e != nil || n > 0 {
				return false, e
			}
			n, e = db.Collection("crawl_jobs").CountDocuments(tx, bson.M{"source_id": current.SourceID, "status": "running"})
			if e != nil || n > 0 {
				return false, e
			}
			r, e := db.Collection("crawled_contents").DeleteOne(tx, SnapshotFilter(snapshot))
			if e != nil {
				return nil, e
			}
			return r.DeletedCount == 1, nil
		})
		session.EndSession(ctx)
		if e != nil {
			return audit, e
		}
		deleted, _ := result.(bool)
		if deleted {
			audit.Deleted++
			audit.IDs = append(audit.IDs, snapshot.Content.ID)
		} else {
			audit.ChangedSkipped++
			audit.Excluded++
		}
		if e = writeAudit(filepath.Join(backupDir, "deletion-audit.json"), audit); e != nil {
			return audit, e
		}
	}
	return audit, nil
}
func writeAudit(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp := path + ".tmp"
	f, e := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(append(b, '\n')); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(tmp, path)
}
