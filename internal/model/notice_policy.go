package model

import "time"

// NoticePolicy is a user-approved derived projection, never an AI raw result.
// Original schedules and immutable analyses remain available for audit.
type NoticePolicy struct {
	Version          string     `json:"version" bson:"version"`
	Year             int        `json:"year" bson:"year"`
	YearBasis        string     `json:"year_basis" bson:"year_basis"`
	ContentHash      string     `json:"content_hash" bson:"content_hash"`
	InputHash        string     `json:"input_hash" bson:"input_hash"`
	AnalysisID       string     `json:"analysis_id" bson:"analysis_id"`
	AppliedAt        time.Time  `json:"applied_at" bson:"applied_at"`
	Schedules        []Schedule `json:"schedules" bson:"schedules"`
	ExpiresAt        *time.Time `json:"expires_at" bson:"expires_at"`
	ExpiryReason     string     `json:"expiry_reason" bson:"expiry_reason"`
	BoundariesFilled int        `json:"boundaries_filled" bson:"boundaries_filled"`
}
