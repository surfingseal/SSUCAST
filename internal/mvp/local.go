package mvp

import (
	"context"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"path/filepath"
	"time"
)

// LocalCheck writes only an owned, newly-created loopback test database.
func LocalCheck(ctx context.Context, baseline, input, media, out string) error {
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
	if _, e = db.Collection("test_control").InsertOne(ctx, bson.M{"_id": owner}); e != nil {
		return e
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		n, _ := db.Collection("test_control").CountDocuments(c, bson.M{"_id": owner})
		if n == 1 && config.ValidateIsolatedTestMongo(uri, name) == nil {
			db.Drop(c)
		}
	}()
	for _, collection := range []string{"crawled_contents", "schedule_analyses", "crawl_sources", "content_classifications"} {
		rows, e := ReadDump(filepath.Join(baseline, collection+".bson"))
		if e != nil {
			return e
		}
		for _, r := range rows {
			if _, e = db.Collection(collection).InsertOne(ctx, r); e != nil {
				return e
			}
		}
	}
	var p Prepared
	if Read(input, &p) != nil || len(p.Items) != 86 {
		return fmt.Errorf("prepared86 missing")
	}
	repo := repository.NewScheduleAnalyses(db)
	updated, duplicates, upgrades := 0, 0, 0
	for _, item := range p.Items {
		var before bson.Raw
		db.Collection("crawled_contents").FindOne(ctx, bson.M{"_id": item.Original.ID}).Decode(&before)
		var old model.Content
		bson.Unmarshal(before, &old)
		if old.ScheduleModelVersion != "" {
			upgrades++
		}
		plan, e := repo.Preview(ctx, item)
		if e != nil || plan.Outcome != "eligible" {
			return fmt.Errorf("local preview: %s %v", plan.Outcome, e)
		}
		r, e := repo.Apply(ctx, item, plan)
		if e != nil || r.Outcome != "updated" {
			return fmt.Errorf("local apply failed: %v", e)
		}
		updated++
		var after bson.Raw
		db.Collection("crawled_contents").FindOne(ctx, bson.M{"_id": item.Original.ID}).Decode(&after)
		h1, _ := OriginalHash(before)
		h2, _ := OriginalHash(after)
		if h1 != h2 {
			return fmt.Errorf("local original changed")
		}
		plan, e = repo.Preview(ctx, item)
		if e != nil || plan.Outcome != "duplicate" {
			return fmt.Errorf("duplicate failed")
		}
		r, e = repo.Apply(ctx, item, plan)
		if e != nil || r.Outcome != "duplicate" {
			return fmt.Errorf("duplicate apply failed")
		}
		duplicates++
		if e = repo.VerifyScheduleHistory(ctx, item); e != nil {
			return e
		}
	}
	// A post-preview content modification must be rejected by exact BSON CAS.
	item := p.Items[0]
	db.Collection("crawled_contents").DeleteOne(ctx, bson.M{"_id": item.Original.ID})
	db.Collection("crawled_contents").InsertOne(ctx, item.Original)
	plan, e := repo.Preview(ctx, item)
	if e != nil {
		return e
	}
	db.Collection("crawled_contents").UpdateOne(ctx, bson.M{"_id": item.Original.ID}, bson.M{"$set": bson.M{"content": "concurrent modification"}})
	r, e := repo.Apply(ctx, item, plan)
	if e != nil || r.Outcome != "cas_conflict" && r.Outcome != "input_hash_conflict" {
		return fmt.Errorf("CAS not blocked: %s %v", r.Outcome, e)
	}
	// A newer analysis time blocks an older result even with the same source.
	db.Collection("crawled_contents").UpdateOne(ctx, bson.M{"_id": item.Original.ID}, bson.M{"$set": bson.M{"content": item.Original.Content, "schedule_analyzed_at": item.Analysis.AnalyzedAt.Add(time.Hour), "schedule_extraction_version": item.Analysis.ExtractionVersion}})
	plan, e = repo.Preview(ctx, item)
	if e != nil || plan.Outcome != "analysis_version_conflict" {
		return fmt.Errorf("downgrade not blocked")
	}
	if media != "" {
		if e = ApplyMedia(ctx, store, name, media, out, true); e != nil {
			return e
		}
		if e = ApplyMedia(ctx, store, name, media, out, true); e != nil {
			return e
		}
	}
	return Write(out, "local-integration.json", map[string]any{"isolated_loopback": true, "ssu_updated": updated, "lite_upgrades": upgrades, "duplicate_noops": duplicates, "original_fields_preserved": true, "bson_readback": true, "content_CAS_blocked": true, "newer_analysis_protected": true, "media_twice": media != "", "database_cleanup": true})
}
