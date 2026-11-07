// Manual read-only checksum audit. Does not create indexes or mutate MongoDB.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/repository"
	"flag"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "read-only audit failed (credentials omitted)")
		os.Exit(1)
	}
}
func run() error {
	out := flag.String("output", "", "local audit JSON path")
	flag.Parse()
	if *out == "" {
		return fmt.Errorf("output required")
	}
	cfg, e := config.LoadMongoConnection("", "")
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, e := repository.OpenReadOnly(ctx, cfg.URI, cfg.Database)
	if e != nil {
		return e
	}
	defer store.Client.Disconnect(context.Background())
	db := store.Client.Database(cfg.Database)
	report := map[string]any{"at": time.Now().UTC(), "database": cfg.Database, "atlas_writes": 0, "gemini_calls": 0, "school_requests": 0}
	hashes := map[string]string{}
	counts := map[string]int{}
	sources := map[string]int{}
	models := map[string]int{}
	for _, name := range []string{"crawled_contents", "crawl_sources", "schedule_analyses", "content_classifications"} {
		c, e := db.Collection(name).Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
		if e != nil {
			return e
		}
		hash := sha256.New()
		n := 0
		for c.Next(ctx) {
			hash.Write(c.Current)
			n++
			if name == "crawled_contents" {
				var v struct {
					Source string `bson:"source_name"`
					Model  string `bson:"schedule_model_version"`
				}
				if bson.Unmarshal(c.Current, &v) != nil {
					return fmt.Errorf("invalid BSON")
				}
				sources[v.Source]++
				if v.Model != "" {
					models[v.Model]++
				}
			}
		}
		if c.Err() != nil {
			return c.Err()
		}
		c.Close(ctx)
		hashes[name] = fmt.Sprintf("%x", hash.Sum(nil))
		counts[name] = n
	}
	indexes := map[string]any{}
	c, e := db.Collection("crawled_contents").Indexes().List(ctx)
	if e != nil {
		return e
	}
	var idx []bson.M
	if e = c.All(ctx, &idx); e != nil {
		return e
	}
	indexes["crawled_contents"] = idx
	var explain bson.M
	e = db.RunCommand(ctx, bson.D{{Key: "explain", Value: bson.D{{Key: "find", Value: "crawled_contents"}, {Key: "filter", Value: bson.M{"source_name": "ssu_global_media"}}, {Key: "sort", Value: bson.D{{Key: "published_at", Value: -1}}}, {Key: "limit", Value: 12}}}, {Key: "verbosity", Value: "queryPlanner"}}).Decode(&explain)
	if e != nil {
		return e
	}
	report["hashes"] = hashes
	report["counts"] = counts
	report["source_counts"] = sources
	report["latest_models"] = models
	report["indexes"] = indexes
	report["source_query_plan"] = explain["queryPlanner"]
	b, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(*out), 0700); e != nil {
		return e
	}
	return os.WriteFile(*out, b, 0600)
}
