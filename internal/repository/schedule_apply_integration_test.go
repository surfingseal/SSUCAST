package repository_test

import (
 "example.com/crawlserver/internal/testutil/localfixture"
	"bytes"
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/schedules"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func actualSchedules(t *testing.T) scheduleapply.Bundle {
	t.Helper()
	localfixture.Require(t, "../scheduleapply/testdata/actual10.json")
	raw, e := os.ReadFile("../scheduleapply/testdata/actual10.json")
	if e != nil {
		t.Fatal(e)
	}
	var b scheduleapply.Bundle
	if e = json.Unmarshal(raw, &b); e != nil {
		t.Fatal(e)
	}
	if e = scheduleapply.ValidateBundle(b); e != nil {
		t.Fatal(e)
	}
	return b
}
func isolatedScheduleStore(t *testing.T) (*repository.Store, string) {
	t.Helper()
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("set a loopback TEST_MONGODB_URI; production/Atlas forbidden")
	}
	name := "ssu_schedule_apply_test_" + bson.NewObjectID().Hex()
	if e := config.ValidateIsolatedTestMongo(uri, name); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s, e := repository.OpenReadOnly(ctx, uri, name)
	if e != nil {
		t.Fatal(e)
	}
	db := s.Client.Database(name)
	names, e := db.ListCollectionNames(ctx, bson.M{})
	if e != nil || len(names) != 0 {
		s.Client.Disconnect(ctx)
		t.Fatal("test database must be fresh and empty")
	}
	owner := bson.NewObjectID()
	if _, e = db.Collection("test_control").InsertOne(ctx, bson.M{"_id": owner, "purpose": "isolated schedule apply integration"}); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		defer s.Client.Disconnect(ctx)
		if e := config.ValidateIsolatedTestMongo(uri, name); e != nil {
			t.Error(e)
			return
		}
		n, e := db.Collection("test_control").CountDocuments(ctx, bson.M{"_id": owner})
		if e != nil || n != 1 {
			t.Error("ownership marker mismatch; refusing cleanup")
			return
		}
		if e = db.Drop(ctx); e != nil {
			t.Error(e)
		}
	})
	// Store repositories retain this DB, and only a server-issued random name can
	// reach a write. No .env loading or Atlas connection takes place in tests.
	return s, name
}
func seedSchedules(t *testing.T, s *repository.Store, name string, items []scheduleapply.Item) {
	t.Helper()
	ctx := context.Background()
	docs := []any{}
	sources := map[bson.ObjectID]bool{}
	for _, v := range items {
		docs = append(docs, v.Original)
		sources[v.Original.SourceID] = true
	}
	if _, e := s.Client.Database(name).Collection("crawled_contents").InsertMany(ctx, docs); e != nil {
		t.Fatal(e)
	}
	for id := range sources {
		if _, e := s.Client.Database(name).Collection("crawl_sources").InsertOne(ctx, model.Source{ID: id, Name: "ssu_catch", ClassificationStrategy: "source"}); e != nil {
			t.Fatal(e)
		}
	}
}
func originalFields(t *testing.T, raw bson.Raw) []byte {
	t.Helper()
	var doc bson.D
	if e := bson.Unmarshal(raw, &doc); e != nil {
		t.Fatal(e)
	}
	keep := bson.D{}
	for _, v := range doc {
		switch v.Key {
		case "schedule_validation_version", "schedule_quality_state", "summary_quality_state", "audience_quality_state", "summary", "target_audience", "target_audience_review_required", "target_audience_review_reasons", "summary_review_required", "schedules", "schedule_extraction_version", "schedule_extraction_method", "schedule_review_required", "schedule_analyzed_at", "schedule_input_hash", "schedule_model_version", "schedule_result_hash", "schedule_analysis_id":
			continue
		}
		keep = append(keep, v)
	}
	b, e := bson.Marshal(keep)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestScheduleActualTenApplyReadbackIdempotencyAndAPI(t *testing.T) {
	bundle := actualSchedules(t)
	s, name := isolatedScheduleStore(t)
	db := s.Client.Database(name)
	seedSchedules(t, s, name, bundle.Items)
	repo := repository.NewScheduleAnalyses(db)
	ctx := context.Background()
	before := map[bson.ObjectID][]byte{}
	plans := []repository.SchedulePlan{}
	for _, v := range bundle.Items {
		p, e := repo.Preview(ctx, v)
		if e != nil || p.Outcome != "eligible" {
			t.Fatalf("preview %s %v", p.Outcome, e)
		}
		before[v.Original.ID] = originalFields(t, p.Snapshot)
		plans = append(plans, p)
	}
	total, base, extra, partial, audienceReview, updated := 0, 0, 0, 0, 0, 0
	for i, item := range bundle.Items {
		result, e := repo.Apply(ctx, item, plans[i])
		if e != nil || result.Modified != 1 || result.Outcome != "updated" || !result.HistoryInserted {
			t.Fatalf("apply %+v %v", result, e)
		}
		updated++
		var raw bson.Raw
		if e = db.Collection("crawled_contents").FindOne(ctx, bson.M{"_id": item.Original.ID}).Decode(&raw); e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(before[item.Original.ID], originalFields(t, raw)) {
			t.Fatal("non-analysis fields changed")
		}
		var got model.Content
		if e = bson.Unmarshal(raw, &got); e != nil {
			t.Fatal(e)
		}
		if got.Summary != item.Analysis.ValidatedOutput.Summary || got.ContentHash != item.Original.ContentHash || got.Title != item.Original.Title || got.Content != item.Original.Content || got.Category != item.Original.Category {
			t.Fatal("original/summary fields changed")
		}
		if got.TargetAudienceReviewRequired {
			audienceReview++
			if got.TargetAudience != nil {
				t.Fatal("unverified audience became confirmed")
			}
		}
		for _, v := range got.Schedules {
			total++
			if v.Timezone != "Asia/Seoul" {
				t.Fatal("timezone lost")
			}
			if v.Origin == "saved_ai_response" {
				base++
			} else if v.Origin == "source_rule_supplement" {
				extra++
			}
			if v.StartPartial != nil || v.EndPartial != nil {
				partial++
			}
			if v.EndDate != nil && *v.EndDate == "2026-11-23" && (v.EndTime != nil || v.EndAt != nil) {
				t.Fatal("date-only clock invented")
			}
		}
		if raw.Lookup("schedule_analyzed_at").Type != bson.TypeDateTime {
			t.Fatal("analysis time must be UTC BSON date")
		}
	}
	duplicates := 0
	for i, item := range bundle.Items {
		p, e := repo.Preview(ctx, item)
		if e != nil || p.Outcome != "duplicate" {
			t.Fatal("duplicate not recognized", e, p)
		}
		a, e := repo.Apply(ctx, item, p)
		if e != nil || a.Modified != 0 || a.HistoryInserted || a.Outcome != "duplicate" {
			t.Fatal("repeat changed data", a, e)
		}
		duplicates++
		// Replay the stale original preview too: history must remain immutable and
		// the failed CAS is classified as a concurrent identical duplicate.
		a, e = repo.Apply(ctx, item, plans[i])
		if e != nil || a.Modified != 0 || a.HistoryInserted || a.Outcome != "duplicate" {
			t.Fatal("stale replay not idempotent", a, e)
		}
	}
	contents, _ := db.Collection("crawled_contents").CountDocuments(ctx, bson.M{})
	histories, _ := db.Collection("schedule_analyses").CountDocuments(ctx, bson.M{})
	classes, _ := db.Collection("content_classifications").CountDocuments(ctx, bson.M{})
	if contents != 10 || histories != 10 || classes != 0 || base != 24 || extra != 4 || total != 28 || partial != 7 || audienceReview != 5 {
		t.Fatalf("counts content=%d history=%d base=%d extra=%d partial=%d audience=%d", contents, histories, base, extra, partial, audienceReview)
	}
	h := (api.API{Store: s, Token: "isolated-schedule-token"}).Handler()
	id := bundle.Items[0].Original.ID
	req := httptest.NewRequest("GET", "/api/v1/contents/"+id.Hex(), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatal("auth lost", w.Code)
	}
	req = httptest.NewRequest("GET", "/api/v1/contents/"+id.Hex(), nil)
	req.Header.Set("Authorization", "Bearer isolated-schedule-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"summary"`)) || bytes.Contains(w.Body.Bytes(), []byte(`"raw_response"`)) {
		t.Fatal("API projection invalid", w.Code)
	}
	// Legacy document without analysis fields still has the original response.
	legacy := bundle.Items[0].Original
	legacy.ID = bson.NewObjectID()
	legacy.OriginalURL += "&legacy_test=1"
	if _, e := db.Collection("crawled_contents").InsertOne(ctx, legacy); e != nil {
		t.Fatal(e)
	}
	req = httptest.NewRequest("GET", "/api/v1/contents/"+legacy.ID.Hex(), nil)
	req.Header.Set("Authorization", "Bearer isolated-schedule-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte(`"summary"`)) || bytes.Contains(w.Body.Bytes(), []byte(`"schedules"`)) {
		t.Fatal("legacy API compatibility lost", w.Code)
	}
	if _, e := db.Collection("crawled_contents").DeleteOne(ctx, bson.M{"_id": legacy.ID}); e != nil {
		t.Fatal(e)
	}
	if path := os.Getenv("SCHEDULE_APPLY_TEST_REPORT"); path != "" {
		report := map[string]any{"database": name, "isolated_loopback_verified": true, "fresh_database_verified": true, "notices": contents, "original_schedules": base, "rule_sessions": extra, "total_schedules": total, "partial_date_schedules": partial, "audience_review_notices": audienceReview, "updated": updated, "duplicates": duplicates, "history_documents": histories, "classification_documents": classes, "bson_and_original_fields_preserved": true, "api_auth_and_legacy_projection_passed": true}
		b, _ := json.MarshalIndent(report, "", "  ")
		if e := os.WriteFile(filepath.Clean(path), append(b, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
func TestScheduleCASBlocksChangesAndNewerAnalyses(t *testing.T) {
	bundle := actualSchedules(t)
	item := bundle.Items[0]
	cases := []struct {
		name    string
		set     bson.M
		outcome string
	}{
		{"changed_hash", bson.M{"content": "수정된 본문", "content_hash": "modified-hash"}, "content_hash_conflict"},
		{"body_changed_without_hash_update", bson.M{"content": "수정되었으나 오래된 hash"}, "input_hash_conflict"},
		{"newer_version", bson.M{"schedule_extraction_version": "schedule-v3", "schedule_analyzed_at": item.Analysis.AnalyzedAt.Add(time.Hour)}, "analysis_version_conflict"},
		{"newer_same_version", bson.M{"schedule_extraction_version": "schedule-v2", "schedule_analyzed_at": item.Analysis.AnalyzedAt.Add(time.Hour)}, "analysis_version_conflict"},
		{"existing_input_state_changed", bson.M{"schedule_input_hash": "different-analysis-input"}, "analysis_state_conflict"},
		{"schedules_changed", bson.M{"schedules": bson.A{bson.M{"label": "newer manual result"}}}, "analysis_state_conflict"},
		{"source_changed", bson.M{"source_name": "different_source"}, "source_conflict"},
		{"metadata_changed", bson.M{"last_seen_at": time.Now().UTC()}, "cas_conflict"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, name := isolatedScheduleStore(t)
			db := s.Client.Database(name)
			seedSchedules(t, s, name, []scheduleapply.Item{item})
			repo := repository.NewScheduleAnalyses(db)
			ctx := context.Background()
			p, e := repo.Preview(ctx, item)
			if e != nil || p.Outcome != "eligible" {
				t.Fatal(p, e)
			}
			if _, e = db.Collection("crawled_contents").UpdateOne(ctx, bson.M{"_id": item.Original.ID}, bson.M{"$set": tc.set}); e != nil {
				t.Fatal(e)
			}
			var before bson.Raw
			db.Collection("crawled_contents").FindOne(ctx, bson.M{"_id": item.Original.ID}).Decode(&before)
			a, e := repo.Apply(ctx, item, p)
			if e != nil || a.Modified != 0 || a.Outcome != tc.outcome {
				t.Fatal(a, e)
			}
			var after bson.Raw
			db.Collection("crawled_contents").FindOne(ctx, bson.M{"_id": item.Original.ID}).Decode(&after)
			if !bytes.Equal(before, after) {
				t.Fatal("CAS changed conflicting document")
			}
		})
	}
}
func TestScheduleConcurrentSameAnalysisAppliedOnce(t *testing.T) {
	item := actualSchedules(t).Items[0]
	s, name := isolatedScheduleStore(t)
	db := s.Client.Database(name)
	seedSchedules(t, s, name, []scheduleapply.Item{item})
	repo := repository.NewScheduleAnalyses(db)
	ctx := context.Background()
	p, e := repo.Preview(ctx, item)
	if e != nil {
		t.Fatal(e)
	}
	results := make(chan repository.ScheduleApplyResult, 8)
	errors := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, e := repo.Apply(ctx, item, p)
			if e != nil {
				errors <- e
			} else {
				results <- a
			}
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for e := range errors {
		t.Fatal(e)
	}
	updated, dup := 0, 0
	for a := range results {
		if a.Outcome == "updated" {
			updated++
		} else if a.Outcome == "duplicate" {
			dup++
		} else {
			t.Fatal(a)
		}
	}
	history, _ := db.Collection("schedule_analyses").CountDocuments(ctx, bson.M{})
	if updated != 1 || dup != 7 || history != 1 {
		t.Fatal(fmt.Sprintf("updated=%d duplicate=%d history=%d", updated, dup, history))
	}
}
func TestScheduleOlderAnalysisUpgradeAndImmutableHistory(t *testing.T) {
	item := actualSchedules(t).Items[0]
	t.Run("older_version_with_existing_input_hash", func(t *testing.T) {
		s, name := isolatedScheduleStore(t)
		db := s.Client.Database(name)
		seedSchedules(t, s, name, []scheduleapply.Item{item})
		ctx := context.Background()
		repo := repository.NewScheduleAnalyses(db)
		_, e := db.Collection("crawled_contents").UpdateOne(ctx, bson.M{"_id": item.Original.ID}, bson.M{"$set": bson.M{"schedule_input_hash": item.Analysis.InputHash, "schedule_extraction_version": "schedule-v1", "schedule_analyzed_at": item.Analysis.AnalyzedAt.Add(-time.Hour), "summary": "old verified summary"}})
		if e != nil {
			t.Fatal(e)
		}
		p, e := repo.Preview(ctx, item)
		if e != nil || p.Outcome != "eligible" {
			t.Fatal(p, e)
		}
		a, e := repo.Apply(ctx, item, p)
		if e != nil || a.Modified != 1 {
			t.Fatal(a, e)
		}
	})
	t.Run("existing_history_cannot_be_replaced", func(t *testing.T) {
		s, name := isolatedScheduleStore(t)
		db := s.Client.Database(name)
		seedSchedules(t, s, name, []scheduleapply.Item{item})
		ctx := context.Background()
		repo := repository.NewScheduleAnalyses(db)
		p, e := repo.Preview(ctx, item)
		if e != nil {
			t.Fatal(e)
		}
		altered := item.Analysis
		altered.RawResponse = "different immutable history"
		if _, e := db.Collection("schedule_analyses").InsertOne(ctx, altered); e != nil {
			t.Fatal(e)
		}
		a, e := repo.Apply(ctx, item, p)
		if e == nil || a.Modified != 0 {
			t.Fatal("history conflict did not block projection", a, e)
		}
		var got model.Content
		db.Collection("crawled_contents").FindOne(ctx, bson.M{"_id": item.Original.ID}).Decode(&got)
		if got.Summary != "" {
			t.Fatal("content changed despite history conflict")
		}
	})
}

// Actual source/raw response fixtures; only synthetic test DB metadata differs.
func TestScheduleQualityLoopbackProjectionAndHistory(t *testing.T) {
	localfixture.Require(t, "../schedules/testdata/actual76.json")
	raw, e := os.ReadFile("../schedules/testdata/actual76.json")
	if e != nil {
		t.Fatal(e)
	}
	var corpus struct {
		Items []struct {
			ID      string `json:"id"`
			Title   string `json:"title"`
			Content string `json:"content"`
			Raw     string `json:"raw_response"`
		} `json:"items"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("fixture")
	}
	base := actualSchedules(t).Items[0].Original
	items := []scheduleapply.Item{}
	for _, x := range corpus.Items {
		if x.ID != "6aed70eb7b8f7b8bf100c086" && x.ID != "6aed70eb7b8f7b8bf100c036" {
			continue
		}
		o := base
		o.ID, _ = bson.ObjectIDFromHex(x.ID)
		o.Title = x.Title
		o.Content = x.Content
		o.ContentHash = model.ContentDigest(o)
		item, e := scheduleapply.PrepareQuality(o, x.Raw, "gemini-3.5-flash-lite", bson.M{}, time.Now())
		if e != nil {
			t.Fatal(e)
		}
		items = append(items, item)
	}
	if len(items) != 2 {
		t.Fatal("fixture IDs")
	}
	store, name := isolatedScheduleStore(t)
	seedSchedules(t, store, name, items)
	db := store.Client.Database(name)
	repo := repository.NewScheduleAnalyses(db)
	ctx := context.Background()
	total := 0
	for _, v := range items {
		p, e := repo.Preview(ctx, v)
		if e != nil || p.Outcome != "eligible" {
			t.Fatal(p, e)
		}
		before := originalFields(t, p.Snapshot)
		result, e := repo.Apply(ctx, v, p)
		if e != nil || result.Outcome != "updated" {
			t.Fatal(result, e)
		}
		var raw bson.Raw
		if e := db.Collection("crawled_contents").FindOne(ctx, bson.M{"_id": v.Original.ID}).Decode(&raw); e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(before, originalFields(t, raw)) {
			t.Fatal("original fields changed")
		}
		var got model.Content
		if bson.Unmarshal(raw, &got) != nil {
			t.Fatal("BSON")
		}
		if got.ScheduleValidationVersion != schedules.QualityVersion || got.ScheduleQualityState != v.Analysis.ValidatedOutput.ScheduleQualityState || scheduleapply.ProjectionHash(scheduleapply.Projection{ScheduleMetadata: got.ScheduleMetadata, Schedules: got.Schedules}) != v.Analysis.ValidatedOutput.ScheduleResultHash {
			t.Fatal("quality projection roundtrip")
		}
		total += len(got.Schedules)
		p, e = repo.Preview(ctx, v)
		if e != nil || p.Outcome != "duplicate" {
			t.Fatal(p, e)
		}
		again, e := repo.Apply(ctx, v, p)
		if e != nil || again.Modified != 0 || again.HistoryInserted {
			t.Fatal("duplicate mutation", e)
		}
		if e = repo.VerifyScheduleHistory(ctx, v); e != nil {
			t.Fatal(e)
		}
	}
	if total != 4 {
		t.Fatal(total)
	}
}
