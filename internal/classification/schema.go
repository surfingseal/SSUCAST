package classification

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"example.com/crawlserver/internal/model"
	"google.golang.org/genai"
)

const TaxonomyVersion = "official-eight-v1-prompt3"

var LegacyPrimary = []string{"academic", "scholarship", "career", "extracurricular", "competition", "international", "volunteer", "student_support", "other", "unknown"}
var Primary = append(append([]string{}, model.OfficialCategories...), "unknown")
var Activities = []string{"internship", "field_training", "lecture", "mentoring", "contest", "scholarship", "recruitment", "exchange_program", "volunteering", "other"}
var Topics = []string{"software", "ai", "design", "business", "finance", "language", "humanities", "engineering", "general"}
var Statuses = []string{"classified", "needs_review"}

func Schema() *genai.Schema {
	str := func(values []string) *genai.Schema { return &genai.Schema{Type: genai.TypeString, Enum: values} }
	arr := func(values []string) *genai.Schema { return &genai.Schema{Type: genai.TypeArray, Items: str(values)} }
	evidence := &genai.Schema{Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{"field": str([]string{"categories", "activity_types", "topics", "target_audience"}), "label": str(nil), "source": str([]string{"title", "content", "original_category"}), "quote": str(nil)}, Required: []string{"field", "label", "source", "quote"}}}
	return &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{
		"evidence": evidence, "primary_category": str(Primary), "categories": {Type: genai.TypeArray, Items: str(Primary), MinItems: genai.Ptr[int64](1), MaxItems: genai.Ptr[int64](1)}, "activity_types": arr(Activities), "topics": arr(Topics), "target_audience": arr(nil), "classification_status": str(Statuses), "reasoning_summary": str(nil),
	}, Required: []string{"primary_category", "categories", "activity_types", "topics", "target_audience", "classification_status", "reasoning_summary", "evidence"}}
}
func Decode(raw string) (model.ClassificationLabels, error) {
	var v model.ClassificationLabels
	// Pointer presence distinguishes a missing string from its zero value.
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &fields) != nil {
		return v, fmt.Errorf("invalid classification JSON")
	}
	for _, key := range []string{"primary_category", "categories", "activity_types", "topics", "target_audience", "classification_status", "reasoning_summary"} {
		b, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
			return v, fmt.Errorf("missing/null classification field %s", key)
		}
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return v, fmt.Errorf("invalid classification shape")
	}
	if d.Decode(new(any)) != io.EOF {
		return v, fmt.Errorf("trailing classification JSON")
	}
	if err := Validate(v); err != nil {
		return v, err
	}
	return v, nil
}
func Validate(v model.ClassificationLabels) error { return validateLabels(v, Primary, true) }
func ValidateLegacy(v model.ClassificationLabels) error {
	return validateLabels(v, LegacyPrimary, false)
}
func validateLabels(v model.ClassificationLabels, primary []string, single bool) error {
	has := func(values []string, s string) bool {
		for _, x := range values {
			if x == s {
				return true
			}
		}
		return false
	}
	if !has(primary, v.PrimaryCategory) || !has(Statuses, v.ClassificationStatus) || strings.TrimSpace(v.ReasoningSummary) == "" || len(v.ReasoningSummary) > 2000 {
		return fmt.Errorf("invalid category/status/reasoning")
	}
	for _, a := range []struct{ values, allowed []string }{{v.Categories, primary}, {v.ActivityTypes, Activities}, {v.Topics, Topics}} {
		if a.values == nil {
			return fmt.Errorf("classification arrays required")
		}
		seen := map[string]bool{}
		for _, s := range a.values {
			if !has(a.allowed, s) || seen[s] {
				return fmt.Errorf("invalid or duplicate taxonomy value")
			}
			seen[s] = true
		}
	}
	if single && len(v.Categories) != 1 {
		return fmt.Errorf("exactly one official category or unknown required")
	}
	if !has(v.Categories, v.PrimaryCategory) {
		return fmt.Errorf("categories must include primary_category")
	}
	if v.PrimaryCategory == "unknown" && v.ClassificationStatus != "needs_review" {
		return fmt.Errorf("unknown requires needs_review")
	}
	if v.TargetAudience == nil || len(v.TargetAudience) > 20 {
		return fmt.Errorf("target_audience array required")
	}
	seen := map[string]bool{}
	for _, s := range v.TargetAudience {
		if strings.TrimSpace(s) == "" || len(s) > 200 || seen[s] {
			return fmt.Errorf("invalid audience")
		}
		seen[s] = true
	}
	if len(v.Evidence) > 64 {
		return fmt.Errorf("too many evidence items")
	}
	for _, e := range v.Evidence {
		if !has([]string{"categories", "activity_types", "topics", "target_audience"}, e.Field) || !has([]string{"title", "content", "original_category"}, e.Source) || e.Label == "" || strings.TrimSpace(e.Quote) == "" || len(e.Quote) > 2000 {
			return fmt.Errorf("invalid evidence structure")
		}
	}
	return nil
}
func InputJSON(v model.Content) []byte {
	// Exact same normalized data is both hashed and sent to the model.
	data := map[string]any{"title": model.NormalizeText(v.Title), "original_category": model.NormalizeText(v.OriginalCategory), "content": model.NormalizeText(v.Content), "source": v.SourceName, "original_url": v.OriginalURL, "published_at": v.PublishedAt, "department": model.NormalizeText(v.Department), "images_present": v.Metadata["has_images"] == "true", "images_analyzed": false, "attachments_present": len(v.Attachments) > 0, "attachments_analyzed": false}
	b, _ := json.Marshal(data)
	return b
}
func InputHash(v model.Content) string {
	s := sha256.Sum256(InputJSON(v))
	return hex.EncodeToString(s[:])
}

// Policy revision and body hash are part of the reservation identity, while the
// original source category and all submitted input remain independently hashed.
func PolicyInputHash(s model.Source, v model.Content) string {
	b, _ := json.Marshal([]string{InputHash(v), model.ContentDigest(v), s.ClassificationPolicyHash()})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
