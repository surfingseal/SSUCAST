package main

import (
	"context"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/mvp"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/scheduleapply"
	"flag"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "MVP operation failed:", e)
		os.Exit(1)
	}
}
func run() error {
	phase := flag.String("phase", "baseline", "explicit phase")
	out := flag.String("output", "", "new output directory")
	baseline := flag.String("baseline", "", "verified backup directory")
	lite := flag.String("lite", "", "original applied ten bundle")
	input := flag.String("input", "", "prepared input or collected contents")
	prior := flag.String("prior", "", "prior actual Flash76 directory")
	ai := flag.String("ai", "", "new checkpoint directory")
	media := flag.String("media", "", "prepared media input")
	execute := flag.Bool("execute", false, "explicit paid/manual action")
	flag.Parse()
	if *out == "" {
		return fmt.Errorf("output required")
	}
	cfg, e := config.LoadMongoConnection("", "")
	if e != nil {
		return e
	}
	if cfg.Database != "ssu_crawler" {
		return fmt.Errorf("expected ssu_crawler")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()
	store, e := repository.OpenReadOnly(ctx, cfg.URI, cfg.Database)
	if e != nil {
		return fmt.Errorf("Mongo connection failed (credentials omitted)")
	}
	defer store.Client.Disconnect(context.Background())
	switch *phase {
	case "baseline":
		return mvp.Snapshot(ctx, store, *out, true)
	case "final-audit":
		return mvp.FinalAudit(ctx, store, *baseline, *out)
	case "prepare-media":
		return mvp.PrepareMedia(*input, *ai, *out)
	case "apply-media":
		return mvp.ApplyMedia(ctx, store, cfg.Database, *input, *out, *execute)
	case "local-check":
		return mvp.LocalCheck(ctx, *baseline, *input, *media, *out)
	case "prepare-ssu":
		return mvp.PrepareSSU(*baseline, *prior, *ai, *out)
	case "apply-ssu":
		return mvp.ApplySSU(ctx, store, *input, *baseline, *out, *execute)
	case "analyze-media":
		var targets []model.Content
		if mvp.Read(*input, &targets) != nil || len(targets) == 0 || len(targets) > 100 {
			return fmt.Errorf("media capture required")
		}
		for _, v := range targets {
			if v.SourceName != "ssu_global_media" {
				return fmt.Errorf("unexpected source")
			}
		}
		if !*execute {
			return mvp.Write(*out, "media-preview.json", map[string]any{"targets": len(targets), "requests": 0})
		}
		vertex, e := classification.LoadConfig()
		if e != nil {
			return e
		}
		return mvp.Analyze(ctx, *out, targets, vertex)
	case "analyze-ten":
		var bundle scheduleapply.Bundle
		if mvp.Read(*lite, &bundle) != nil || len(bundle.Items) != 10 {
			return fmt.Errorf("ten-notice input required")
		}
		var active []model.Content
		if mvp.Read(filepath.Join(*baseline, "active-originals.json"), &active) != nil {
			return fmt.Errorf("backup originals required")
		}
		by := map[string]model.Content{}
		for _, v := range active {
			by[v.ID.Hex()] = v
		}
		targets := []model.Content{}
		for _, v := range bundle.Items {
			o, ok := by[v.Original.ID.Hex()]
			if !ok {
				return fmt.Errorf("ten missing in backup")
			}
			var now model.Content
			if store.Client.Database(cfg.Database).Collection("crawled_contents").FindOne(ctx, bson.M{"_id": o.ID}).Decode(&now) != nil || scheduleapply.InputHash(now) != scheduleapply.InputHash(o) || now.ContentHash != o.ContentHash {
				return fmt.Errorf("current source changed")
			}
			if now.ScheduleModelVersion == mvp.AIModel {
				return fmt.Errorf("ten already has Flash analysis; reuse history before generating")
			}
			repo := repository.NewScheduleAnalyses(store.Client.Database(cfg.Database))
			plan, e := repo.Preview(ctx, v)
			if e != nil || plan.Outcome != "duplicate" {
				return fmt.Errorf("existing Lite projection linkage mismatch")
			}
			if e = repo.VerifyScheduleHistory(ctx, v); e != nil {
				return e
			}
			targets = append(targets, o)
		}
		if !*execute {
			return mvp.Write(*out, "ten-preview.json", map[string]any{"ids": func() []string {
				x := []string{}
				for _, v := range targets {
					x = append(x, v.ID.Hex())
				}
				return x
			}(), "model": mvp.AIModel, "requests": 0})
		}
		vertex, e := classification.LoadConfig()
		if e != nil {
			return e
		}
		return mvp.Analyze(ctx, *out, targets, vertex)
	default:
		return fmt.Errorf("unknown phase")
	}
}
