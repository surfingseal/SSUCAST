package model

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"time"
)

type ClassificationLabels struct {
	Evidence             []ClassificationEvidence `json:"evidence" bson:"evidence,omitempty"`
	PrimaryCategory      string                   `json:"primary_category" bson:"primary_category"`
	Categories           []string                 `json:"categories" bson:"categories"`
	ActivityTypes        []string                 `json:"activity_types" bson:"activity_types"`
	Topics               []string                 `json:"topics" bson:"topics"`
	TargetAudience       []string                 `json:"target_audience" bson:"target_audience"`
	ClassificationStatus string                   `json:"classification_status" bson:"classification_status"`
	ReasoningSummary     string                   `json:"reasoning_summary" bson:"reasoning_summary"`
}
type Classification struct {
	ClassificationMethod     string `json:"classification_method" bson:"classification_method"`
	ClassificationPolicyHash string `json:"classification_policy_hash" bson:"classification_policy_hash"`
	ContentHash              string `json:"content_hash" bson:"content_hash"`
	Applied                  bool   `json:"applied" bson:"applied"`
	ApplicationStatus        string `json:"application_status" bson:"application_status"`
	Reused                   bool   `json:"reused,omitempty" bson:"-"`
	ClassificationLabels     `bson:",inline"`
	RawLabels                *ClassificationLabels  `json:"raw_labels,omitempty" bson:"raw_labels,omitempty"`
	Quality                  *ClassificationQuality `json:"quality,omitempty" bson:"quality,omitempty"`
	ModelRequests            int                    `json:"model_requests" bson:"model_requests"`
	UsageKnown               bool                   `json:"usage_known" bson:"usage_known"`
	ID                       bson.ObjectID          `json:"id" bson:"_id,omitempty"`
	ContentID                bson.ObjectID          `json:"content_id" bson:"content_id"`
	InputHash                string                 `json:"input_hash" bson:"input_hash"`
	TaxonomyVersion          string                 `json:"taxonomy_version" bson:"taxonomy_version"`
	ModelVersion             string                 `json:"model_version" bson:"model_version"`
	ResolvedModelVersion     string                 `json:"resolved_model_version,omitempty" bson:"resolved_model_version,omitempty"`
	ClassifiedAt             *time.Time             `json:"classified_at" bson:"classified_at"`
	InputTokens              int32                  `json:"input_tokens" bson:"input_tokens"`
	OutputTokens             int32                  `json:"output_tokens" bson:"output_tokens"`
	ErrorCode                string                 `json:"error_code,omitempty" bson:"error_code,omitempty"`
}

// Evidence quotes must come from the text actually supplied to the model.
type ClassificationEvidence struct {
	Field  string `json:"field" bson:"field"`
	Label  string `json:"label" bson:"label"`
	Source string `json:"source" bson:"source"`
	Quote  string `json:"quote" bson:"quote"`
}
type RejectedClaim struct {
	Field  string `json:"field" bson:"field"`
	Label  string `json:"label" bson:"label"`
	Reason string `json:"reason" bson:"reason"`
}
type ClassificationQuality struct {
	ImagesState                string          `json:"images_state" bson:"images_state"`
	Version                    string          `json:"version" bson:"version"`
	ReviewReasons              []string        `json:"review_reasons" bson:"review_reasons"`
	RejectedClaims             []RejectedClaim `json:"rejected_claims" bson:"rejected_claims"`
	ImagesUnanalysed           bool            `json:"images_unanalysed" bson:"images_unanalysed"`
	AttachmentsUnanalysed      bool            `json:"attachments_unanalysed" bson:"attachments_unanalysed"`
	ShortText                  bool            `json:"short_text" bson:"short_text"`
	SourceCategoryDisagreement bool            `json:"source_category_disagreement" bson:"source_category_disagreement"`
}
