package repository

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"example.com/crawlserver/internal/lifecycle"
	"example.com/crawlserver/internal/model"
)

type ContentRepository struct{ contents *mongo.Collection }

func (s *ContentRepository) Save(ctx context.Context, items []model.Content) error {
	for start := 0; start < len(items); start += 100 {
		end := min(start+100, len(items))
		writes := make([]mongo.WriteModel, 0, end-start)
		for _, v := range items[start:end] {
			if model.IsExcludedNotice(v) {
				continue
			}
			if v.Title == "" || v.Content == "" || v.OriginalURL == "" || v.SourceID.IsZero() {
				return fmt.Errorf("invalid content")
			}
			var err error
			v.OriginalURL, err = model.NormalizeContentURL(v.OriginalURL)
			if err != nil {
				return fmt.Errorf("invalid content URL: %w", err)
			}
			if v.CrawledAt.IsZero() {
				v.CrawledAt = time.Now().UTC()
			}
			if v.LastSeenAt.IsZero() {
				v.LastSeenAt = v.CrawledAt
			}
			if v.UpdatedAt.IsZero() {
				v.UpdatedAt = v.CrawledAt
			}
			if v.ContentHash == "" {
				v.ContentHash = model.ContentDigest(v)
			}
			if v.Attachments == nil {
				v.Attachments = []model.Attachment{}
			}
			v.ID = bson.NilObjectID
			update, err := contentUpdate(v)
			if err != nil {
				return err
			}
			writes = append(writes, mongo.NewUpdateOneModel().SetFilter(bson.M{"original_url": v.OriginalURL}).SetUpdate(update).SetUpsert(true))
		}
		if len(writes) == 0 {
			continue
		}
		if _, err := s.contents.BulkWrite(ctx, writes, options.BulkWrite().SetOrdered(true)); err != nil {
			return err
		}
	}
	return nil
}

// A single atomic pipeline avoids read/compare/write races. The URL unique index
// remains the identity; _id and first successful crawled_at are preserved.
func contentUpdate(v model.Content) (mongo.Pipeline, error) {
	raw, err := bson.Marshal(v)
	if err != nil {
		return nil, err
	}
	fields := bson.M{}
	if err = bson.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	delete(fields, "_id")
	delete(fields, "last_seen_at")
	delete(fields, "crawled_at")
	delete(fields, "categories")
	delete(fields, "original_categories")
	delete(fields, "deadline")
	delete(fields, "lifecycle")
	delete(fields, "lifecycle_status")
	changed := bson.M{"$ne": bson.A{bson.M{"$ifNull": bson.A{"$content_hash", ""}}, v.ContentHash}}
	set := bson.M{}
	set["deadline"] = bson.M{"$cond": bson.A{changed, nil, "$deadline"}}
	set["lifecycle"] = bson.M{"$cond": bson.A{changed, nil, "$lifecycle"}}
	set["lifecycle_status"] = bson.M{"$cond": bson.A{changed, "needs_review", bson.M{"$ifNull": bson.A{"$lifecycle_status", "needs_review"}}}}
	for k, value := range fields {
		set[k] = bson.M{"$cond": bson.A{changed, bson.M{"$literal": value}, "$" + k}}
	}
	set["original_url"] = bson.M{"$literal": v.OriginalURL}
	set["source_name"] = bson.M{"$literal": v.SourceName}
	set["metadata"] = bson.M{"$literal": v.Metadata}
	set["crawled_at"] = bson.M{"$ifNull": bson.A{"$crawled_at", v.CrawledAt}}
	set["last_seen_at"] = bson.M{"$max": bson.A{bson.M{"$ifNull": bson.A{"$last_seen_at", v.LastSeenAt}}, v.LastSeenAt}}
	categories := append([]string{}, v.Categories...)
	if len(categories) == 0 && v.Category != "" {
		categories = append(categories, v.Category)
	}
	originals := append([]string{}, v.OriginalCategories...)
	if len(originals) == 0 && v.OriginalCategory != "" {
		originals = append(originals, v.OriginalCategory)
	}
	// A final AI category is not discovery membership. Never union an old AI
	// candidate into a new/source projection. Preserve only source memberships
	// within the same policy revision.
	samePolicy := bson.M{"$and": bson.A{bson.M{"$eq": bson.A{"$classification_policy_hash", v.ClassificationPolicyHash}}, bson.M{"$eq": bson.A{"$classification_method", "source"}}}}
	projectionSame := bson.M{"$and": bson.A{bson.M{"$eq": bson.A{"$classification_policy_hash", v.ClassificationPolicyHash}}, bson.M{"$eq": bson.A{"$classification_input_hash", v.ClassificationInputHash}}, bson.M{"$not": bson.A{changed}}}}
	for _, k := range []string{"classification_strategy", "classification_policy_hash", "classification_input_hash", "classification_method", "classification_status", "model_version", "category"} {
		value := fields[k]
		if v.ClassificationMethod == "ai" {
			set[k] = bson.M{"$cond": bson.A{projectionSame, "$" + k, bson.M{"$literal": value}}}
		} else {
			set[k] = bson.M{"$literal": value}
		}
	}
	if v.ClassificationMethod == "ai" {
		set["categories"] = bson.M{"$cond": bson.A{projectionSame, "$categories", bson.M{"$literal": categories}}}
	} else {
		set["categories"] = bson.M{"$cond": bson.A{samePolicy, mergeMembership("categories", "category", categories), bson.M{"$literal": categories}}}
	}
	now := time.Now().UTC()
	set["classification_updated_at"] = bson.M{"$cond": bson.A{projectionSame, bson.M{"$ifNull": bson.A{"$classification_updated_at", now}}, now}}
	set["original_category"] = bson.M{"$literal": v.OriginalCategory}
	set["original_categories"] = mergeMembership("original_categories", "original_category", originals)
	return mongo.Pipeline{bson.D{{Key: "$set", Value: set}}}, nil
}
func mergeMembership(array, scalar string, incoming []string) bson.M {
	legacy := bson.M{"$cond": bson.A{bson.M{"$ne": bson.A{bson.M{"$ifNull": bson.A{"$" + scalar, ""}}, ""}}, bson.A{"$" + scalar}, bson.A{}}}
	return bson.M{"$setUnion": bson.A{bson.M{"$ifNull": bson.A{"$" + array, legacy}}, bson.M{"$literal": incoming}}}
}
func query(f model.Filter, date string) bson.M {
	q := bson.M{"original_category": bson.M{"$nin": bson.A{"채용", "교원채용"}}, "original_categories": bson.M{"$nin": bson.A{"채용", "교원채용"}}}
	if date == "crawled_at" {
		expired := bson.M{"lifecycle_status": bson.M{"$in": bson.A{"active", "expired"}}, "lifecycle.confirmed": true, "lifecycle.version": lifecycle.Version, "deadline": bson.M{"$ne": nil, "$lt": time.Now().UTC()}, "$expr": bson.M{"$eq": bson.A{"$lifecycle.content_hash", "$content_hash"}}}
		if f.LifecycleStatus == "expired" {
			q["$and"] = bson.A{expired}
		} else {
			if !f.IncludeExpired {
				q["$nor"] = bson.A{expired}
			}
			if f.LifecycleStatus == "active" {
				q["lifecycle_status"] = "active"
				q["lifecycle.confirmed"] = true
				q["deadline"] = bson.M{"$gte": time.Now().UTC()}
			}
			if f.LifecycleStatus == "no_deadline" {
				q["lifecycle_status"] = "no_deadline"
			}
			if f.LifecycleStatus == "needs_review" {
				q["$and"] = bson.A{bson.M{"$or": bson.A{bson.M{"lifecycle_status": "needs_review"}, bson.M{"lifecycle_status": bson.M{"$exists": false}}, bson.M{"lifecycle_status": ""}}}}
			}
		}
	}
	if f.SourceID != nil {
		q["source_id"] = *f.SourceID
	}
	if f.Category != "" {
		q["$or"] = bson.A{bson.M{"category": f.Category}, bson.M{"categories": f.Category}}
	}
	if f.SourceName != "" {
		q["source_name"] = f.SourceName
	}
	d := bson.M{}
	if f.From != nil {
		d["$gte"] = *f.From
	}
	if f.To != nil {
		d["$lte"] = *f.To
	}
	if len(d) > 0 {
		q[date] = d
	}
	return q
}
func (s *ContentRepository) Contents(ctx context.Context, f model.Filter) ([]model.Content, error) {
	filter, err := s.ListVisibilityQuery(ctx, f)
	if err != nil {
		return nil, err
	}
	cur, err := s.contents.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "crawled_at", Value: -1}, {Key: "_id", Value: -1}}).SetSkip(int64((f.Page-1)*f.Limit)).SetLimit(int64(f.Limit)))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := cur.Close(ctx); err != nil {
			slog.Warn("cursor close failed", "error", err)
		}
	}()
	out := []model.Content{}
	err = cur.All(ctx, &out)
	for i := range out {
		out[i].LifecycleStatus = lifecycle.EffectiveStatus(out[i], time.Now())
		if out[i].LifecycleStatus == "" {
			out[i].LifecycleStatus = "needs_review"
		}
	}
	return out, err
}
func (s *ContentRepository) Content(ctx context.Context, id bson.ObjectID) (model.Content, error) {
	var v model.Content
	filter := query(model.Filter{IncludeExpired: true, IncludeCompleted: true}, "crawled_at")
	filter["_id"] = id
	err := s.contents.FindOne(ctx, filter).Decode(&v)
	if err == nil {
		v.LifecycleStatus = lifecycle.EffectiveStatus(v, time.Now())
		if v.LifecycleStatus == "" {
			v.LifecycleStatus = "needs_review"
		}
	}
	return v, err
}
