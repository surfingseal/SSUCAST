package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Source struct {
	CollectionMode      string `bson:"collection_mode,omitempty" json:"collection_mode,omitempty"`
	PolicyApprovalBasis string `bson:"policy_approval_basis,omitempty" json:"policy_approval_basis,omitempty"`
	RequestJitterMS     int64  `bson:"request_jitter_ms,omitempty" json:"request_jitter_ms,omitempty"`
	MaxRetries          *int   `bson:"max_retries,omitempty" json:"max_retries,omitempty"`

	DisplayName            string        `bson:"display_name,omitempty" json:"display_name,omitempty"`
	ClassificationStrategy string        `bson:"classification_strategy" json:"classification_strategy"`
	ClassificationRevision int64         `bson:"classification_revision" json:"classification_revision"`
	PolicyApproved         bool          `bson:"policy_approved,omitempty" json:"policy_approved,omitempty"`
	DetailBodySelector     string        `bson:"detail_body_selector,omitempty" json:"detail_body_selector,omitempty"`
	CrawlerType            string        `bson:"crawler_type,omitempty" json:"crawler_type,omitempty"`
	ListURL                string        `bson:"list_url,omitempty" json:"list_url,omitempty"`
	RequestIntervalMS      int64         `bson:"request_interval_ms,omitempty" json:"request_interval_ms,omitempty"`
	MaxPages               int           `bson:"max_pages,omitempty" json:"max_pages,omitempty"`
	MaxContents            int           `bson:"max_contents,omitempty" json:"max_contents,omitempty"`
	MaxRequests            int           `bson:"max_requests,omitempty" json:"max_requests,omitempty"`
	MaxConcurrency         int           `bson:"max_concurrency,omitempty" json:"max_concurrency,omitempty"`
	AutoEnabled            *bool         `bson:"auto_enabled,omitempty" json:"auto_enabled,omitempty"`
	ID                     bson.ObjectID `bson:"_id" json:"id"`
	Name                   string        `bson:"name" json:"name"`
	BaseURL                string        `bson:"base_url" json:"base_url"`
	Enabled                bool          `bson:"enabled" json:"enabled"`
	Schedule               string        `bson:"schedule" json:"schedule"`
	LastCrawledAt          *time.Time    `bson:"last_crawled_at" json:"last_crawled_at"`
	CreatedAt              time.Time     `bson:"created_at" json:"created_at"`
	UpdatedAt              time.Time     `bson:"updated_at" json:"updated_at"`
}
type Job struct {
	ErrorCode      string        `bson:"error_code,omitempty" json:"error_code,omitempty"`
	ErrorStage     string        `bson:"error_stage,omitempty" json:"error_stage,omitempty"`
	Deadline       time.Time     `bson:"deadline" json:"deadline"`
	ID             bson.ObjectID `bson:"_id" json:"id"`
	SourceID       bson.ObjectID `bson:"source_id" json:"source_id"`
	Status         string        `bson:"status" json:"status"`
	StartedAt      time.Time     `bson:"started_at" json:"started_at"`
	FinishedAt     *time.Time    `bson:"finished_at" json:"finished_at"`
	CollectedCount int           `bson:"collected_count" json:"collected_count"`
	ErrorCount     int           `bson:"error_count" json:"error_count"`
	ErrorMessage   string        `bson:"error_message" json:"error_message"`
}
type Content struct {
	NoticePolicy   *NoticePolicy `json:"notice_policy,omitempty" bson:"notice_policy,omitempty"`
	ExternalPostID string        `bson:"external_post_id,omitempty" json:"external_post_id,omitempty"`
	IsPinned       bool          `bson:"is_pinned,omitempty" json:"is_pinned,omitempty"`
	ParserVersion  string        `bson:"parser_version,omitempty" json:"parser_version,omitempty"`
	CategorySource string        `bson:"category_source,omitempty" json:"category_source,omitempty"`

	ScheduleMetadata         `bson:",inline"`
	Schedules                []Schedule           `bson:"schedules,omitempty" json:"schedules,omitempty"`
	LifecycleStatus          string               `bson:"lifecycle_status,omitempty" json:"lifecycle_status,omitempty"`
	Lifecycle                *LifecycleEvaluation `bson:"lifecycle,omitempty" json:"lifecycle,omitempty"`
	ImageURLs                []string             `bson:"image_urls" json:"image_urls,omitempty"`
	Author                   string               `bson:"author" json:"author,omitempty"`
	ClassificationStrategy   string               `bson:"classification_strategy" json:"classification_strategy"`
	ClassificationPolicyHash string               `bson:"classification_policy_hash" json:"classification_policy_hash"`
	ClassificationInputHash  string               `bson:"classification_input_hash" json:"classification_input_hash"`
	ClassificationMethod     string               `bson:"classification_method" json:"classification_method"`
	ClassificationStatus     string               `bson:"classification_status" json:"classification_status"`
	ModelVersion             string               `bson:"model_version" json:"model_version"`
	ClassificationUpdatedAt  *time.Time           `bson:"classification_updated_at,omitempty" json:"classification_updated_at,omitempty"`
	SourceName               string               `bson:"source_name,omitempty" json:"source_name,omitempty"`
	OriginalCategory         string               `bson:"original_category,omitempty" json:"original_category,omitempty"`
	OriginalCategories       []string             `bson:"original_categories,omitempty" json:"original_categories,omitempty"`
	Categories               []string             `bson:"categories,omitempty" json:"categories,omitempty"`
	Department               string               `bson:"department,omitempty" json:"department,omitempty"`
	Status                   string               `bson:"status,omitempty" json:"status,omitempty"`
	Attachments              []Attachment         `bson:"attachments" json:"attachments"`
	Deadline                 *time.Time           `bson:"deadline" json:"deadline"`
	LastSeenAt               time.Time            `bson:"last_seen_at" json:"last_seen_at"`
	ID                       bson.ObjectID        `bson:"_id,omitempty" json:"id"`
	SourceID                 bson.ObjectID        `bson:"source_id" json:"source_id"`
	Title                    string               `bson:"title" json:"title"`
	Content                  string               `bson:"content" json:"content"`
	OriginalURL              string               `bson:"original_url" json:"original_url"`
	Category                 string               `bson:"category" json:"category"`
	Tags                     []string             `bson:"tags" json:"tags"`
	PublishedAt              *time.Time           `bson:"published_at" json:"published_at"`
	CrawledAt                time.Time            `bson:"crawled_at" json:"crawled_at"`
	UpdatedAt                time.Time            `bson:"updated_at" json:"updated_at"`
	ContentHash              string               `bson:"content_hash" json:"content_hash"`
	Metadata                 map[string]string    `bson:"metadata" json:"metadata"`
}
type Attachment struct {
	Extension string `bson:"extension,omitempty" json:"extension,omitempty"`
	Name      string `bson:"name" json:"name"`
	URL       string `bson:"url" json:"url"`
}
type Filter struct {
	IncludeCompleted bool
	IncludeExpired   bool
	LifecycleStatus  string
	Page, Limit      int
	SourceID         *bson.ObjectID
	Category         string
	SourceName       string
	From, To         *time.Time
}

// Missing crawler_type and auto_enabled identify legacy records. Preserve their schedule.
func (s Source) CrawlerKey() string {
	if s.CrawlerType != "" {
		return s.CrawlerType
	}
	return s.Name
}
func (s Source) Automatic() bool {
	// The SSU MVP is manual-only even if stale DB/config records enable cron.
	if s.CrawlerKey() == "ssu_catch" {
		return false
	}
	if !s.Enabled {
		return false
	}
	if s.AutoEnabled != nil {
		return *s.AutoEnabled
	}
	return s.CrawlerType == ""
}

// Zero values in existing BSON records retain the original example limits.
func (s Source) WithDefaults() Source {
	if s.ClassificationStrategy == "" {
		s.ClassificationStrategy = "source"
	}
	if s.ClassificationRevision < 1 {
		s.ClassificationRevision = 1
	}
	if s.ListURL == "" {
		s.ListURL = s.BaseURL
	}
	if s.RequestIntervalMS == 0 {
		s.RequestIntervalMS = 1000
	}
	if s.MaxPages == 0 {
		s.MaxPages = 100
	}
	if s.MaxContents == 0 {
		s.MaxContents = 1000
	}
	if s.MaxRequests == 0 {
		s.MaxRequests = 10000
	}
	if s.MaxConcurrency == 0 {
		s.MaxConcurrency = 1
	}
	return s
}
