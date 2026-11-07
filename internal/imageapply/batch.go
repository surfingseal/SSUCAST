package imageapply

import (
	"bytes"
	"context"
	"example.com/crawlserver/internal/imageanalysis"
	"example.com/crawlserver/internal/mvp"
	"example.com/crawlserver/internal/repository"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"path/filepath"
	"time"
)

// Batch is manual-only. Backups are reparsed and restored in an owned loopback DB
// before any Atlas mutation. Each update still uses the exact snapshot CAS.
func Batch(ctx context.Context, store *repository.Store, items []imageanalysis.Prepared, dir string, apply bool) error {
	if len(items) == 0 || len(items) > 32 {
		return fmt.Errorf("expected 1..32 prepared notices")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	db := store.Client.Database("ssu_crawler")
	plans := []Plan{}
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, p := range items {
		id := p.Original.ID.Hex()
		if seen[id] {
			return fmt.Errorf("duplicate input ID")
		}
		seen[id] = true
		plan, e := Preview(ctx, db, p)
		if e != nil {
			return e
		}
		plans = append(plans, plan)
		counts[plan.Outcome]++
		// Verify the prior history belongs to this input.
		n, e := db.Collection("schedule_analyses").CountDocuments(ctx, bson.M{"_id": p.PreviousAnalysisID, "content_id": p.Original.ID, "content_hash": p.Analysis.ContentHash, "input_hash": p.Analysis.InputHash})
		if e != nil || n != 1 {
			return fmt.Errorf("prior analysis linkage missing: %s", id)
		}
	}
	if e := mvp.Write(dir, "preview.json", map[string]any{"outcomes": counts, "plans": plans, "apply_requested": apply}); e != nil {
		return e
	}
	if !apply {
		return nil
	}
	if counts["eligible"]+counts["duplicate"] != len(items) {
		return fmt.Errorf("conflicting batch; no writes")
	}
	if counts["eligible"] == 0 {
		return mvp.Write(dir, "apply-report.json", map[string]any{"updated": 0, "duplicate": len(items), "writes": 0})
	}
	n, e := db.Collection("crawled_contents").CountDocuments(ctx, bson.M{})
	if e != nil || n != 226 {
		return fmt.Errorf("expected 226 preserved notices")
	}
	if e = mvp.Snapshot(ctx, store, filepath.Join(dir, "backup"), true); e != nil {
		return fmt.Errorf("backup/restore verification failed: %w", e)
	}
	original, _, e := mvp.Collection(ctx, db, "crawled_contents")
	if e != nil {
		return e
	}
	histories, _, e := mvp.Collection(ctx, db, "schedule_analyses")
	if e != nil {
		return e
	}
	_, classHash, e := mvp.Collection(ctx, db, "content_classifications")
	if e != nil {
		return e
	}
	outcomes := []map[string]string{}
	updated, duplicates := 0, 0
	for i, p := range items {
		out, e := Apply(ctx, db, p, plans[i])
		if e != nil {
			out = "error"
		}
		outcomes = append(outcomes, map[string]string{"id": p.Original.ID.Hex(), "outcome": out})
		if out == "updated" {
			updated++
		}
		if out == "duplicate" {
			duplicates++
		}
		if w := mvp.Write(dir, "apply-report.json", map[string]any{"updated": updated, "duplicate": duplicates, "outcomes": outcomes, "at": time.Now().UTC()}); w != nil {
			return w
		}
		if e != nil || out != "updated" && out != "duplicate" {
			return fmt.Errorf("apply stopped on %s; inspect durable report; no rollback", out)
		}
	}
	// Compare every pre-existing field except the explicit analysis projection.
	allowed := map[string]bool{"notice_policy": true, "image_analysis_used": true, "image_analysis_input_hash": true, "image_analysis_content_hash": true, "image_schedule_count": true, "image_evidence_count": true}
	for _, p := range items {
		raw, _ := bson.Marshal(p.Analysis.ValidatedOutput)
		els, _ := bson.Raw(raw).Elements()
		for _, el := range els {
			allowed[el.Key()] = true
		}
	}
	for _, before := range original {
		var after bson.Raw
		if e = db.Collection("crawled_contents").FindOne(ctx, bson.M{"_id": before.Lookup("_id").ObjectID()}).Decode(&after); e != nil {
			return fmt.Errorf("notice missing after apply")
		}
		if !seen[before.Lookup("_id").ObjectID().Hex()] {
			if !bytes.Equal(before, after) {
				return fmt.Errorf("unselected notice changed")
			}
			continue
		}
		els, _ := before.Elements()
		for _, el := range els {
			if allowed[el.Key()] {
				continue
			}
			v := after.Lookup(el.Key())
			old := el.Value()
			if v.Type != old.Type || !bytes.Equal(v.Value, old.Value) {
				return fmt.Errorf("protected field changed: %s", el.Key())
			}
		}
	}
	for _, before := range histories {
		var after bson.Raw
		if e = db.Collection("schedule_analyses").FindOne(ctx, bson.M{"_id": before.Lookup("_id").StringValue()}).Decode(&after); e != nil || !bytes.Equal(before, after) {
			return fmt.Errorf("existing analysis history changed")
		}
	}
	_, afterHash, e := mvp.Collection(ctx, db, "content_classifications")
	if e != nil || afterHash != classHash {
		return fmt.Errorf("classification history changed")
	}
	count, e := db.Collection("crawled_contents").CountDocuments(ctx, bson.M{})
	if e != nil || count != 226 {
		return fmt.Errorf("notice count changed")
	}
	final := map[string]int{}
	images, total := 0, 0
	for _, p := range items {
		plan, e := Preview(ctx, db, p)
		if e != nil || plan.Outcome != "duplicate" {
			return fmt.Errorf("readback/duplicate failed")
		}
		final[plan.Outcome]++
		images += lenImage(p)
		total += len(p.Merge.Schedules)
	}
	return mvp.Write(dir, "verification.json", map[string]any{"contents": count, "updated": updated, "duplicate": duplicates, "readback": final, "image_schedules": images, "merged_schedules": total, "old_histories_preserved": len(histories), "protected_fields_preserved": true, "classification_unchanged": true, "backup_restore_verified": true})
}
func lenImage(p imageanalysis.Prepared) int {
	n := 0
	for _, s := range p.Merge.Schedules {
		if len(s.ImageEvidence) > 0 {
			n++
		}
	}
	return n
}
