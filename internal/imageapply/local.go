package imageapply

import (
	"bytes"
	"context"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/imageanalysis"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/mvp"
	"example.com/crawlserver/internal/repository"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"os"
	"time"
)

func VerifyLocal(ctx context.Context, production *mongo.Database, items []imageanalysis.Prepared, dir string) error {
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		uri = "mongodb://127.0.0.1:27030"
	}
	name := "ssu_schedule_apply_test_" + bson.NewObjectID().Hex()
	if e := config.ValidateIsolatedTestMongo(uri, name); e != nil {
		return e
	}
	store, e := repository.OpenReadOnly(ctx, uri, name)
	if e != nil {
		return fmt.Errorf("isolated Mongo unavailable")
	}
	defer store.Client.Disconnect(context.Background())
	db := store.Client.Database(name)
	names, e := db.ListCollectionNames(ctx, bson.M{})
	if e != nil || len(names) != 0 {
		return fmt.Errorf("test DB not empty")
	}
	owner := bson.NewObjectID()
	db.Collection("test_control").InsertOne(ctx, bson.M{"_id": owner})
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		n, _ := db.Collection("test_control").CountDocuments(c, bson.M{"_id": owner})
		if n == 1 && config.ValidateIsolatedTestMongo(uri, name) == nil {
			db.Drop(c)
		}
	}()
	old := map[string]bson.Raw{}
	for _, p := range items {
		if _, e = db.Collection("crawled_contents").InsertOne(ctx, p.Original); e != nil {
			return e
		}
		var raw bson.Raw
		if e = production.Collection("schedule_analyses").FindOne(ctx, bson.M{"_id": p.PreviousAnalysisID}).Decode(&raw); e != nil {
			return fmt.Errorf("text history missing")
		}
		old[p.PreviousAnalysisID] = append(bson.Raw{}, raw...)
		if _, e = db.Collection("schedule_analyses").InsertOne(ctx, raw); e != nil {
			return e
		}
	}
	updated, duplicate, total, imageCount := 0, 0, 0, 0
	for _, p := range items {
		plan, e := Preview(ctx, db, p)
		if e != nil || plan.Outcome != "eligible" {
			return fmt.Errorf("local image preview failed: %v", e)
		}
		out, e := Apply(ctx, db, p, plan)
		if e != nil || out != "updated" {
			return fmt.Errorf("local image apply failed: %v", e)
		}
		updated++
		var got model.Content
		if db.Collection("crawled_contents").FindOne(ctx, bson.M{"_id": p.Original.ID}).Decode(&got) != nil {
			return fmt.Errorf("readback failed")
		}
		v := p.Original
		if got.Title != v.Title || got.Content != v.Content || got.OriginalURL != v.OriginalURL || got.SourceID != v.SourceID || got.ContentHash != v.ContentHash || got.Category != v.Category || got.Status != v.Status || got.LifecycleStatus != v.LifecycleStatus || !equal(got.Deadline, v.Deadline) {
			return fmt.Errorf("protected original changed")
		}
		if got.Summary != v.Summary || !equal(got.TargetAudience, v.TargetAudience) {
			return fmt.Errorf("text summary/audience overwritten")
		}
		if !equal(got.Schedules, p.Merge.Schedules) {
			return fmt.Errorf("calendar/precision/evidence BSON roundtrip lost")
		}
		total += len(got.Schedules)
		imageCount += got.ImageScheduleCount
		plan, e = Preview(ctx, db, p)
		if e != nil || plan.Outcome != "duplicate" {
			return fmt.Errorf("image duplicate preview failed")
		}
		out, e = Apply(ctx, db, p, plan)
		if e != nil || out != "duplicate" {
			return fmt.Errorf("image duplicate apply failed")
		}
		duplicate++
	}
	for id, raw := range old {
		var after bson.Raw
		if db.Collection("schedule_analyses").FindOne(ctx, bson.M{"_id": id}).Decode(&after) != nil || !bytes.Equal(raw, after) {
			return fmt.Errorf("text-only history changed")
		}
	}
	count, _ := db.Collection("crawled_contents").CountDocuments(ctx, bson.M{})
	history, _ := db.Collection("schedule_analyses").CountDocuments(ctx, bson.M{})
	return mvp.Write(dir, "local-image-integration.json", map[string]any{"isolated_loopback": true, "actual_saved_results": len(items), "updated": updated, "duplicate_noops": duplicate, "contents": count, "histories": history, "text_histories_preserved": len(old), "total_merged_schedules": total, "image_schedule_count": imageCount, "original_fields_preserved": true, "calendar_and_evidence_BSON_roundtrip": true, "temporary_db_cleaned": true, "atlas_writes": 0})
}
func equal(a, b any) bool {
	x, _ := bson.Marshal(bson.M{"value": a})
	y, _ := bson.Marshal(bson.M{"value": b})
	return bytes.Equal(x, y)
}
