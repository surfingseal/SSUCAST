package imageanalysis

import (
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/noticepolicy"
	"example.com/crawlserver/internal/scheduleapply"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"time"
)

type Prepared struct {
	Analysis           scheduleapply.Analysis `json:"analysis" bson:",inline"`
	Mode               string                 `json:"analysis_mode" bson:"analysis_mode"`
	ImageInputHash     string                 `json:"image_input_hash" bson:"image_input_hash"`
	Images             []ImageInput           `json:"image_inputs" bson:"image_inputs"`
	PreviousAnalysisID string                 `json:"previous_analysis_id" bson:"previous_analysis_id"`
	Merge              MergeResult            `json:"merge" bson:"merge"`
	VisualReview       *VisualReview          `json:"visual_review,omitempty" bson:"visual_review,omitempty"`
	Original           model.Content          `json:"original" bson:"-"`
}

func Prepare(r Result) (Prepared, error) {
	var p Prepared
	if r.Error != "" || r.Model != Model {
		return p, fmt.Errorf("successful fixed-model response required")
	}
	expected := InputHashFor(Candidate{Original: r.Original, Images: r.Images}, r.Format)
	if r.Format == ReviewVersion {
		expected = reviewHash(Candidate{Original: r.Original, Images: r.Images}, r.Review)
	}
	if r.InputHash != expected {
		return p, fmt.Errorf("image/text input hash mismatch")
	}
	out, e := resultOutput(r)
	if e != nil {
		return p, e
	}
	ss, issues := resultSchedules(r, out)
	merged := Merge(noticepolicy.Effective(r.Original), ss)
	projection := scheduleapply.Projection{ScheduleMetadata: r.Original.ScheduleMetadata, Schedules: merged.Schedules}
	projection.ScheduleValidationVersion = Version
	projection.ScheduleExtractionMethod = "text_plus_image"
	projection.ScheduleReviewRequired = r.Original.ScheduleReviewRequired || len(ss) > 0
	projection.ScheduleAnalyzedAt = func() *time.Time { t := r.At.UTC().Truncate(time.Millisecond); return &t }()
	projection.ScheduleQualityState = "uncertain"
	projection.ScheduleModelVersion = Model
	usage := bson.M{}
	b, _ := json.Marshal(r.Usage)
	json.Unmarshal(b, &usage)
	a := scheduleapply.Analysis{ResponseFormat: Version, ContentID: r.Original.ID, SourceID: r.Original.SourceID, SourceName: r.Original.SourceName, InputHash: scheduleapply.InputHash(r.Original), ContentHash: r.Original.ContentHash, ModelVersion: Model, ExtractionVersion: r.Original.ScheduleExtractionVersion, RawResponse: r.Raw, RawResponseHash: digest([]byte(r.Raw)), ValidatedOutput: projection, ValidationIssues: append(issues, merged.Issues...), TokenUsage: usage, AnalyzedAt: *projection.ScheduleAnalyzedAt}
	if r.Format == CompactVersion || r.Format == ReviewVersion {
		a.ResponseFormat = r.Format
		a.ValidatedOutput.ScheduleValidationVersion = r.Format
	}
	a.ValidatedOutput.ScheduleResultHash = scheduleapply.ProjectionHash(a.ValidatedOutput)
	a.ID = digest([]byte(r.InputHash + "\n" + a.RawResponseHash + "\n" + a.ValidatedOutput.ScheduleResultHash))
	a.ValidatedOutput.ScheduleAnalysisID = a.ID
	p = Prepared{VisualReview: r.Review, Analysis: a, Mode: "text_plus_image", ImageInputHash: r.InputHash, Images: r.Images, PreviousAnalysisID: r.Original.ScheduleAnalysisID, Merge: merged, Original: r.Original}
	return p, nil
}
func ValidatePrepared(p Prepared) error {
	format := ""
	if p.Analysis.ResponseFormat == CompactVersion || p.Analysis.ResponseFormat == ReviewVersion {
		format = p.Analysis.ResponseFormat
	}
	r := Result{Review: p.VisualReview, Format: format, Original: p.Original, Images: p.Images, InputHash: p.ImageInputHash, Raw: p.Analysis.RawResponse, Model: p.Analysis.ModelVersion, At: p.Analysis.AnalyzedAt}
	// Usage does not affect schedule semantics but belongs to immutable history.
	raw, _ := json.Marshal(p.Analysis.TokenUsage)
	json.Unmarshal(raw, &r.Usage)
	wanted, e := Prepare(r)
	if e != nil {
		return e
	}
	a, _ := json.Marshal(wanted)
	b, _ := json.Marshal(p)
	if string(a) != string(b) {
		return fmt.Errorf("prepared multimodal result was modified")
	}
	return nil
}

// ReviewedResult reuses the original paid raw response; no model invocation.
func ReviewedResult(original Result, current model.Content, review VisualReview, at time.Time) Result {
	r := Result{ID: current.ID.Hex(), Format: ReviewVersion, Review: &review, Original: current, Images: original.Images, Raw: original.Raw, Model: original.Model, At: at}
	r.InputHash = reviewHash(Candidate{Original: current, Images: r.Images}, r.Review)
	return r
}
