package model

import (
	"regexp"
	"strings"
)

// ExclusionReason contains a rule code and evidence location, never the body or
// personal data. Callers can audit skipped items without storing excluded posts.
type ExclusionReason struct {
	Code  string `json:"code"`
	Field string `json:"field"`
	Rule  string `json:"rule"`
}

var employmentRules = []struct {
	code, rule string
	pattern    *regexp.Regexp
}{
	{"faculty_recruitment", "faculty-hiring-v1", regexp.MustCompile(`(교원|교수|강사|전임교원|비전임교원).{0,24}(채용|초빙)|(채용|초빙).{0,24}(교원|교수|강사)`)},
	{"employment", "explicit-hiring-v1", regexp.MustCompile(`\[채용\]|\[교원채용\]|(직원|연구원|조교|인턴|계약직|정규직).{0,16}(채용|공개채용)|(공개채용|신규채용|채용공고|채용 공고|채용 안내)`)},
}
var careerEvent = regexp.MustCompile(`채용\s*(설명회|박람회|상담|특강|정보|연계|연계형)|취업\s*(설명회|박람회|특강|상담|교육)`)

func CollectionExclusion(v Content) *ExclusionReason {
	raws := append([]string{v.OriginalCategory}, v.OriginalCategories...)
	for _, raw := range raws {
		if IsExcludedCategory(raw) {
			return &ExclusionReason{"official_category_excluded", "original_category", NormalizeText(raw)}
		}
	}
	// Official source categories take precedence. Body keyword checks are for
	// uncategorized department boards, not incidental hiring mentions in 학사 etc.
	if c := NormalizeOfficialCategory(v.OriginalCategory); c != "unknown" && c != "other" {
		return nil
	}
	fields := []struct{ name, text string }{{"title", v.Title}, {"content", v.Content}}
	for _, field := range fields {
		// Evaluate sentences individually so a career event cannot mask a separate
		// explicit hiring statement. A bare '모집' is deliberately never a signal.
		for _, line := range strings.FieldsFunc(field.text, func(r rune) bool { return r == '\n' || r == '.' || r == ';' }) {
			if careerEvent.MatchString(line) {
				continue
			}
			for _, rule := range employmentRules {
				if rule.pattern.MatchString(line) {
					return &ExclusionReason{rule.code, field.name, rule.rule}
				}
			}
		}
	}
	return nil
}
