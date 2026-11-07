// Package imageapply stores separately auditable multimodal results with exact
// snapshot CAS. The default command uses Preview; Apply is explicit/manual only.
package imageapply

import (
	"bytes"
	"context"
	"example.com/crawlserver/internal/imageanalysis"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/noticepolicy"
	"example.com/crawlserver/internal/scheduleapply"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"time"
)

type Plan struct {
	ID             bson.ObjectID `json:"id"`
	Title          string        `json:"title"`
	Outcome        string        `json:"outcome"`
	Schedules      int           `json:"schedules"`
	Added          int           `json:"added"`
	Conflicts      int           `json:"conflicts"`
	ReviewRequired bool          `json:"review_required"`
	Snapshot       bson.Raw      `json:"-"`
}

func Preview(ctx context.Context, db *mongo.Database, p imageanalysis.Prepared) (Plan, error) {
	plan := Plan{ID: p.Original.ID, Title: p.Original.Title, Schedules: len(p.Merge.Schedules), Added: p.Merge.Added, Conflicts: p.Merge.Conflicts, ReviewRequired: p.Analysis.ValidatedOutput.ScheduleReviewRequired}
	if e := imageanalysis.ValidatePrepared(p); e != nil {
		return plan, e
	}
	var raw bson.Raw
	e := db.Collection("crawled_contents").FindOne(ctx, bson.M{"_id": p.Original.ID}).Decode(&raw)
	if e == mongo.ErrNoDocuments {
		plan.Outcome = "not_found"
		return plan, nil
	}
	if e != nil {
		return plan, fmt.Errorf("preview read failed")
	}
	var v model.Content
	if bson.Unmarshal(raw, &v) != nil {
		return plan, fmt.Errorf("invalid content BSON")
	}
	plan.Snapshot = raw
	switch {
	case v.SourceID != p.Original.SourceID || v.SourceName != p.Original.SourceName:
		plan.Outcome = "source_conflict"
	case v.ContentHash != p.Analysis.ContentHash:
		plan.Outcome = "content_hash_conflict"
	case v.ScheduleInputHash != p.Analysis.InputHash || scheduleapply.InputHash(v) != p.Analysis.InputHash:
		plan.Outcome = "input_hash_conflict"
	case v.ScheduleAnalysisID == p.Analysis.ID && v.ImageAnalysisInputHash == p.ImageInputHash:
		got := scheduleapply.Projection{ScheduleMetadata: v.ScheduleMetadata, Schedules: v.Schedules}
		if scheduleapply.ProjectionHash(got) != p.Analysis.ValidatedOutput.ScheduleResultHash {
			return plan, fmt.Errorf("stored projection mismatch")
		}
		plan.Outcome = "duplicate"
	case v.ScheduleAnalysisID != p.PreviousAnalysisID:
		plan.Outcome = "analysis_state_conflict"
	case !sameProjection(v, p.Original) || !equal(v.NoticePolicy, p.Original.NoticePolicy):
		plan.Outcome = "analysis_state_conflict"
	case v.ScheduleAnalyzedAt != nil && !v.ScheduleAnalyzedAt.Before(p.Analysis.AnalyzedAt):
		plan.Outcome = "analysis_version_conflict"
	default:
		plan.Outcome = "eligible"
	}
	return plan, nil
}
func Apply(ctx context.Context, db *mongo.Database, p imageanalysis.Prepared, plan Plan) (string, error) {
	if e := imageanalysis.ValidatePrepared(p); e != nil {
		return "", e
	}
	if plan.Outcome == "duplicate" {
		return "duplicate", nil
	}
	if plan.Outcome != "eligible" || len(plan.Snapshot) == 0 {
		return "", fmt.Errorf("eligible snapshot required")
	}
	latest, e := Preview(ctx, db, p)
	if e != nil {
		return "", e
	}
	if latest.Outcome != "eligible" {
		return latest.Outcome, nil
	}
	if !bytes.Equal(latest.Snapshot, plan.Snapshot) {
		return "cas_conflict", nil
	}
	// Immutable multimodal history is separate from the original text-only record.
	_, e = db.Collection("schedule_analyses").InsertOne(ctx, p)
	if mongo.IsDuplicateKeyError(e) {
		var old imageanalysis.Prepared
		if db.Collection("schedule_analyses").FindOne(ctx, bson.M{"_id": p.Analysis.ID}).Decode(&old) != nil || old.ImageInputHash != p.ImageInputHash || old.Analysis.RawResponseHash != p.Analysis.RawResponseHash || old.Analysis.ValidatedOutput.ScheduleResultHash != p.Analysis.ValidatedOutput.ScheduleResultHash {
			return "", fmt.Errorf("immutable image history conflict")
		}
	} else if e != nil {
		return "", fmt.Errorf("image history insert failed")
	}
	raw, e := bson.Marshal(p.Analysis.ValidatedOutput)
	if e != nil {
		return "", e
	}
	fields := bson.M{}
	bson.Unmarshal(raw, &fields)
	fields["image_analysis_used"] = true
	fields["image_analysis_input_hash"] = p.ImageInputHash
	fields["image_analysis_content_hash"] = p.Analysis.ContentHash
	count, evidence := 0, 0
	for _, s := range p.Merge.Schedules {
		if len(s.ImageEvidence) > 0 {
			count++
			evidence += len(s.ImageEvidence)
		}
	}
	fields["image_schedule_count"] = count
	fields["image_evidence_count"] = evidence
	v := p.Original
	v.ScheduleMetadata = p.Analysis.ValidatedOutput.ScheduleMetadata
	v.Schedules = p.Merge.Schedules
	v.NoticePolicy = nil
	if p.Original.NoticePolicy != nil {
		policy, e := noticepolicy.Project(v, p.Original.NoticePolicy.Year, time.Now().UTC())
		if e != nil {
			return "", e
		}
		fields["notice_policy"] = policy
	}
	var observed bson.D
	if bson.Unmarshal(plan.Snapshot, &observed) != nil {
		return "", fmt.Errorf("invalid CAS snapshot")
	}
	filter := bson.M{"_id": p.Original.ID, "source_id": p.Original.SourceID, "content_hash": p.Analysis.ContentHash, "schedule_input_hash": p.Analysis.InputHash, "schedule_analysis_id": p.PreviousAnalysisID, "$expr": bson.M{"$eq": bson.A{"$$ROOT", bson.M{"$literal": observed}}}}
	r, e := db.Collection("crawled_contents").UpdateOne(ctx, filter, bson.M{"$set": fields})
	if e != nil {
		return "", fmt.Errorf("image projection update failed")
	}
	if r.MatchedCount != 1 {
		return "cas_conflict", nil
	}
	return "updated", nil
}

// Content JSON omits an empty schedules array. Treat only nil and empty as
// equivalent when checking the frozen text snapshot; historical hashes are not
// changed and Apply still compares the complete BSON snapshot atomically.
func sameProjection(a, b model.Content) bool {
	if len(a.Schedules) == 0 {
		a.Schedules = nil
	}
	if len(b.Schedules) == 0 {
		b.Schedules = nil
	}
	return scheduleapply.ProjectionHash(scheduleapply.Projection{ScheduleMetadata: a.ScheduleMetadata, Schedules: a.Schedules}) == scheduleapply.ProjectionHash(scheduleapply.Projection{ScheduleMetadata: b.ScheduleMetadata, Schedules: b.Schedules})
}
