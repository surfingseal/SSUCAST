// Package scheduleapply prepares immutable, validated schedule projections.
// It has no model client, database connection, automatic runner or HTTP client.
package scheduleapply

import (
	"crypto/sha256"
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/schedules"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"strings"
	"time"
	"unicode/utf8"
)

const BundleVersion = "schedule-apply-v1"

type Projection struct {
	model.ScheduleMetadata `bson:",inline"`
	Schedules              []model.Schedule `json:"schedules" bson:"schedules"`
}
type Analysis struct {
	ResponseFormat     string        `json:"response_format,omitempty" bson:"response_format,omitempty"`
	ID                 string        `json:"id" bson:"_id"`
	ContentID          bson.ObjectID `json:"content_id" bson:"content_id"`
	SourceID           bson.ObjectID `json:"source_id" bson:"source_id"`
	SourceName         string        `json:"source_name" bson:"source_name"`
	InputHash          string        `json:"input_hash" bson:"input_hash"`
	ContentHash        string        `json:"content_hash" bson:"content_hash"`
	ModelVersion       string        `json:"model_version" bson:"model_version"`
	ExtractionVersion  string        `json:"extraction_version" bson:"extraction_version"`
	RawResponse        string        `json:"raw_response" bson:"raw_response"`
	RawResponseHash    string        `json:"raw_response_hash" bson:"raw_response_hash"`
	ValidatedOutput    Projection    `json:"validated_output" bson:"validated_output"`
	AudienceValidation bson.M        `json:"audience_validation" bson:"audience_validation"`
	ValidationIssues   []string      `json:"validation_issues" bson:"validation_issues"`
	TokenUsage         bson.M        `json:"token_usage" bson:"token_usage"`
	AnalyzedAt         time.Time     `json:"analyzed_at" bson:"analyzed_at"`
}
type Item struct {
	Original model.Content `json:"original"`
	Analysis Analysis      `json:"analysis"`
}
type BatchScope struct {
	ModelVersion  string   `json:"model_version,omitempty"`
	Database      string   `json:"database"`
	SourceName    string   `json:"source_name"`
	SnapshotHash  string   `json:"snapshot_sha256"`
	SelectionHash string   `json:"selection_hash"`
	SelectedIDs   []string `json:"selected_ids"`
	ExcludedIDs   []string `json:"excluded_ids"`
	SuccessfulIDs []string `json:"successful_ids"`
	ExpectedItems int      `json:"expected_items"`
}

const BatchBundleVersion = "schedule-apply-batch-v1"
const ProductionResponseFormat = "schedule-production-v1"
const FlashResponseFormat = "schedule-v2-flash-comparison-v1"
const QualityResponseFormat = "schedule-v2-quality-v1"
const V2ResponseFormat = "schedule-v2-direct-v1"

type Bundle struct {
	Batch      *BatchScope `json:"batch,omitempty"`
	Version    string      `json:"version"`
	PreparedAt time.Time   `json:"prepared_at"`
	Items      []Item      `json:"items"`
}

func digest(v any) string { b, _ := json.Marshal(v); return fmt.Sprintf("%x", sha256.Sum256(b)) }

// InputHash covers the actual notice input plus source identity, independent of
// mutable analysis fields, DB timestamps, category and lifecycle projections.
func InputHash(v model.Content) string {
	published := ""
	if v.PublishedAt != nil {
		published = v.PublishedAt.UTC().Format(time.RFC3339Nano)
	}
	return digest([]string{v.ID.Hex(), v.SourceID.Hex(), v.SourceName, v.OriginalURL, v.Title, v.Content, published, v.Status})
}
func ProjectionHash(p Projection) string {
	p.ScheduleResultHash = ""
	p.ScheduleAnalysisID = ""
	p.ScheduleAnalyzedAt = nil
	return digest(p)
}
func analysisID(a Analysis) string {
	values := []string{a.ContentID.Hex(), a.SourceID.Hex(), a.InputHash, a.ContentHash, a.ModelVersion, a.ExtractionVersion, a.RawResponseHash, ProjectionHash(a.ValidatedOutput)}
	if a.ResponseFormat != "" {
		values = append(values, a.ResponseFormat)
	}
	return digest(values)
}
func Prepare(original model.Content, n schedules.NoticeResult, raw string, modelVersion string, usage bson.M, analyzed time.Time) (Item, error) {
	return prepare(original, n, raw, modelVersion, usage, analyzed, "")
}
func PrepareProduction(original model.Content, raw, modelVersion string, usage bson.M, analyzed time.Time) (Item, error) {
	q, e := schedules.RevalidateProduction(original.ID.Hex(), raw, schedules.Document{Title: original.Title, Content: original.Content})
	if e != nil {
		return Item{}, e
	}
	return prepare(original, q.Notice, raw, modelVersion, usage, analyzed, ProductionResponseFormat)
}
func PrepareFlash(original model.Content, n schedules.NoticeResult, raw, modelVersion string, usage bson.M, analyzed time.Time) (Item, error) {
	return prepare(original, n, raw, modelVersion, usage, analyzed, FlashResponseFormat)
}
func PrepareV2(original model.Content, n schedules.NoticeResult, raw, modelVersion string, usage bson.M, analyzed time.Time) (Item, error) {
	return prepare(original, n, raw, modelVersion, usage, analyzed, V2ResponseFormat)
}

// PrepareQuality deterministically rebuilds a new validation revision of the
// same immutable response; changing a projection alone cannot pass validation.
func PrepareQuality(original model.Content, raw, modelVersion string, usage bson.M, analyzed time.Time) (Item, error) {
	q, e := schedules.RevalidateQuality(original.ID.Hex(), raw, schedules.Document{Title: original.Title, Content: original.Content})
	if e != nil {
		return Item{}, e
	}
	return prepare(original, q.Notice, raw, modelVersion, usage, analyzed, QualityResponseFormat)
}
func prepare(original model.Content, n schedules.NoticeResult, raw, modelVersion string, usage bson.M, analyzed time.Time, format string) (Item, error) {
	if original.ID.IsZero() || original.ContentHash == "" || (original.SourceName != "ssu_catch" && !(format == ProductionResponseFormat && original.SourceName == "ssu_global_media")) || n.ID != original.ID.Hex() || n.Title != original.Title {
		return Item{}, fmt.Errorf("invalid original notice identity")
	}
	if model.ContentDigest(original) != original.ContentHash {
		return Item{}, fmt.Errorf("original content digest mismatch")
	}
	if analyzed.IsZero() || strings.TrimSpace(modelVersion) == "" || utf8.RuneCountInString(n.Summary) > 100 {
		return Item{}, fmt.Errorf("invalid analysis metadata")
	}
	doc := schedules.Document{Title: original.Title, Content: original.Content}
	var rebuilt schedules.NoticeResult
	var err error
	var quality schedules.QualityResult
	if format == ProductionResponseFormat {
		quality, err = schedules.RevalidateProduction(n.ID, raw, doc)
		rebuilt = quality.Notice
	} else if format == QualityResponseFormat {
		quality, err = schedules.RevalidateQuality(n.ID, raw, doc)
		rebuilt = quality.Notice
	} else if format == V2ResponseFormat || format == FlashResponseFormat {
		rebuilt, err = schedules.RevalidateV2(n.ID, raw, doc)
	} else {
		var old schedules.LegacyOutput
		if json.Unmarshal([]byte(raw), &old) != nil || old.Summary != n.Summary {
			return Item{}, fmt.Errorf("raw summary mismatch")
		}
		rebuilt, err = schedules.Revalidate(n.ID, old, old, doc)
	}
	if err != nil {
		return Item{}, err
	}
	if digest(rebuilt.Schedules) != digest(n.Schedules) || digest(rebuilt.RuleSupplements) != digest(n.RuleSupplements) || digest(rebuilt.Audience) != digest(n.Audience) {
		return Item{}, fmt.Errorf("v2 evidence validation mismatch")
	}
	rows := append(append([]model.Schedule{}, n.Schedules...), n.RuleSupplements...)
	if len(schedules.ValidateSet(rows)) > 0 {
		return Item{}, fmt.Errorf("schedule set invalid")
	}
	issues := []string{}
	review := n.Audience.ReviewRequired || n.Audience.SummaryReviewRequired
	for _, s := range n.Schedules {
		copy := s
		if format != QualityResponseFormat && format != ProductionResponseFormat && !schedules.ValidateSchedule(&copy, doc) && !((format == V2ResponseFormat || format == FlashResponseFormat) && schedules.SafeReviewedPlaceholder(s)) {
			return Item{}, fmt.Errorf("invalid schedule grounding")
		}
		if s.ReviewRequired {
			review = true
		}
		for _, code := range s.ReviewReasons {
			issues = append(issues, s.ID+":"+code)
		}
	}
	for _, s := range n.RuleSupplements {
		if format != QualityResponseFormat && format != ProductionResponseFormat && !schedules.ValidateChild(s, n.Schedules, doc) {
			return Item{}, fmt.Errorf("invalid child session")
		}
		if s.ReviewRequired {
			review = true
		}
	}
	for _, code := range n.Audience.ReviewReasons {
		issues = append(issues, "audience:"+code)
	}
	t := analyzed.UTC().Truncate(time.Millisecond)
	p := Projection{ScheduleMetadata: model.ScheduleMetadata{Summary: n.Summary, TargetAudience: n.Audience.TargetAudience, TargetAudienceReviewRequired: n.Audience.ReviewRequired, TargetAudienceReviewReasons: n.Audience.ReviewReasons, SummaryReviewRequired: n.Audience.SummaryReviewRequired, ScheduleExtractionVersion: schedules.Version, ScheduleExtractionMethod: "saved_ai_response+rule_validation", ScheduleReviewRequired: review, ScheduleAnalyzedAt: &t, ScheduleInputHash: InputHash(original), ScheduleModelVersion: modelVersion}, Schedules: rows}
	p.ScheduleResultHash = ProjectionHash(p)
	audience := bson.M{}
	b, _ := json.Marshal(n.Audience)
	if json.Unmarshal(b, &audience) != nil {
		return Item{}, fmt.Errorf("audience encoding failed")
	}
	if format == V2ResponseFormat || format == FlashResponseFormat {
		p.ScheduleExtractionMethod = "saved_ai_response_v2+rule_validation"
		p.ScheduleResultHash = ProjectionHash(p)
	}
	if format == FlashResponseFormat {
		p.ScheduleValidationVersion = FlashResponseFormat
		p.ScheduleResultHash = ProjectionHash(p)
	}
	if format == QualityResponseFormat || format == ProductionResponseFormat {
		p.ScheduleValidationVersion = schedules.QualityVersion
		p.ScheduleQualityState = quality.ScheduleState
		p.SummaryQualityState = quality.Summary.State
		p.AudienceQualityState = quality.Summary.AudienceState
		p.ScheduleExtractionMethod = "saved_ai_response_v2+offline_quality_validation"
		p.ScheduleResultHash = ProjectionHash(p)
	}
	if format == ProductionResponseFormat {
		p.ScheduleValidationVersion = schedules.ProductionVersion
		p.ScheduleExtractionMethod = "saved_flash+versioned_source_validation"
		p.ScheduleResultHash = ProjectionHash(p)
	}
	a := Analysis{ResponseFormat: format, ContentID: original.ID, SourceID: original.SourceID, SourceName: original.SourceName, InputHash: p.ScheduleInputHash, ContentHash: original.ContentHash, ModelVersion: modelVersion, ExtractionVersion: schedules.Version, RawResponse: raw, RawResponseHash: digest(raw), ValidatedOutput: p, AudienceValidation: audience, ValidationIssues: issues, TokenUsage: usage, AnalyzedAt: t}
	a.ID = analysisID(a)
	a.ValidatedOutput.ScheduleAnalysisID = a.ID
	item := Item{original, a}
	return item, validateItemIntegrity(item)
}
func validateItemIntegrity(item Item) error {
	o, a := item.Original, item.Analysis
	p := a.ValidatedOutput
	if a.ContentID != o.ID || a.SourceID != o.SourceID || a.SourceName != o.SourceName || a.ContentHash != o.ContentHash || a.InputHash != InputHash(o) || a.RawResponseHash != digest(a.RawResponse) || a.ID != analysisID(a) || p.ScheduleAnalysisID != a.ID || p.ScheduleResultHash != ProjectionHash(p) || p.ScheduleInputHash != a.InputHash || p.ScheduleModelVersion != a.ModelVersion || p.ScheduleExtractionVersion != a.ExtractionVersion || p.ScheduleAnalyzedAt == nil || !p.ScheduleAnalyzedAt.Equal(a.AnalyzedAt) {
		return fmt.Errorf("analysis integrity mismatch")
	}
	if a.ExtractionVersion != schedules.Version || ((a.ResponseFormat == FlashResponseFormat || a.ResponseFormat == ProductionResponseFormat) && a.ModelVersion != "gemini-3.8-flash") || (a.ResponseFormat != FlashResponseFormat && a.ResponseFormat != ProductionResponseFormat && a.ModelVersion != "gemini-3.5-flash-lite") {
		return fmt.Errorf("unsupported saved experiment version")
	}
	if model.ContentDigest(o) != o.ContentHash {
		return fmt.Errorf("original input hash mismatch")
	}
	return nil
}
func ValidateBundle(b Bundle) error {
	if b.Version == BatchBundleVersion {
		if e := validateBatchScope(b); e != nil {
			return e
		}
	} else if b.Version != BundleVersion || len(b.Items) != 10 {
		return fmt.Errorf("bundle must contain exactly ten notices")
	}
	seen := map[bson.ObjectID]bool{}
	original, supplement := 0, 0
	for _, v := range b.Items {
		if seen[v.Original.ID] {
			return fmt.Errorf("duplicate content ID")
		}
		seen[v.Original.ID] = true
		if e := ValidateItem(v); e != nil {
			return e
		}
		for _, s := range v.Analysis.ValidatedOutput.Schedules {
			switch s.Origin {
			case "saved_ai_response":
				original++
			case "source_rule_supplement":
				supplement++
			default:
				return fmt.Errorf("unknown schedule origin")
			}
		}
	}
	if b.Version == BundleVersion && (original != 24 || supplement != 4) {
		return fmt.Errorf("expected 24 schedules and four rule sessions")
	}
	return nil
}

func ValidateItem(item Item) error {
	if e := validateItemIntegrity(item); e != nil {
		return e
	}
	var n schedules.NoticeResult
	var e error
	doc := schedules.Document{Title: item.Original.Title, Content: item.Original.Content}
	if item.Analysis.ResponseFormat == ProductionResponseFormat {
		q, err := schedules.RevalidateProduction(item.Original.ID.Hex(), item.Analysis.RawResponse, doc)
		n, e = q.Notice, err
	} else if item.Analysis.ResponseFormat == QualityResponseFormat {
		q, err := schedules.RevalidateQuality(item.Original.ID.Hex(), item.Analysis.RawResponse, doc)
		n, e = q.Notice, err
	} else if item.Analysis.ResponseFormat == V2ResponseFormat || item.Analysis.ResponseFormat == FlashResponseFormat {
		n, e = schedules.RevalidateV2(item.Original.ID.Hex(), item.Analysis.RawResponse, doc)
	} else if item.Analysis.ResponseFormat == "" {
		var raw schedules.LegacyOutput
		if json.Unmarshal([]byte(item.Analysis.RawResponse), &raw) != nil {
			return fmt.Errorf("invalid preserved response")
		}
		n, e = schedules.Revalidate(item.Original.ID.Hex(), raw, raw, doc)
	} else {
		return fmt.Errorf("unsupported response format")
	}
	if e != nil {
		return e
	}
	rebuilt, e := prepare(item.Original, n, item.Analysis.RawResponse, item.Analysis.ModelVersion, item.Analysis.TokenUsage, item.Analysis.AnalyzedAt, item.Analysis.ResponseFormat)
	if e != nil {
		return e
	}
	if digest(rebuilt.Analysis) != digest(item.Analysis) {
		return fmt.Errorf("saved analysis is not the validated source projection")
	}

	return nil
}

func validateBatchScope(b Bundle) error {
	s := b.Batch
	if s == nil || s.Database != "ssu_crawler" || s.SourceName != "ssu_catch" || len(s.SelectedIDs) != 76 || len(s.ExcludedIDs) != 10 || s.ExpectedItems != len(b.Items) || s.ExpectedItems < 1 || s.ExpectedItems > 76 || len(s.SuccessfulIDs) != s.ExpectedItems || len(s.SnapshotHash) != 64 {
		return fmt.Errorf("invalid bounded batch scope")
	}
	expected := digest([]any{"remaining-76-v1", s.Database, "gemini-3.5-flash-lite", s.SnapshotHash, s.SelectedIDs, s.ExcludedIDs})
	if s.SelectionHash != expected {
		return fmt.Errorf("batch selection manifest mismatch")
	}
	selected, excluded, success := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, id := range s.SelectedIDs {
		if _, e := bson.ObjectIDFromHex(id); e != nil || selected[id] {
			return fmt.Errorf("invalid selected IDs")
		}
		selected[id] = true
	}
	for _, id := range s.ExcludedIDs {
		if _, e := bson.ObjectIDFromHex(id); e != nil || excluded[id] || selected[id] {
			return fmt.Errorf("excluded/selected overlap")
		}
		excluded[id] = true
	}
	for _, id := range s.SuccessfulIDs {
		if !selected[id] || success[id] {
			return fmt.Errorf("invalid successful IDs")
		}
		success[id] = true
	}
	for _, v := range b.Items {
		if (v.Analysis.ResponseFormat == FlashResponseFormat) != (s.ModelVersion == "gemini-3.8-flash") || (s.ModelVersion != "" && s.ModelVersion != "gemini-3.8-flash") {
			return fmt.Errorf("batch model provenance mismatch")
		}
		if !success[v.Original.ID.Hex()] || v.Original.SourceName != s.SourceName || (v.Original.Status != "진행" && v.Original.Status != "unknown") || (v.Analysis.ResponseFormat != V2ResponseFormat && v.Analysis.ResponseFormat != QualityResponseFormat && v.Analysis.ResponseFormat != FlashResponseFormat) {
			return fmt.Errorf("batch item outside frozen scope")
		}
	}
	return nil
}
