package model

import "time"

type DeadlineEvidence struct {
	Source     string     `bson:"source" json:"source"`
	Sentence   string     `bson:"sentence" json:"sentence"`
	Deadline   *time.Time `bson:"deadline,omitempty" json:"deadline,omitempty"`
	YearPolicy string     `bson:"year_policy,omitempty" json:"year_policy,omitempty"`
	TimePolicy string     `bson:"time_policy,omitempty" json:"time_policy,omitempty"`
	Extension  bool       `bson:"extension" json:"extension"`
}
type LifecycleEvaluation struct {
	Version     string             `bson:"version" json:"version"`
	InputHash   string             `bson:"input_hash" json:"input_hash"`
	ContentHash string             `bson:"content_hash" json:"content_hash"`
	CheckedAt   time.Time          `bson:"checked_at" json:"checked_at"`
	Status      string             `bson:"status" json:"status"`
	Confirmed   bool               `bson:"confirmed" json:"confirmed"`
	Deadline    *time.Time         `bson:"deadline" json:"deadline"`
	Evidence    []DeadlineEvidence `bson:"evidence" json:"evidence"`
	Reasons     []string           `bson:"reasons" json:"reasons"`
}
