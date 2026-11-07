package model

import "time"

// Optional derived fields: older documents/API responses remain unchanged.
// No field is automatically projected into deadline, category or lifecycle.
type ScheduleMetadata struct {
	ScheduleValidationVersion string `json:"schedule_validation_version,omitempty" bson:"schedule_validation_version,omitempty"`
	ScheduleQualityState      string `json:"schedule_quality_state,omitempty" bson:"schedule_quality_state,omitempty"`
	SummaryQualityState       string `json:"summary_quality_state,omitempty" bson:"summary_quality_state,omitempty"`
	AudienceQualityState      string `json:"audience_quality_state,omitempty" bson:"audience_quality_state,omitempty"`

	Summary                      string     `json:"summary,omitempty" bson:"summary,omitempty"`
	TargetAudience               *string    `json:"target_audience,omitempty" bson:"target_audience,omitempty"`
	TargetAudienceReviewRequired bool       `json:"target_audience_review_required,omitempty" bson:"target_audience_review_required,omitempty"`
	TargetAudienceReviewReasons  []string   `json:"target_audience_review_reasons,omitempty" bson:"target_audience_review_reasons,omitempty"`
	SummaryReviewRequired        bool       `json:"summary_review_required,omitempty" bson:"summary_review_required,omitempty"`
	ScheduleExtractionVersion    string     `json:"schedule_extraction_version,omitempty" bson:"schedule_extraction_version,omitempty"`
	ScheduleExtractionMethod     string     `json:"schedule_extraction_method,omitempty" bson:"schedule_extraction_method,omitempty"`
	ScheduleReviewRequired       bool       `json:"schedule_review_required,omitempty" bson:"schedule_review_required,omitempty"`
	ScheduleAnalyzedAt           *time.Time `json:"schedule_analyzed_at,omitempty" bson:"schedule_analyzed_at,omitempty"`
	ScheduleInputHash            string     `json:"schedule_input_hash,omitempty" bson:"schedule_input_hash,omitempty"`
	ScheduleModelVersion         string     `json:"schedule_model_version,omitempty" bson:"schedule_model_version,omitempty"`
	ScheduleResultHash           string     `json:"schedule_result_hash,omitempty" bson:"schedule_result_hash,omitempty"`
	ScheduleAnalysisID           string     `json:"schedule_analysis_id,omitempty" bson:"schedule_analysis_id,omitempty"`
}
