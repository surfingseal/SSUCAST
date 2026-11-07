package mvp

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"net/http/httptest"
	"os"
	"path/filepath"
)

func FinalAudit(ctx context.Context, store *repository.Store, baseline, out string) error {
	db := store.Client.Database("ssu_crawler")
	rows, _, e := Collection(ctx, db, "crawled_contents")
	if e != nil {
		return e
	}
	originals, e := ReadDump(filepath.Join(baseline, "crawled_contents.bson"))
	if e != nil {
		return e
	}
	base := map[bson.ObjectID]string{}
	for _, r := range originals {
		var v model.Content
		bson.Unmarshal(r, &v)
		h, _ := OriginalHash(r)
		base[v.ID] = h
	}
	counts, models, cats, states := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	review, ssuFlash, mediaAnalyzed, schedulesCount, mediaSchedules, links, unchanged := 0, 0, 0, 0, 0, 0, 0
	reasons := []map[string]any{}
	for _, r := range rows {
		var v model.Content
		if bson.Unmarshal(r, &v) != nil {
			return fmt.Errorf("invalid readback")
		}
		counts[v.SourceName]++
		if h, ok := base[v.ID]; ok {
			actual, _ := OriginalHash(r)
			if h != actual {
				return fmt.Errorf("SSU original changed")
			}
			unchanged++
		}
		if v.ScheduleModelVersion != "" {
			models[v.ScheduleModelVersion]++
		}
		if v.SourceName == "ssu_catch" && v.ScheduleModelVersion == AIModel {
			ssuFlash++
		}
		if v.SourceName == "ssu_global_media" {
			mediaAnalyzed++
			mediaSchedules += len(v.Schedules)
			cats[v.Category]++
			if v.ClassificationMethod != "ai" || v.CategorySource != "ai" {
				return fmt.Errorf("media classification missing")
			}
		}
		if v.ScheduleReviewRequired {
			review++
			reasons = append(reasons, map[string]any{"id": v.ID.Hex(), "source": v.SourceName, "title": v.Title, "schedule_quality": v.ScheduleQualityState, "summary_review": v.SummaryReviewRequired, "audience_review": v.TargetAudienceReviewRequired, "audience_reasons": v.TargetAudienceReviewReasons})
		}
		schedulesCount += len(v.Schedules)
		for _, s := range v.Schedules {
			if s.Quality != nil {
				states[s.Quality.State]++
			}
		}
		if v.ScheduleAnalysisID != "" {
			var a struct {
				ContentID bson.ObjectID `bson:"content_id"`
				InputHash string        `bson:"input_hash"`
				Model     string        `bson:"model_version"`
			}
			if db.Collection("schedule_analyses").FindOne(ctx, bson.M{"_id": v.ScheduleAnalysisID}).Decode(&a) != nil || a.ContentID != v.ID || a.InputHash != v.ScheduleInputHash || a.Model != v.ScheduleModelVersion {
				return fmt.Errorf("analysis history linkage failed")
			}
			links++
		}
	}
	old, e := ReadDump(filepath.Join(baseline, "schedule_analyses.bson"))
	if e != nil {
		return e
	}
	for _, r := range old {
		var v bson.M
		bson.Unmarshal(r, &v)
		var now bson.Raw
		if db.Collection("schedule_analyses").FindOne(ctx, bson.M{"_id": v["_id"]}).Decode(&now) != nil || Hash(r) != Hash(now) {
			return fmt.Errorf("old history changed")
		}
	}
	histories, _ := db.Collection("schedule_analyses").CountDocuments(ctx, bson.M{})
	classes, _ := db.Collection("content_classifications").CountDocuments(ctx, bson.M{})
	sources, _ := db.Collection("crawl_sources").CountDocuments(ctx, bson.M{})
	// Exercise the existing authenticated HTTP handler against actual read-only Atlas data.
	token := os.Getenv("ADMIN_TOKEN")
	if len(token) < 24 {
		return fmt.Errorf("API verification token missing")
	}
	handler := api.API{Store: store, Token: token}.Handler()
	checks := []map[string]any{}
	for _, path := range []string{"/api/v1/contents?source=ssu_catch&limit=100", "/api/v1/contents?source=ssu_global_media&limit=100", "/api/v1/contents?source=ssu_global_media&category=scholarship&limit=100", "/api/v1/sources"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != 200 {
			return fmt.Errorf("API read failed: %s status%d", path, rec.Code)
		}
		var v any
		if json.Unmarshal(rec.Body.Bytes(), &v) != nil {
			return fmt.Errorf("API invalid JSON")
		}
		n := 0
		if a, ok := v.([]any); ok {
			n = len(a)
		}
		if m, ok := v.(map[string]any); ok {
			if a, ok := m["items"].([]any); ok {
				n = len(a)
			}
		}
		expected := map[string]int{"/api/v1/contents?source=ssu_catch&limit=100": 86, "/api/v1/contents?source=ssu_global_media&limit=100": 14, "/api/v1/contents?source=ssu_global_media&category=scholarship&limit=100": 3, "/api/v1/sources": 2}
		if n != expected[path] {
			return fmt.Errorf("API filtered count mismatch %s: %d", path, n)
		}
		checks = append(checks, map[string]any{"path": path, "status": rec.Code, "items": n})
	}
	req := httptest.NewRequest("GET", "/api/v1/contents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 401 {
		return fmt.Errorf("API authentication regression")
	}
	report := map[string]any{"contents": len(rows), "source_counts": counts, "latest_models": models, "ssu_flash": ssuFlash, "ssu_originals_unchanged": unchanged, "media_analyzed": mediaAnalyzed, "media_schedules": mediaSchedules, "media_categories": cats, "total_schedules": schedulesCount, "review_notices": review, "schedule_quality_states": states, "schedule_analyses": histories, "content_classifications": classes, "sources": sources, "linked_latest_analyses": links, "old_lite_histories_preserved": len(old), "api_checks": checks, "unauthorized_status": 401, "atlas_writes": 0}
	if e = Write(out, "final-audit.json", report); e != nil {
		return e
	}
	if e = Write(out, "review-notices.json", reasons); e != nil {
		return e
	}
	if counts["ssu_catch"] != 212 || unchanged != 212 || ssuFlash != 86 || mediaAnalyzed != 14 || links != 100 {
		return fmt.Errorf("final count differs from validated batch")
	}
	fmt.Printf("Atlas verified contents=%d SSU=%d Flash=%d media=%d histories=%d schedules=%d\n", len(rows), counts["ssu_catch"], ssuFlash, mediaAnalyzed, histories, schedulesCount)
	return nil
}
