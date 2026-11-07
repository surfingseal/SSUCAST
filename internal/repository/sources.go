package repository

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"example.com/crawlserver/internal/model"
)

type SourceRepository struct{ sources *mongo.Collection }

func (s *SourceRepository) Seed(ctx context.Context, url, schedule string) error {
	return s.seedLegacy(ctx, url, schedule, false)
}
func (s *SourceRepository) SeedDemo(ctx context.Context, url, schedule string) error {
	return s.seedLegacy(ctx, url, schedule, true)
}
func (s *SourceRepository) seedLegacy(ctx context.Context, url, schedule string, auto bool) error {
	now := time.Now().UTC()
	_, err := s.sources.UpdateOne(ctx, bson.M{"name": "example"}, bson.M{"$set": bson.M{"base_url": url, "schedule": schedule, "updated_at": now}, "$setOnInsert": bson.M{"_id": bson.NewObjectID(), "name": "example", "enabled": true, "auto_enabled": auto, "created_at": now, "last_crawled_at": nil}}, options.UpdateOne().SetUpsert(true))
	return err
}
func (s *SourceRepository) Sources(ctx context.Context) ([]model.Source, error) {
	cur, err := s.sources.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := cur.Close(ctx); err != nil {
			slog.Warn("cursor close failed", "error", err)
		}
	}()
	out := []model.Source{}
	err = cur.All(ctx, &out)
	for i := range out {
		out[i].ClassificationStrategy = out[i].ClassificationMode()
		if out[i].ClassificationRevision < 1 {
			out[i].ClassificationRevision = 1
		}
	}
	return out, err
}
func (s *SourceRepository) Source(ctx context.Context, id bson.ObjectID) (model.Source, error) {
	var v model.Source
	err := s.sources.FindOne(ctx, bson.M{"_id": id}).Decode(&v)
	v.ClassificationStrategy = v.ClassificationMode()
	if v.ClassificationRevision < 1 {
		v.ClassificationRevision = 1
	}
	return v, err
}

// ApplySources only overwrites explicit configuration fields. IDs, creation time
// and crawl history survive restarts; omitted IDs are matched by unique name.
func (s *SourceRepository) ApplySources(ctx context.Context, sources []model.Source) error {
	for _, v := range sources {
		if err := v.ValidateClassificationStrategy(); err != nil {
			return err
		}
		v.ClassificationStrategy = v.ClassificationMode()
		filter := bson.M{"name": v.Name}
		if !v.ID.IsZero() {
			var existing model.Source
			err := s.sources.FindOne(ctx, bson.M{"name": v.Name}).Decode(&existing)
			if err != nil && err != mongo.ErrNoDocuments {
				return err
			}
			if err == nil && existing.ID != v.ID {
				return fmt.Errorf("source %s id conflicts with existing name", v.Name)
			}
			filter = bson.M{"_id": v.ID}
		}
		id := v.ID
		if id.IsZero() {
			id = bson.NewObjectID()
		}
		now := time.Now().UTC()
		auto := false
		if v.AutoEnabled != nil {
			auto = *v.AutoEnabled
		}
		set := bson.M{"display_name": v.DisplayName, "name": v.Name, "crawler_type": v.CrawlerType, "base_url": v.BaseURL, "list_url": v.ListURL, "enabled": v.Enabled, "auto_enabled": auto, "schedule": v.Schedule, "request_interval_ms": v.RequestIntervalMS, "max_pages": v.MaxPages, "max_contents": v.MaxContents, "max_requests": v.MaxRequests, "max_concurrency": v.MaxConcurrency, "updated_at": now}
		set["policy_approved"] = v.PolicyApproved
		set["collection_mode"] = v.CollectionMode
		set["policy_approval_basis"] = v.PolicyApprovalBasis
		set["request_jitter_ms"] = v.RequestJitterMS
		set["max_retries"] = v.MaxRetries
		set["detail_body_selector"] = v.DetailBodySelector
		strategy := v.ClassificationMode()
		revision := bson.M{"$cond": bson.A{bson.M{"$ne": bson.A{bson.M{"$ifNull": bson.A{"$classification_strategy", "source"}}, strategy}}, bson.M{"$add": bson.A{bson.M{"$ifNull": bson.A{"$classification_revision", 0}}, 1}}, bson.M{"$ifNull": bson.A{"$classification_revision", 1}}}}
		fields := bson.M{}
		for k, value := range set {
			fields[k] = bson.M{"$literal": value}
		}
		fields["classification_strategy"] = bson.M{"$literal": strategy}
		fields["classification_revision"] = revision
		fields["_id"] = bson.M{"$ifNull": bson.A{"$_id", id}}
		fields["created_at"] = bson.M{"$ifNull": bson.A{"$created_at", now}}
		fields["last_crawled_at"] = bson.M{"$ifNull": bson.A{"$last_crawled_at", nil}}
		if _, err := s.sources.UpdateOne(ctx, filter, mongo.Pipeline{bson.D{{Key: "$set", Value: fields}}}, options.UpdateOne().SetUpsert(true)); err != nil {
			return fmt.Errorf("source %s: %w", v.Name, err)
		}
	}
	return nil
}
