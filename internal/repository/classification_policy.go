package repository

import (
	"context"
	"fmt"
	"time"

	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (s *Store) contentSource(ctx context.Context, id bson.ObjectID) (model.Source, error) {
	source, err := s.Source(ctx, id)
	if err == mongo.ErrNoDocuments {
		return model.Source{ID: id, ClassificationStrategy: "source", ClassificationRevision: 1}, nil
	}
	return source, err
}

// Save keeps the JobStore interface. Source policy is resolved from persisted
// configuration rather than trusting crawler-supplied AI/category metadata.
func (s *Store) Save(ctx context.Context, items []model.Content) error {
	prepared := make([]model.Content, 0, len(items))
	for _, v := range items {
		if !model.IsExcludedNotice(v) {
			prepared = append(prepared, v)
		}
	}
	for i := range prepared {
		v := &prepared[i]
		source, err := s.contentSource(ctx, v.SourceID)
		if err != nil {
			return err
		}
		if source.Name != "" {
			v.SourceName = source.Name
		}
		var normErr error
		v.OriginalURL, normErr = model.NormalizeContentURL(v.OriginalURL)
		if normErr != nil {
			return normErr
		}
		// Compute classification input before resetting a previous AI projection.
		v.ContentHash = model.ContentDigest(*v)
		inputHash := classification.PolicyInputHash(source, *v)
		if err := model.PrepareClassification(source, v); err != nil {
			return err
		}
		v.ClassificationInputHash = inputHash
	}
	if err := s.ContentRepository.Save(ctx, prepared); err != nil {
		return err
	}
	for _, v := range prepared {
		var saved model.Content
		if err := s.contents.FindOne(ctx, bson.M{"original_url": v.OriginalURL}).Decode(&saved); err != nil {
			return err
		}
		// Keep the prediction itself immutable; only mark whether it still applies.
		filter := bson.M{"content_id": saved.ID, "applied": true, "input_hash": bson.M{"$ne": saved.ClassificationInputHash}}
		if _, err := s.classifications.UpdateMany(ctx, filter, bson.M{"$set": bson.M{"applied": false, "application_status": "superseded"}}); err != nil {
			return err
		}
	}
	return nil
}

// ApplySources invalidates/reprojects existing notices after strategy changes.
// It never classifies with AI, even if classification_strategy is ai.
func (s *Store) ApplySources(ctx context.Context, sources []model.Source) error {
	if err := s.SourceRepository.ApplySources(ctx, sources); err != nil {
		return err
	}
	for _, cfg := range sources {
		var source model.Source
		if err := s.sources.FindOne(ctx, bson.M{"name": cfg.Name}).Decode(&source); err != nil {
			return err
		}
		if err := s.ReconcileSource(ctx, source.ID); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) ReconcileSource(ctx context.Context, id bson.ObjectID) error {
	cur, err := s.contents.Find(ctx, bson.M{"source_id": id})
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var v model.Content
		if err := cur.Decode(&v); err != nil {
			return err
		}
		if model.IsExcludedNotice(v) {
			continue
		}
		if err := s.Save(ctx, []model.Content{v}); err != nil {
			return err
		}
	}
	return cur.Err()
}
func (s *Store) CurrentDecision(ctx context.Context, item model.Content) (model.Source, model.ClassificationDecision, error) {
	source, err := s.contentSource(ctx, item.SourceID)
	if err != nil {
		return source, model.ClassificationDecision{}, err
	}
	decision, err := model.DecideClassification(source, item)
	return source, decision, err
}
func (s *Store) applyAIProjection(ctx context.Context, source model.Source, item model.Content, record *model.Classification) error {
	latest, err := s.contentSource(ctx, item.SourceID)
	if err != nil {
		return err
	}
	applied := false
	if latest.ClassificationPolicyHash() == source.ClassificationPolicyHash() {
		filter := bson.M{"_id": item.ID, "content_hash": item.ContentHash, "classification_policy_hash": source.ClassificationPolicyHash(), "classification_input_hash": record.InputHash}
		now := time.Now().UTC()
		result, err := s.contents.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"category": record.PrimaryCategory, "categories": record.Categories, "classification_method": "ai", "classification_status": record.ClassificationStatus, "model_version": record.ModelVersion, "classification_updated_at": now}})
		if err != nil {
			return err
		}
		applied = result.MatchedCount == 1
	}
	record.Applied = applied
	record.ApplicationStatus = "superseded"
	if applied {
		record.ApplicationStatus = "applied"
	}
	if _, err := s.classifications.UpdateOne(ctx, classificationKey(*record), bson.M{"$set": bson.M{"applied": record.Applied, "application_status": record.ApplicationStatus}}); err != nil {
		return err
	}
	if !applied {
		return nil
	}
	// Cross-document policy updates are not a transaction on standalone MongoDB.
	// A post-check converges a policy change that raced the content CAS.
	latest, err = s.contentSource(ctx, item.SourceID)
	if err != nil {
		return err
	}
	if latest.ClassificationPolicyHash() != source.ClassificationPolicyHash() {
		current, err := s.ContentRepository.Content(ctx, item.ID)
		if err != nil {
			return err
		}
		if err = s.Save(ctx, []model.Content{current}); err != nil {
			return err
		}
		record.Applied = false
		record.ApplicationStatus = "superseded"
		_, err = s.classifications.UpdateOne(ctx, classificationKey(*record), bson.M{"$set": bson.M{"applied": false, "application_status": "superseded"}})
		return err
	}
	return nil
}

// ReconcileAll is an explicit local migration. AI history is never modified.
func (s *Store) ReconcileAll(ctx context.Context) (int, error) {
	cur, err := s.contents.Find(ctx, bson.M{})
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)
	n := 0
	for cur.Next(ctx) {
		var v model.Content
		if err := cur.Decode(&v); err != nil {
			return n, err
		}
		if model.IsExcludedNotice(v) {
			continue
		}
		if err := s.Save(ctx, []model.Content{v}); err != nil {
			return n, err
		}
		n++
	}
	return n, cur.Err()
}

var ErrAIClassifierRequired = fmt.Errorf("AI strategy requires an explicitly supplied classifier")
