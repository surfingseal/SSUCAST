package repository

import (
	"context"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/model"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"reflect"
)

// ApplyPreparedClassification reuses one already-paid combined response. It
// never invokes a model or changes source policy, and retains the usual CAS.
func (s *Store) ApplyPreparedClassification(ctx context.Context, source model.Source, item model.Content, record model.Classification) error {
	if record.ContentID != item.ID || record.ContentHash != item.ContentHash || record.InputHash != classification.PolicyInputHash(source, item) || record.ClassificationPolicyHash != source.ClassificationPolicyHash() || record.ClassifiedAt == nil {
		return fmt.Errorf("prepared classification identity mismatch")
	}
	if e := classification.Validate(record.ClassificationLabels); e != nil {
		return e
	}
	latest, e := s.ContentRepository.Content(ctx, item.ID)
	if e != nil {
		return e
	}
	if latest.ContentHash != item.ContentHash || latest.ClassificationInputHash != record.InputHash {
		return fmt.Errorf("classification input CAS conflict")
	}
	reserved, e := s.ReserveClassification(ctx, record)
	if e != nil {
		return e
	}
	if reserved {
		if e = s.CompleteClassification(ctx, record); e != nil {
			return e
		}
	} else {
		var old model.Classification
		if e = s.classifications.FindOne(ctx, classificationKey(record)).Decode(&old); e != nil {
			return e
		}
		if !reflect.DeepEqual(old.ClassificationLabels, record.ClassificationLabels) || old.ContentHash != record.ContentHash || old.ClassifiedAt == nil {
			return fmt.Errorf("immutable classification history conflict")
		}
		if old.Applied && latest.Category == old.PrimaryCategory && latest.ModelVersion == old.ModelVersion && latest.ClassificationStatus == old.ClassificationStatus {
			return nil
		}
		record = old
	}
	if e = s.applyAIProjection(ctx, source, latest, &record); e != nil {
		return e
	}
	if !record.Applied {
		return fmt.Errorf("classification policy CAS conflict")
	}
	_, e = s.contents.UpdateOne(ctx, bson.M{"_id": item.ID, "content_hash": item.ContentHash, "classification_input_hash": record.InputHash, "classification_policy_hash": source.ClassificationPolicyHash(), "model_version": record.ModelVersion}, bson.M{"$set": bson.M{"category_source": "ai"}})
	return e
}
