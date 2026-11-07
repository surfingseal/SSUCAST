package mvp

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/schedulebatch"
	"example.com/crawlserver/internal/schedules"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"path/filepath"
	"reflect"
	"time"
)

type MediaPrepared struct {
	Version    string                 `json:"version"`
	Source     model.Source           `json:"source"`
	Items      []scheduleapply.Item   `json:"items"`
	Categories []model.Classification `json:"classifications"`
}

func PrepareMedia(capture, ai, out string) error {
	var source model.Source
	var docs []model.Content
	var cps []schedulebatch.Checkpoint
	if Read(filepath.Join(capture, "source.json"), &source) != nil || Read(filepath.Join(capture, "contents.json"), &docs) != nil || Read(filepath.Join(ai, "results.json"), &cps) != nil {
		return fmt.Errorf("media source/results unavailable")
	}
	p := MediaPrepared{Version: schedules.ProductionVersion, Source: source, Items: []scheduleapply.Item{}, Categories: []model.Classification{}}
	by := map[string]schedulebatch.Checkpoint{}
	for _, c := range cps {
		by[c.ID] = c
	}
	failures := []map[string]string{}
	for _, v := range docs {
		c, ok := by[v.ID.Hex()]
		if !ok {
			failures = append(failures, map[string]string{"id": v.ID.Hex(), "error": "no_successful_model_response"})
			continue
		}
		if c.InputHash != scheduleapply.InputHash(v) || c.ModelVersion != AIModel || c.Error != "" || c.HTTPStatus != 200 {
			return fmt.Errorf("media checkpoint mismatch")
		}
		v.ClassificationInputHash = classification.PolicyInputHash(source, v)
		usage := bson.M{}
		b, _ := json.Marshal(c.Usage)
		json.Unmarshal(b, &usage)
		item, e := scheduleapply.PrepareProduction(v, c.RawResponse, AIModel, usage, c.CompletedAt)
		if e != nil {
			failures = append(failures, map[string]string{"id": v.ID.Hex(), "error": e.Error()})
			continue
		}
		category, e := MediaLabels(source, v, c.RawResponse)
		if e != nil {
			failures = append(failures, map[string]string{"id": v.ID.Hex(), "error": e.Error()})
			continue
		}
		category.ClassifiedAt = &c.CompletedAt
		if c.Usage != nil {
			category.InputTokens = c.Usage.PromptTokenCount
			category.OutputTokens = c.Usage.CandidatesTokenCount
		}
		raw := category.ClassificationLabels
		category.RawLabels = &raw
		p.Items = append(p.Items, item)
		p.Categories = append(p.Categories, category)
	}
	if e := Write(out, "media-validation-failures.json", failures); e != nil {
		return e
	}
	return Write(out, "media-prepared.json", p)
}
func ApplyMedia(ctx context.Context, store *repository.Store, dbName, input, out string, apply bool) error {
	var p MediaPrepared
	if Read(input, &p) != nil || p.Version != schedules.ProductionVersion || p.Source.Name != "ssu_global_media" || p.Source.Enabled || !p.Source.PolicyApproved || p.Source.ID.IsZero() || p.Source.ClassificationMode() != "hybrid" || p.Source.Automatic() || len(p.Items) == 0 || len(p.Items) != len(p.Categories) {
		return fmt.Errorf("validated manual media input required")
	}
	db := store.Client.Database(dbName)
	repo := repository.NewScheduleAnalyses(db)
	plans := []map[string]any{}
	seen := map[bson.ObjectID]bool{}
	var existing model.Source
	e := db.Collection("crawl_sources").FindOne(ctx, bson.M{"name": p.Source.Name}).Decode(&existing)
	if e != nil && e != mongo.ErrNoDocuments {
		return e
	}
	if e == nil && (existing.ID != p.Source.ID || existing.ClassificationPolicyHash() != p.Source.ClassificationPolicyHash()) {
		return fmt.Errorf("media source identity/policy conflict")
	}
	for i, item := range p.Items {
		v := item.Original
		r := p.Categories[i]
		if seen[v.ID] || v.SourceID != p.Source.ID || v.SourceName != p.Source.Name || v.ExternalPostID == "" || model.IsExcludedNotice(v) || scheduleapply.ValidateItem(item) != nil || r.ContentID != v.ID || r.InputHash != classification.PolicyInputHash(p.Source, v) {
			return fmt.Errorf("media prepared identity invalid")
		}
		seen[v.ID] = true
		checked, e := MediaLabels(p.Source, v, item.Analysis.RawResponse)
		if e != nil || checked.InputHash != r.InputHash || !reflect.DeepEqual(checked.ClassificationLabels, r.ClassificationLabels) || checked.ID != r.ID || r.ContentHash != v.ContentHash || r.ClassificationPolicyHash != p.Source.ClassificationPolicyHash() || r.ModelVersion != AIModel || r.TaxonomyVersion != classification.TaxonomyVersion || r.ClassifiedAt == nil || !r.ClassifiedAt.UTC().Truncate(time.Millisecond).Equal(item.Analysis.AnalyzedAt) {
			return fmt.Errorf("media category validation mismatch")
		}
		var old model.Content
		e = db.Collection("crawled_contents").FindOne(ctx, bson.M{"$or": bson.A{bson.M{"_id": v.ID}, bson.M{"original_url": v.OriginalURL}, bson.M{"source_id": v.SourceID, "external_post_id": v.ExternalPostID}}}).Decode(&old)
		outcome := "new"
		if e != nil && e != mongo.ErrNoDocuments {
			return e
		}
		if e == nil {
			if old.ID != v.ID || old.SourceID != v.SourceID || old.ContentHash != v.ContentHash || scheduleapply.InputHash(old) != scheduleapply.InputHash(v) {
				return fmt.Errorf("media original identity/input conflict")
			}
			plan, e := repo.Preview(ctx, item)
			if e != nil {
				return e
			}
			outcome = plan.Outcome
			if outcome != "eligible" && outcome != "duplicate" {
				return fmt.Errorf("media analysis conflict: %s", outcome)
			}
		}
		plans = append(plans, map[string]any{"id": v.ID, "title": v.Title, "outcome": outcome, "category": r.PrimaryCategory, "schedules": len(item.Analysis.ValidatedOutput.Schedules), "review_required": item.Analysis.ValidatedOutput.ScheduleReviewRequired})
	}
	report := map[string]any{"mode": "preview", "targets": len(p.Items), "plans": plans, "atlas_writes": 0}
	if e := Write(out, "preview-media.json", report); e != nil {
		return e
	}
	if !apply {
		return nil
	}
	if existing.ID.IsZero() {
		if _, e = db.Collection("crawl_sources").InsertOne(ctx, p.Source); e != nil {
			return e
		}
	}
	inserted, updated, duplicates, schedulesCount := 0, 0, 0, 0
	categories := map[string]int{}
	for i, item := range p.Items {
		v := item.Original
		// Deterministic IDs plus the existing unique original_url index prevent dupes.
		_, e = db.Collection("crawled_contents").InsertOne(ctx, v)
		if e == nil {
			inserted++
		} else if !mongo.IsDuplicateKeyError(e) {
			return e
		}
		plan, e := repo.Preview(ctx, item)
		if e != nil {
			return e
		}
		r, e := repo.Apply(ctx, item, plan)
		if e != nil {
			return e
		}
		if r.Outcome == "updated" {
			updated++
		} else if r.Outcome == "duplicate" {
			duplicates++
		} else {
			return fmt.Errorf("media CAS conflict: %s", r.Outcome)
		}
		if e = store.ApplyPreparedClassification(ctx, p.Source, v, p.Categories[i]); e != nil {
			return e
		}
		var now model.Content
		if e = db.Collection("crawled_contents").FindOne(ctx, bson.M{"_id": v.ID}).Decode(&now); e != nil {
			return e
		}
		if now.Title != v.Title || now.Content != v.Content || now.ContentHash != v.ContentHash || now.Category != p.Categories[i].PrimaryCategory || now.CategorySource != "ai" || now.ScheduleModelVersion != AIModel {
			return fmt.Errorf("media readback mismatch")
		}
		if e = repo.VerifyScheduleHistory(ctx, item); e != nil {
			return e
		}
		schedulesCount += len(now.Schedules)
		categories[now.Category]++
		report["inserted"] = inserted
		report["updated"] = updated
		report["duplicates"] = duplicates
		report["stored_schedules"] = schedulesCount
		report["categories"] = categories
		report["mode"] = "apply"
		delete(report, "atlas_writes")
		report["content_inserts"] = inserted
		report["schedule_projection_updates"] = updated
		report["classification_projections_verified"] = i + 1
		if e = Write(out, "apply-media.json", report); e != nil {
			return e
		}
	}
	return nil
}
