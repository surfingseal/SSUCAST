package mvp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/schedulebatch"
	"example.com/crawlserver/internal/scheduleexperiment"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"os"
	"path/filepath"
	"time"
)

func Hash(v []byte) string { return fmt.Sprintf("%x", sha256.Sum256(v)) }
func Write(dir, name string, v any) error {
	return scheduleexperiment.Write(filepath.Join(dir, name), v)
}
func Read(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return fmt.Errorf("input file unavailable")
	}
	return json.Unmarshal(b, v)
}
func Collection(ctx context.Context, db *mongo.Database, name string) ([]bson.Raw, string, error) {
	c, e := db.Collection(name).Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if e != nil {
		return nil, "", fmt.Errorf("collection read failed: %s", name)
	}
	defer c.Close(ctx)
	rows := []bson.Raw{}
	var buf bytes.Buffer
	for c.Next(ctx) {
		r := append(bson.Raw{}, c.Current...)
		rows = append(rows, r)
		buf.Write(r)
	}
	if e = c.Err(); e != nil {
		return nil, "", fmt.Errorf("cursor failed")
	}
	return rows, Hash(buf.Bytes()), nil
}
func ReadDump(path string) ([]bson.Raw, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	rows := []bson.Raw{}
	for len(b) > 0 {
		if len(b) < 5 {
			return nil, fmt.Errorf("truncated BSON")
		}
		n := int(binary.LittleEndian.Uint32(b[:4]))
		if n < 5 || n > len(b) {
			return nil, fmt.Errorf("invalid BSON length")
		}
		r := append(bson.Raw{}, b[:n]...)
		if r.Validate() != nil {
			return nil, fmt.Errorf("invalid BSON")
		}
		rows = append(rows, r)
		b = b[n:]
	}
	return rows, nil
}
func Snapshot(ctx context.Context, store *repository.Store, dir string, restore bool) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	db := store.Client.Database("ssu_crawler")
	counts := map[string]int{}
	hashes := map[string]string{}
	all := map[string][]bson.Raw{}
	status, models, sources, classes, life := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	originals := []model.Content{}
	perID := map[string]string{}
	for _, name := range []string{"crawled_contents", "crawl_sources", "schedule_analyses", "content_classifications"} {
		rows, h, e := Collection(ctx, db, name)
		if e != nil {
			return e
		}
		counts[name] = len(rows)
		hashes[name] = h
		all[name] = rows
		var b bytes.Buffer
		for _, r := range rows {
			b.Write(r)
		}
		file := filepath.Join(dir, name+".bson")
		if _, e := os.Stat(file); e == nil {
			old, e := os.ReadFile(file)
			if e != nil || Hash(old) != h {
				return fmt.Errorf("refusing overwrite of changed backup")
			}
		} else if os.IsNotExist(e) {
			if e = os.WriteFile(file, b.Bytes(), 0600); e != nil {
				return e
			}
		} else {
			return e
		}
		parsed, e := ReadDump(file)
		if e != nil || len(parsed) != len(rows) {
			return fmt.Errorf("backup reparse failed")
		}
		if name == "crawled_contents" {
			for _, r := range rows {
				var v model.Content
				if bson.Unmarshal(r, &v) != nil {
					return fmt.Errorf("decode")
				}
				sources[v.SourceName]++
				models[v.ScheduleModelVersion]++
				classes[v.ClassificationStatus]++
				life[v.LifecycleStatus]++
				if v.SourceName == "ssu_catch" {
					status[v.Status]++
					if v.Status == "진행" || v.Status == "unknown" {
						originals = append(originals, v)
					}
				}
				perID[v.ID.Hex()] = Hash(r)
			}
		}
	}
	indexes := map[string][]bson.M{}
	for _, name := range []string{"crawled_contents", "crawl_sources"} {
		cur, e := db.Collection(name).Indexes().List(ctx)
		if e != nil {
			return fmt.Errorf("index inspection failed")
		}
		var rows []bson.M
		if e = cur.All(ctx, &rows); e != nil {
			return e
		}
		cur.Close(ctx)
		indexes[name] = rows
	}
	if e := Write(dir, "indexes.json", indexes); e != nil {
		return e
	}
	if e := Write(dir, "active-originals.json", originals); e != nil {
		return e
	}
	report := map[string]any{"database": "ssu_crawler", "counts": counts, "hashes": hashes, "source_counts": sources, "schedule_model_counts": models, "ssu_status_counts": status, "classification_status_counts": classes, "lifecycle_status_counts": life, "per_document_hashes": perID, "bson_reparse_valid": true, "active_ssu_count": len(originals), "atlas_writes": 0, "at": time.Now().UTC()}
	if restore {
		if e := RestoreCheck(ctx, all); e != nil {
			return e
		}
		report["loopback_restore_exact"] = true
	}
	if e := Write(dir, "baseline.json", report); e != nil {
		return e
	}
	fmt.Printf("Atlas baseline contents=%d activeSSU=%d histories=%d, backup BSON verified, restore=%v\n", counts["crawled_contents"], len(originals), counts["schedule_analyses"], restore)
	return nil
}
func RestoreCheck(ctx context.Context, all map[string][]bson.Raw) error {
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
		return fmt.Errorf("isolated restore connection failed")
	}
	defer store.Client.Disconnect(context.Background())
	db := store.Client.Database(name)
	names, e := db.ListCollectionNames(ctx, bson.M{})
	if e != nil || len(names) != 0 {
		return fmt.Errorf("temporary restore database not empty")
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
	for collection, rows := range all {
		if len(rows) == 0 {
			continue
		}
		docs := []any{}
		for _, r := range rows {
			docs = append(docs, r)
		}
		if _, e = db.Collection(collection).InsertMany(ctx, docs); e != nil {
			return fmt.Errorf("restore insert failed")
		}
		restored, _, e := Collection(ctx, db, collection)
		if e != nil || len(restored) != len(rows) {
			return fmt.Errorf("restore count failed")
		}
		for i, r := range rows {
			if !bytes.Equal(r, restored[i]) {
				return fmt.Errorf("BSON restore mismatch")
			}
		}
	}
	return nil
}

// Verify the frozen 76 and their original input hashes, independently of which
// projections passed the old comparison's stricter summary length check.
func VerifyPrior76(dir string, originals []model.Content) error {
	var cp []schedulebatch.Checkpoint
	if e := Read(filepath.Join(dir, "flash-raw-results.json"), &cp); e != nil {
		return e
	}
	if len(cp) != 76 {
		return fmt.Errorf("missing real Flash responses")
	}
	by := map[string]model.Content{}
	for _, v := range originals {
		by[v.ID.Hex()] = v
	}
	seen := map[string]bool{}
	for _, c := range cp {
		v, ok := by[c.ID]
		if !ok || seen[c.ID] || c.ModelVersion != "gemini-3.8-flash" || c.HTTPStatus != 200 || c.Error != "" || c.InputHash != scheduleapply.InputHash(v) {
			return fmt.Errorf("prior Flash/source mismatch: %s", c.ID)
		}
		seen[c.ID] = true
	}
	return nil
}
