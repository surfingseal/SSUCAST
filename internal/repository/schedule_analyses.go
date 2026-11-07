package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/scheduleapply"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"regexp"
	"strconv"
)

// ScheduleAnalyses is explicitly constructed by a manual command/test. Opening
// it does not create collections/indexes and does not touch classifications.
type ScheduleAnalyses struct{ contents, history *mongo.Collection }

func NewScheduleAnalyses(db *mongo.Database) *ScheduleAnalyses {
	return &ScheduleAnalyses{db.Collection("crawled_contents"), db.Collection("schedule_analyses")}
}

type SchedulePlan struct {
	ID                bson.ObjectID `json:"id"`
	Title             string        `json:"title"`
	Outcome           string        `json:"outcome"`
	ReviewRequired    bool          `json:"review_required"`
	Schedules         int           `json:"schedules"`
	OriginalSchedules int           `json:"original_schedules"`
	RuleSessions      int           `json:"rule_sessions"`
	Snapshot          bson.Raw      `json:"-"`
}

func (s *ScheduleAnalyses) Preview(ctx context.Context, item scheduleapply.Item) (SchedulePlan, error) {
	if e := scheduleapply.ValidateItem(item); e != nil {
		return SchedulePlan{}, e
	}
	var raw bson.Raw
	e := s.contents.FindOne(ctx, bson.M{"_id": item.Original.ID}).Decode(&raw)
	if errors.Is(e, mongo.ErrNoDocuments) {
		return SchedulePlan{ID: item.Original.ID, Title: item.Original.Title, Outcome: "not_found"}, nil
	}
	if e != nil {
		return SchedulePlan{}, fmt.Errorf("schedule preview read failed")
	}
	return planSchedule(raw, item)
}

var versionNumber = regexp.MustCompile(`^schedule-v([0-9]+)$`)

func rank(v string) int {
	m := versionNumber.FindStringSubmatch(v)
	if len(m) != 2 {
		return -1
	}
	n, _ := strconv.Atoi(m[1])
	return n
}
func projection(v model.Content) scheduleapply.Projection {
	return scheduleapply.Projection{ScheduleMetadata: v.ScheduleMetadata, Schedules: v.Schedules}
}
func planSchedule(raw bson.Raw, item scheduleapply.Item) (SchedulePlan, error) {
	var current model.Content
	if e := bson.Unmarshal(raw, &current); e != nil {
		return SchedulePlan{}, fmt.Errorf("invalid stored content")
	}
	wanted := item.Analysis.ValidatedOutput
	p := SchedulePlan{ID: current.ID, Title: current.Title, ReviewRequired: wanted.ScheduleReviewRequired, Schedules: len(wanted.Schedules), Snapshot: append(bson.Raw{}, raw...)}
	for _, v := range wanted.Schedules {
		if v.Origin == "saved_ai_response" {
			p.OriginalSchedules++
		} else if v.Origin == "source_rule_supplement" {
			p.RuleSessions++
		}
	}
	switch {
	case current.ID != item.Analysis.ContentID || current.SourceName != item.Analysis.SourceName || current.SourceID != item.Analysis.SourceID:
		p.Outcome = "source_conflict"
	case current.ContentHash != item.Analysis.ContentHash:
		p.Outcome = "content_hash_conflict"
	case scheduleapply.InputHash(current) != item.Analysis.InputHash:
		p.Outcome = "input_hash_conflict"
	case current.ScheduleExtractionVersion != "" && (rank(current.ScheduleExtractionVersion) < 0 || rank(current.ScheduleExtractionVersion) > rank(item.Analysis.ExtractionVersion)):
		p.Outcome = "analysis_version_conflict"
	case current.ScheduleAnalyzedAt != nil && current.ScheduleAnalyzedAt.After(item.Analysis.AnalyzedAt):
		p.Outcome = "analysis_version_conflict"
	case current.ScheduleAnalysisID == item.Analysis.ID && current.ScheduleInputHash == item.Analysis.InputHash && current.ScheduleResultHash == wanted.ScheduleResultHash && scheduleapply.ProjectionHash(projection(current)) == wanted.ScheduleResultHash:
		p.Outcome = "duplicate"
	case current.ScheduleAnalyzedAt != nil && !current.ScheduleAnalyzedAt.Before(item.Analysis.AnalyzedAt):
		p.Outcome = "analysis_version_conflict"
	case current.ScheduleInputHash != "" && current.ScheduleInputHash != item.Analysis.InputHash:
		p.Outcome = "analysis_state_conflict"
	case current.ScheduleExtractionVersion == "" && (len(current.Schedules) > 0 || current.Summary != "" || current.ScheduleInputHash != "" || current.ScheduleAnalysisID != ""):
		p.Outcome = "analysis_state_conflict"
	default:
		p.Outcome = "eligible"
	}
	return p, nil
}

// scheduleCAS matches identity, immutable input and the exact observed document.
// Full BSON equality also guards analysis version/schedules/time and the precise
// absent/null/value state of schedule_input_hash, not only its string value.
func scheduleCAS(snapshot bson.Raw, item scheduleapply.Item) (bson.D, error) {
	var doc bson.D
	if bson.Unmarshal(snapshot, &doc) != nil {
		return nil, fmt.Errorf("invalid CAS snapshot")
	}
	filter := bson.D{{Key: "_id", Value: item.Analysis.ContentID}, {Key: "source_name", Value: item.Analysis.SourceName}, {Key: "source_id", Value: item.Analysis.SourceID}, {Key: "content_hash", Value: item.Analysis.ContentHash}}
	if value, e := snapshot.LookupErr("schedule_input_hash"); e == nil {
		filter = append(filter, bson.E{Key: "schedule_input_hash", Value: bson.M{"$exists": true, "$eq": value}})
	} else {
		filter = append(filter, bson.E{Key: "schedule_input_hash", Value: bson.M{"$exists": false}})
	}
	filter = append(filter, bson.E{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$$ROOT", bson.D{{Key: "$literal", Value: doc}}}}}})
	return filter, nil
}

// Explicit allowlist. No replacement, array push, upsert, updated_at, content,
// category, hash, source, deadline, lifecycle or classification mutation.
func scheduleSet(p scheduleapply.Projection) bson.M {
	fields := bson.M{"summary": p.Summary, "target_audience": p.TargetAudience, "target_audience_review_required": p.TargetAudienceReviewRequired, "target_audience_review_reasons": p.TargetAudienceReviewReasons, "summary_review_required": p.SummaryReviewRequired, "schedules": p.Schedules, "schedule_extraction_version": p.ScheduleExtractionVersion, "schedule_extraction_method": p.ScheduleExtractionMethod, "schedule_review_required": p.ScheduleReviewRequired, "schedule_analyzed_at": p.ScheduleAnalyzedAt, "schedule_input_hash": p.ScheduleInputHash, "schedule_model_version": p.ScheduleModelVersion, "schedule_result_hash": p.ScheduleResultHash, "schedule_analysis_id": p.ScheduleAnalysisID}
	if p.ScheduleValidationVersion != "" {
		fields["schedule_validation_version"] = p.ScheduleValidationVersion
		fields["schedule_quality_state"] = p.ScheduleQualityState
		fields["summary_quality_state"] = p.SummaryQualityState
		fields["audience_quality_state"] = p.AudienceQualityState
	}
	return fields
}

type ScheduleApplyResult struct {
	ID              bson.ObjectID `json:"id"`
	Outcome         string        `json:"outcome"`
	Modified        int64         `json:"modified"`
	HistoryInserted bool          `json:"history_inserted"`
}

// Apply stores an immutable analysis first, then atomically CAS-updates only the
// projection. History existence NEVER means content was updated. A crash/CAS
// failure can leave an unapplied history; it cannot leave an applied projection
// without its raw analysis. This works on standalone MongoDB and Atlas without
// transactions, privilege changes or new indexes. _id enforces history identity.
func (s *ScheduleAnalyses) Apply(ctx context.Context, item scheduleapply.Item, observed SchedulePlan) (ScheduleApplyResult, error) {
	result := ScheduleApplyResult{ID: item.Original.ID}
	if e := scheduleapply.ValidateItem(item); e != nil {
		return result, e
	}
	if observed.ID != item.Original.ID || len(observed.Snapshot) == 0 {
		return result, fmt.Errorf("observed snapshot required")
	}
	verified, e := planSchedule(observed.Snapshot, item)
	if e != nil {
		return result, e
	}
	if verified.Outcome != "eligible" {
		result.Outcome = verified.Outcome
		if result.Outcome == "duplicate" {
			latest, e := s.Preview(ctx, item)
			if e != nil {
				return result, e
			}
			result.Outcome = latest.Outcome
			if result.Outcome == "eligible" {
				result.Outcome = "cas_conflict"
			}
		}
		return result, nil
	}
	// Insert immutable raw and validated output. Never replace existing history.
	_, e = s.history.InsertOne(ctx, item.Analysis)
	if mongo.IsDuplicateKeyError(e) {
		var existing scheduleapply.Analysis
		if err := s.history.FindOne(ctx, bson.M{"_id": item.Analysis.ID}).Decode(&existing); err != nil {
			return result, fmt.Errorf("history verification failed")
		}
		if canonical(existing) != canonical(item.Analysis) {
			return result, fmt.Errorf("immutable history conflict")
		}
	} else if e != nil {
		return result, fmt.Errorf("analysis history insert failed")
	} else {
		result.HistoryInserted = true
	}
	filter, e := scheduleCAS(observed.Snapshot, item)
	if e != nil {
		return result, e
	}
	update, e := s.contents.UpdateOne(ctx, filter, bson.M{"$set": scheduleSet(item.Analysis.ValidatedOutput)})
	if e != nil {
		return result, fmt.Errorf("schedule CAS update failed; history may exist without application")
	}
	result.Modified = update.ModifiedCount
	if update.MatchedCount == 1 && update.ModifiedCount == 1 {
		result.Outcome = "updated"
		return result, nil
	}
	current, e := s.Preview(ctx, item)
	if e != nil {
		return result, e
	}
	result.Outcome = current.Outcome
	if result.Outcome == "eligible" {
		result.Outcome = "cas_conflict"
	}
	return result, nil
}
func canonical(v any) string {
	raw, e := bson.Marshal(v)
	if e != nil {
		return "invalid BSON"
	}
	decoder := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(raw)))
	decoder.DefaultDocumentM()
	var doc bson.M
	if decoder.Decode(&doc) != nil {
		return "invalid BSON"
	}
	out, _ := json.Marshal(doc)
	return string(out)
}

// VerifyScheduleHistory is a read-only provenance check, not a summary-presence
// heuristic. Projection duplicate status is checked separately by Preview.
func (s *ScheduleAnalyses) VerifyScheduleHistory(ctx context.Context, item scheduleapply.Item) error {
	if e := scheduleapply.ValidateItem(item); e != nil {
		return e
	}
	var stored scheduleapply.Analysis
	if e := s.history.FindOne(ctx, bson.M{"_id": item.Analysis.ID}).Decode(&stored); e != nil {
		return fmt.Errorf("linked schedule history missing")
	}
	if canonical(stored) != canonical(item.Analysis) {
		return fmt.Errorf("linked schedule history content mismatch")
	}
	return nil
}
