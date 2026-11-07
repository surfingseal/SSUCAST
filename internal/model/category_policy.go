package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const CategoryPolicyVersion = "official-eight-v1"

var OfficialCategories = []string{"academic", "scholarship", "international", "international_student", "extracurricular", "teaching", "volunteer", "other"}

func IsOfficialCategory(v string) bool {
	for _, c := range OfficialCategories {
		if c == v {
			return true
		}
	}
	return false
}
func NormalizeOfficialCategory(raw string) string {
	raw = NormalizeText(raw)
	if IsOfficialCategory(raw) {
		return raw
	}
	switch raw {
	case "학사":
		return "academic"
	case "장학":
		return "scholarship"
	case "국제교류":
		return "international"
	case "외국인 유학생", "외국인유학생":
		return "international_student"
	case "비교과·행사":
		return "extracurricular"
	case "교직":
		return "teaching"
	case "봉사":
		return "volunteer"
	case "기타":
		return "other"
	}
	return "unknown"
}
func (s Source) ClassificationMode() string {
	if s.ClassificationStrategy == "" {
		return "source"
	}
	return s.ClassificationStrategy
}
func (s Source) ValidateClassificationStrategy() error {
	switch s.ClassificationMode() {
	case "source", "ai", "hybrid":
		return nil
	}
	return fmt.Errorf("classification_strategy must be source, ai or hybrid")
}
func (s Source) ClassificationPolicyHash() string {
	rev := s.ClassificationRevision
	if rev < 1 {
		rev = 1
	}
	b, _ := json.Marshal([]any{CategoryPolicyVersion, s.ClassificationMode(), rev})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type ClassificationDecision struct {
	NeedsAI                              bool
	Category, Status, Method, PolicyHash string
}

func DecideClassification(s Source, v Content) (ClassificationDecision, error) {
	if IsExcludedNotice(v) {
		return ClassificationDecision{}, ErrExcludedCategory
	}
	d := ClassificationDecision{Category: "unknown", Status: "needs_review", Method: "source", PolicyHash: s.ClassificationPolicyHash()}
	if err := s.ValidateClassificationStrategy(); err != nil {
		return d, err
	}
	raw := v.OriginalCategory
	// Compatibility with parsers that historically supplied only a normalized
	// category. Never reuse a previous AI projection as official source evidence.
	if raw == "" && v.ClassificationMethod != "ai" && s.ClassificationMode() == "source" && IsOfficialCategory(v.Category) {
		raw = v.Category
	}
	normalized := NormalizeOfficialCategory(raw)
	if s.ClassificationMode() == "ai" || s.ClassificationMode() == "hybrid" && (normalized == "unknown" || normalized == "other") {
		d.NeedsAI = true
		d.Method = "ai"
		return d, nil
	}
	d.Category = normalized
	if normalized != "unknown" {
		d.Status = "classified"
	}
	return d, nil
}

// PrepareClassification assigns the pending/official projection, never invokes
// a model, and never changes the original labels extracted by the crawler.
func PrepareClassification(s Source, v *Content) error {
	d, err := DecideClassification(s, *v)
	if err != nil {
		return err
	}
	v.ClassificationStrategy = s.ClassificationMode()
	v.ClassificationPolicyHash = d.PolicyHash
	v.ClassificationMethod = d.Method
	v.ClassificationStatus = d.Status
	v.ModelVersion = ""
	v.Category = d.Category
	v.Categories = []string{d.Category}
	if !d.NeedsAI {
		categories := []string{}
		add := func(c string) {
			if c == "unknown" {
				return
			}
			for _, x := range categories {
				if x == c {
					return
				}
			}
			categories = append(categories, c)
		}
		add(d.Category)
		for _, raw := range v.OriginalCategories {
			add(NormalizeOfficialCategory(raw))
		}
		if len(categories) > 0 {
			v.Categories = categories
		}
	}
	return nil
}

var ErrExcludedCategory = fmt.Errorf("notice category excluded from collection")

func IsExcludedCategory(raw string) bool {
	v := NormalizeText(raw)
	return v == "채용" || v == "교원채용"
}
func IsExcludedNotice(v Content) bool { return CollectionExclusion(v) != nil }
