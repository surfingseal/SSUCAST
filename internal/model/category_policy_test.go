package model

import (
	"testing"
)

func TestOfficialEightNormalizationAndStrategies(t *testing.T) {
	for raw, want := range map[string]string{"학사": "academic", "장학": "scholarship", "국제교류": "international", "외국인 유학생": "international_student", "비교과·행사": "extracurricular", "교직": "teaching", "봉사": "volunteer", "기타": "other", "채용": "unknown", "student_support": "unknown", "career": "unknown"} {
		if got := NormalizeOfficialCategory(raw); got != want {
			t.Fatal(raw, got, want)
		}
	}
	for _, tc := range []struct {
		strategy, raw, want string
		ai                  bool
	}{{"source", "장학", "scholarship", false}, {"source", "미등록분류", "unknown", false}, {"", "", "unknown", false}, {"ai", "학사", "unknown", true}, {"hybrid", "학사", "academic", false}, {"hybrid", "기타", "unknown", true}, {"hybrid", "일반", "unknown", true}, {"hybrid", "", "unknown", true}} {
		s := Source{Name: "arbitrary-name", ClassificationStrategy: tc.strategy}
		d, err := DecideClassification(s, Content{OriginalCategory: tc.raw})
		if err != nil || d.Category != tc.want || d.NeedsAI != tc.ai {
			t.Fatal(tc, d, err)
		}
	}
	if (Source{ClassificationStrategy: "unsupported"}).ValidateClassificationStrategy() == nil {
		t.Fatal("invalid strategy accepted")
	}
	a := Source{ClassificationStrategy: "ai", ClassificationRevision: 1}
	b := a
	b.ClassificationRevision = 2
	if a.ClassificationPolicyHash() == b.ClassificationPolicyHash() {
		t.Fatal("policy revision ignored")
	}
}

func TestExcludedCategoriesAreNotClassificationFallbacks(t *testing.T) {
	for _, raw := range []string{"채용", "교원채용", " 채용 "} {
		for _, strategy := range []string{"source", "ai", "hybrid"} {
			if _, err := DecideClassification(Source{ClassificationStrategy: strategy}, Content{OriginalCategory: raw}); err != ErrExcludedCategory {
				t.Fatal(raw, strategy, err)
			}
		}
	}
	if !IsExcludedNotice(Content{OriginalCategory: "학사", OriginalCategories: []string{"학사", "교원채용"}}) {
		t.Fatal("excluded secondary label missed")
	}
	if IsExcludedCategory("교직") {
		t.Fatal("teaching incorrectly excluded")
	}
}
