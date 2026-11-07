package model

// CalendarParts preserves partial calendar information without inventing a year,
// day or clock. Full dates use YYYY-MM-DD strings, not midnight BSON Dates.
type CalendarParts struct {
	Year  *int `json:"year" bson:"year"`
	Month *int `json:"month" bson:"month"`
	Day   *int `json:"day" bson:"day"`
}

// EvidenceLocation uses zero-based, end-exclusive offsets in Field (title/content).
// Byte offsets address the original UTF-8 string; rune offsets address Unicode.
type EvidenceLocation struct {
	Matched      bool   `json:"matched" bson:"matched"`
	MatchType    string `json:"match_type" bson:"match_type"`
	Field        string `json:"field" bson:"field"`
	ByteStart    int    `json:"byte_start" bson:"byte_start"`
	ByteEnd      int    `json:"byte_end" bson:"byte_end"`
	RuneStart    int    `json:"rune_start" bson:"rune_start"`
	RuneEnd      int    `json:"rune_end" bson:"rune_end"`
	OriginalText string `json:"original_text" bson:"original_text"`
}
type ScheduleCorrection struct {
	Code     string `json:"code" bson:"code"`
	Field    string `json:"field" bson:"field"`
	Original string `json:"original" bson:"original"`
}

// SourceSpan is a separate contiguous quotation, never a synthesized quote.
type SourceSpan struct {
	Role     string           `json:"role" bson:"role"`
	Location EvidenceLocation `json:"location" bson:"location"`
}

// ScheduleQuality is derived by an offline, versioned validator. No notifications
// are scheduled by these flags; a candidate still requires user/product policy.
type ScheduleQuality struct {
	Version           string   `json:"version" bson:"version"`
	State             string   `json:"state" bson:"state"`
	Reasons           []string `json:"reasons" bson:"reasons"`
	DisplayAllowed    bool     `json:"display_allowed" bson:"display_allowed"`
	DeadlineCandidate bool     `json:"deadline_candidate" bson:"deadline_candidate"`
}
type Schedule struct {
	EvidenceSpans []SourceSpan     `json:"evidence_spans,omitempty" bson:"evidence_spans,omitempty"`
	Quality       *ScheduleQuality `json:"quality,omitempty" bson:"quality,omitempty"`

	ID               string               `json:"id" bson:"id"`
	ParentID         *string              `json:"parent_id" bson:"parent_id"`
	Type             string               `json:"type" bson:"type"`
	Label            string               `json:"label" bson:"label"`
	StartDate        *string              `json:"start_date" bson:"start_date"`
	StartTime        *string              `json:"start_time" bson:"start_time"`
	EndDate          *string              `json:"end_date" bson:"end_date"`
	EndTime          *string              `json:"end_time" bson:"end_time"`
	StartPartial     *CalendarParts       `json:"start_partial" bson:"start_partial"`
	EndPartial       *CalendarParts       `json:"end_partial" bson:"end_partial"`
	Timezone         string               `json:"timezone" bson:"timezone"`
	Precision        string               `json:"precision" bson:"precision"`
	StartPrecision   string               `json:"start_precision" bson:"start_precision"`
	EndPrecision     string               `json:"end_precision" bson:"end_precision"`
	StartAt          *string              `json:"start_at" bson:"start_at"`
	EndAt            *string              `json:"end_at" bson:"end_at"`
	Scope            *string              `json:"scope" bson:"scope"`
	Evidence         string               `json:"evidence" bson:"evidence"`
	EvidenceLocation EvidenceLocation     `json:"evidence_location" bson:"evidence_location"`
	Origin           string               `json:"origin" bson:"origin"`
	ExtractionMethod string               `json:"extraction_method" bson:"extraction_method"`
	ConversionMethod string               `json:"conversion_method" bson:"conversion_method"`
	DateContext      string               `json:"date_context,omitempty" bson:"date_context,omitempty"`
	ReviewRequired   bool                 `json:"review_required" bson:"review_required"`
	ReviewReasons    []string             `json:"review_reasons" bson:"review_reasons"`
	Omissions        []string             `json:"omissions" bson:"omissions"`
	Qualifiers       []string             `json:"qualifiers" bson:"qualifiers"`
	Corrections      []ScheduleCorrection `json:"corrections" bson:"corrections"`
}
