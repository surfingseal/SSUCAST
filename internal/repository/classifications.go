package repository

import (
	"context"
	"fmt"
	"time"

	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type ClassificationRepository struct{ classifications *mongo.Collection }

// Reserving before generation prevents concurrent/duplicate paid calls. Failed
// and interrupted reservations are retained; this MVP never retries them.
func (s *ClassificationRepository) ReserveClassification(ctx context.Context, v model.Classification) (bool, error) {
	if v.ContentID.IsZero() || v.InputHash == "" || v.TaxonomyVersion == "" || v.ModelVersion == "" {
		return false, fmt.Errorf("invalid classification identity")
	}
	if v.ID.IsZero() {
		v.ID = bson.NewObjectID()
	}
	v.ClassificationStatus = "processing"
	_, err := s.classifications.InsertOne(ctx, v)
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	return err == nil, err
}
func classificationKey(v model.Classification) bson.M {
	return bson.M{"content_id": v.ContentID, "input_hash": v.InputHash, "taxonomy_version": v.TaxonomyVersion, "model_version": v.ModelVersion}
}
func (s *ClassificationRepository) CompleteClassification(ctx context.Context, v model.Classification) error {
	if err := classification.Validate(v.ClassificationLabels); err != nil {
		return err
	}
	if v.ClassifiedAt == nil {
		return fmt.Errorf("classified_at required")
	}
	key := classificationKey(v)
	key["classification_status"] = "processing"
	raw, err := bson.Marshal(v)
	if err != nil {
		return err
	}
	var fields bson.M
	if err = bson.Unmarshal(raw, &fields); err != nil {
		return err
	}
	delete(fields, "_id")
	r, err := s.classifications.UpdateOne(ctx, key, bson.M{"$set": fields})
	if err == nil && r.MatchedCount != 1 {
		return fmt.Errorf("classification reservation missing or already finalized")
	}
	return err
}
func (s *ClassificationRepository) FailClassification(ctx context.Context, v model.Classification, code string, telemetry ...model.Classification) error {
	key := classificationKey(v)
	key["classification_status"] = "processing"
	fields := bson.M{"classification_status": "failed", "error_code": code, "classified_at": nil}
	if len(telemetry) > 0 {
		v := telemetry[0]
		fields["model_requests"] = v.ModelRequests
		fields["input_tokens"] = v.InputTokens
		fields["output_tokens"] = v.OutputTokens
		fields["usage_known"] = v.UsageKnown
	}
	_, err := s.classifications.UpdateOne(ctx, key, bson.M{"$set": fields})
	return err
}
func (s *ClassificationRepository) Classifications(ctx context.Context, id bson.ObjectID) ([]model.Classification, error) {
	cur, err := s.classifications.Find(ctx, bson.M{"content_id": id}, options.Find().SetSort(bson.D{{Key: "classified_at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(100))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []model.Classification{}
	err = cur.All(ctx, &out)
	return out, err
}
func (s *Store) ClassifyOne(ctx context.Context, item model.Content, classifier classification.Classifier, modelID string) (model.Classification, error) {
	current, err := s.ContentRepository.Content(ctx, item.ID)
	if err != nil {
		return model.Classification{}, err
	}
	item = current
	source, decision, err := s.CurrentDecision(ctx, item)
	if err != nil {
		return model.Classification{}, err
	}
	if !decision.NeedsAI {
		if err := s.Save(ctx, []model.Content{item}); err != nil {
			return model.Classification{}, err
		}
		return model.Classification{ContentID: item.ID, ClassificationMethod: "source", Applied: true, ApplicationStatus: "applied", ClassificationLabels: model.ClassificationLabels{PrimaryCategory: decision.Category, Categories: []string{decision.Category}, ActivityTypes: []string{}, Topics: []string{}, TargetAudience: []string{}, ClassificationStatus: decision.Status, ReasoningSummary: "공식 원본 카테고리 정규화; AI 호출 없음"}}, nil
	}
	if classifier == nil {
		return model.Classification{}, ErrAIClassifierRequired
	}
	// Refresh pending state before reserving. This also invalidates stale results
	// after a body or policy change, while preserving an unchanged AI projection.
	if err := s.Save(ctx, []model.Content{item}); err != nil {
		return model.Classification{}, err
	}
	item, err = s.ContentRepository.Content(ctx, item.ID)
	if err != nil {
		return model.Classification{}, err
	}
	record := model.Classification{ID: bson.NewObjectID(), ContentID: item.ID, InputHash: classification.PolicyInputHash(source, item), TaxonomyVersion: classification.TaxonomyVersion, ModelVersion: modelID, ClassificationMethod: "ai", ClassificationPolicyHash: source.ClassificationPolicyHash(), ContentHash: item.ContentHash, ApplicationStatus: "pending"}
	reserved, err := s.ReserveClassification(ctx, record)
	if err != nil {
		return record, err
	}
	if !reserved {
		var cached model.Classification
		if err := s.classifications.FindOne(ctx, classificationKey(record)).Decode(&cached); err != nil {
			return record, err
		}
		if cached.ClassificationStatus == "classified" || cached.ClassificationStatus == "needs_review" {
			cached.Reused = true
			if err := s.applyAIProjection(ctx, source, item, &cached); err != nil {
				return cached, err
			}
			return cached, nil
		}
		return record, fmt.Errorf("classification already reserved or failed; no generation attempted")
	}
	latestSource, checkErr := s.contentSource(ctx, item.SourceID)
	latestItem, inputErr := s.ContentRepository.Content(ctx, item.ID)
	if checkErr != nil || inputErr != nil || latestSource.ClassificationPolicyHash() != source.ClassificationPolicyHash() || latestItem.ContentHash != item.ContentHash || latestItem.ClassificationInputHash != record.InputHash {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if markErr := s.FailClassification(cleanup, record, "policy_or_input_changed_before_call"); markErr != nil {
			return record, markErr
		}
		return record, fmt.Errorf("policy or input changed before model call; no request attempted")
	}
	result, err := classifier.Classify(ctx, item)
	record.ModelRequests = result.ModelRequests
	record.InputTokens = result.InputTokens
	record.OutputTokens = result.OutputTokens
	record.UsageKnown = result.UsageKnown
	if err == nil {
		err = classification.Validate(result.Labels)
	}
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if markErr := s.FailClassification(cleanup, record, classification.ErrorCode(err), record); markErr != nil {
			return record, fmt.Errorf("classification failed (%s); failure persistence failed: %w", classification.ErrorCode(err), markErr)
		}
		return record, err
	}
	now := time.Now().UTC()
	raw := result.Labels
	record.RawLabels = &raw
	cleaned, quality := classification.ApplyQuality(item, raw)
	if cleaned.ClassificationStatus == "needs_review" {
		cleaned.PrimaryCategory = "unknown"
		cleaned.Categories = []string{"unknown"}
	}
	record.ClassificationLabels = cleaned
	record.Quality = &quality
	record.ClassifiedAt = &now
	record.ResolvedModelVersion = result.ResolvedModel
	record.InputTokens = result.InputTokens
	record.OutputTokens = result.OutputTokens
	if err = s.CompleteClassification(ctx, record); err != nil {
		return record, err
	}
	if err = s.applyAIProjection(ctx, source, item, &record); err != nil {
		return record, err
	}
	return record, nil
}
