// Explicit manual, backed-up, full-BSON-CAS projection; no AI or school requests.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/mvp"
	"example.com/crawlserver/internal/noticepolicy"
	"example.com/crawlserver/internal/repository"
	"flag"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "notice policy stopped:", e)
		os.Exit(1)
	}
}
func same(a, b model.NoticePolicy) bool {
	a.AppliedAt = time.Time{}
	b.AppliedAt = time.Time{}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
func original(raw bson.Raw) ([]byte, error) {
	var d bson.D
	if e := bson.Unmarshal(raw, &d); e != nil {
		return nil, e
	}
	v := bson.D{}
	for _, x := range d {
		if x.Key != "notice_policy" {
			v = append(v, x)
		}
	}
	return bson.Marshal(v)
}
func run() error {
	out := flag.String("output", "", "private preview/backup directory")
	year := flag.Int("year", time.Now().In(noticepolicy.Seoul).Year(), "explicit assumed year")
	apply := flag.Bool("apply", false, "apply policy fields only after validated backup")
	flag.Parse()
	if *out == "" {
		return fmt.Errorf("output required")
	}
	if e := os.MkdirAll(*out, 0700); e != nil {
		return e
	}
	cfg, e := config.LoadMongoConnection("", "")
	if e != nil {
		return e
	}
	if cfg.Database != "ssu_crawler" {
		return fmt.Errorf("expected ssu_crawler")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	store, e := repository.OpenReadOnly(ctx, cfg.URI, cfg.Database)
	if e != nil {
		return fmt.Errorf("database connection failed")
	}
	defer store.Client.Disconnect(context.Background())
	db := store.Client.Database(cfg.Database)
	rows, hash, e := mvp.Collection(ctx, db, "crawled_contents")
	if e != nil {
		return e
	}
	now := time.Now().UTC()
	type target struct {
		Raw bson.Raw
		V   model.Content
		P   model.NoticePolicy
	}
	targets := []target{}
	plans := []map[string]any{}
	filled, hidden, duplicate, acceptedImages := 0, 0, 0, 0
	for _, raw := range rows {
		var v model.Content
		if bson.Unmarshal(raw, &v) != nil {
			return fmt.Errorf("invalid BSON")
		}
		if v.ScheduleAnalysisID == "" || v.ScheduleInputHash == "" {
			continue
		}
		p, e := noticepolicy.Project(v, *year, now)
		if e != nil {
			return e
		}
		outcome := "eligible"
		if v.NoticePolicy != nil && same(*v.NoticePolicy, p) {
			outcome = "duplicate"
			duplicate++
		}
		hide := p.ExpiresAt != nil && !now.Before(*p.ExpiresAt)
		if hide {
			hidden++
		}
		acceptedImages += p.ImageAcceptedSchedules
		filled += p.BoundariesFilled
		targets = append(targets, target{raw, v, p})
		plans = append(plans, map[string]any{"id": v.ID.Hex(), "title": v.Title, "source": v.SourceName, "accepted_image_schedules": p.ImageAcceptedSchedules, "filled_year_boundaries": p.BoundariesFilled, "hidden_expired": hide, "expires_at": p.ExpiresAt, "expiry_reason": p.ExpiryReason, "outcome": outcome})
	}
	report := map[string]any{"year": *year, "total": len(rows), "targets": len(targets), "image_source_policy": noticepolicy.ImageSourcePolicy, "accepted_image_schedules": acceptedImages, "year_boundaries_filled": filled, "expired_candidates": hidden, "duplicates": duplicate, "plans": plans, "gemini_requests": 0, "school_requests": 0, "writes": 0, "mode": "preview"}
	if e = mvp.Write(*out, "preview.json", report); e != nil {
		return e
	}
	if !*apply {
		fmt.Printf("Preview targets=%d years=%d expired=%d duplicates=%d\n", len(targets), filled, hidden, duplicate)
		return nil
	}
	if duplicate == len(targets) {
		report["mode"] = "apply"
		report["writes"] = 0
		if e = mvp.Write(*out, "duplicate-apply.json", report); e != nil {
			return e
		}
		fmt.Printf("No changes: %d duplicate policies\n", duplicate)
		return nil
	}
	if e = os.MkdirAll(*out, 0700); e != nil {
		return e
	}
	var buf bytes.Buffer
	for _, r := range rows {
		buf.Write(r)
	}
	backup := filepath.Join(*out, "before.bson")
	if old, e := os.ReadFile(backup); e == nil && !bytes.Equal(old, buf.Bytes()) {
		return fmt.Errorf("backup exists with different data")
	}
	if e = os.WriteFile(backup, buf.Bytes(), 0600); e != nil {
		return e
	}
	restored, e := mvp.ReadDump(backup)
	if e != nil || len(restored) != len(rows) {
		return fmt.Errorf("backup reparse failed")
	}
	if e = mvp.RestoreCheck(ctx, map[string][]bson.Raw{"crawled_contents": restored}); e != nil {
		return e
	}
	baseline := map[bson.ObjectID][]byte{}
	for _, raw := range rows {
		var v model.Content
		bson.Unmarshal(raw, &v)
		b, e := original(raw)
		if e != nil {
			return e
		}
		baseline[v.ID] = b
	}
	writes := int64(0)
	for _, t := range targets {
		if t.V.NoticePolicy != nil && same(*t.V.NoticePolicy, t.P) {
			continue
		}
		var observed bson.D
		if bson.Unmarshal(t.Raw, &observed) != nil {
			return fmt.Errorf("invalid CAS document")
		}
		f := bson.M{"_id": t.V.ID, "content_hash": t.V.ContentHash, "schedule_input_hash": t.V.ScheduleInputHash, "schedule_analysis_id": t.V.ScheduleAnalysisID, "$expr": bson.M{"$eq": bson.A{"$$ROOT", bson.M{"$literal": observed}}}}
		r, e := db.Collection("crawled_contents").UpdateOne(ctx, f, bson.M{"$set": bson.M{"notice_policy": t.P}})
		if e != nil {
			return fmt.Errorf("update failed after %d writes", writes)
		}
		if r.MatchedCount != 1 {
			return fmt.Errorf("CAS conflict after %d writes", writes)
		}
		writes += r.ModifiedCount
		report["writes"] = writes
		report["mode"] = "apply"
		if e = mvp.Write(*out, "apply.json", report); e != nil {
			return e
		}
	}
	after, _, e := mvp.Collection(ctx, db, "crawled_contents")
	if e != nil {
		return e
	}
	if len(after) != len(rows) {
		return fmt.Errorf("content count changed")
	}
	for _, raw := range after {
		var v model.Content
		bson.Unmarshal(raw, &v)
		b, _ := original(raw)
		if !bytes.Equal(b, baseline[v.ID]) {
			return fmt.Errorf("original/AI/classification fields changed")
		}
	}
	report["backup_hash"] = hash
	report["backup_restored_verified"] = true
	report["original_and_AI_fields_unchanged"] = true
	report["content_count_after"] = len(after)
	if e = mvp.Write(*out, "apply.json", report); e != nil {
		return e
	}
	fmt.Printf("Applied=%d targets=%d restoredYearBoundaries=%d expired=%d originals=%d\n", writes, len(targets), filled, hidden, len(after))
	return nil
}
