package schedules

import (
	"example.com/crawlserver/internal/model"
	"regexp"
	"strings"
	"unicode/utf8"
)

type AudienceClaim struct {
	Role     string                 `json:"role"`
	Text     string                 `json:"text"`
	Evidence model.EvidenceLocation `json:"evidence"`
}
type AudienceValidation struct {
	Raw                       *string         `json:"raw_target_audience"`
	TargetAudience            *string         `json:"target_audience"`
	Mandatory                 []AudienceClaim `json:"mandatory_conditions"`
	Preferences               []AudienceClaim `json:"preferences"`
	ReviewRequired            bool            `json:"review_required"`
	ReviewReasons             []string        `json:"review_reasons"`
	Summary                   string          `json:"summary"`
	SummaryCharacters         int             `json:"summary_characters"`
	SummaryReviewRequired     bool            `json:"summary_review_required"`
	SummaryIssues             []string        `json:"summary_issues"`
	UnsupportedBenefitNumbers []string        `json:"unsupported_benefit_numbers"`
}

var audienceHeading = regexp.MustCompile(`(?:지원\s*대상|신청\s*대상|참가\s*대상|참여\s*대상|모집\s*대상|조사\s*대상|적용\s*대상|신청\s*자격|지원\s*자격|수혜\s*자격)`)
var majorHeading = regexp.MustCompile(`^\s*[0-9]+\.\s*[^0-9]`)
var number = regexp.MustCompile(`[0-9]+(?:[.,][0-9]+)*`)

func ValidateAudience(old LegacyOutput, doc Document) AudienceValidation {
	a := AudienceValidation{Raw: old.TargetAudience, Mandatory: []AudienceClaim{}, Preferences: []AudienceClaim{}, ReviewReasons: []string{}, Summary: old.Summary, SummaryCharacters: utf8.RuneCountInString(old.Summary), SummaryReviewRequired: old.SummaryReviewRequired, SummaryIssues: []string{}, UnsupportedBenefitNumbers: []string{}}
	lines := strings.Split(doc.Content, "\n")
	inSection := false
	sectionLines := 0
	explicitMatches := []string{}
	add := func(role, text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		for _, c := range append(append([]AudienceClaim{}, a.Mandatory...), a.Preferences...) {
			if c.Role == role && c.Text == text {
				return
			}
		}
		claim := AudienceClaim{role, text, MatchEvidence(doc.Title, doc.Content, text)}
		if role == "preference" {
			a.Preferences = append(a.Preferences, claim)
		} else {
			a.Mandatory = append(a.Mandatory, claim)
		}
	}
	for i, line := range lines {
		clean := strings.TrimSpace(line)
		preference := strings.Contains(clean, "우대") || strings.Contains(clean, "가산점") || strings.Contains(clean, "우선 선발")
		if preference {
			add("preference", clean)
		}
		heading := audienceHeading.FindStringIndex(clean)
		if heading != nil {
			inSection = true
			sectionLines = 0
			after := strings.TrimSpace(strings.TrimLeft(clean[heading[1]:], ":： "))
			// A labelled one-line audience is explicit; retain the entire qualifier.
			if after != "" {
				if preference {
					a.ReviewReasons = unique(a.ReviewReasons, "mixed_audience_conditions")
					inSection = false
					continue
				}
				add("mandatory", clean)
				explicitMatches = append(explicitMatches, after)
				inSection = false
			} else if i+1 < len(lines) {
				explicitMatches = append(explicitMatches, strings.TrimSpace(lines[i+1]))
			}
			continue
		}
		if inSection {
			if majorHeading.MatchString(clean) || isFieldHeading(clean) || sectionLines >= 16 {
				inSection = false
			} else if clean != "" {
				if !preference {
					add("mandatory", clean)
				}
				sectionLines++
			}
		}
		if strings.Contains(clean, "을 대상으로") || strings.Contains(clean, "를 대상으로") {
			pos := strings.Index(clean, "대상으로")
			before := strings.TrimSpace(clean[:pos])
			before = strings.TrimSuffix(strings.TrimSuffix(before, "을"), "를")
			explicitMatches = append(explicitMatches, before)
		}
		if strings.Contains(clean, "선발에서 배제") || strings.Contains(clean, "참여 불가") || strings.Contains(clean, "지원 불가") || strings.Contains(clean, "이중수혜 불가능") || strings.Contains(clean, "둘중 하나") {
			if !preference {
				add("restriction", clean)
			}
		}
	}
	raw := value(old.TargetAudience)
	if raw != "" {
		matched := MatchEvidence(doc.Title, doc.Content, raw)
		explicit := false
		for _, x := range explicitMatches {
			if NormalizeWhitespace(x) == NormalizeWhitespace(raw) {
				explicit = true
			}
		}
		if !matched.Matched {
			a.ReviewReasons = unique(a.ReviewReasons, "inferred_audience")
		} else if !explicit {
			a.ReviewReasons = unique(a.ReviewReasons, "audience_scope_unverified")
			if matched.Field == "content" {
				prefix := doc.Content[:matched.ByteStart]
				if strings.Contains(prefix, "설립") || strings.Contains(prefix, "비영리") {
					a.ReviewReasons = unique(a.ReviewReasons, "foundation_intro_not_eligibility")
				}
			}
		}
		// A broad noun is not a substitute for a multi-condition scholarship table.
		if (len(a.Mandatory) > 2 && (utf8.RuneCountInString(raw) < 15 || !matched.Matched)) || strings.Contains(doc.Content, "수혜자격") && utf8.RuneCountInString(raw) < 15 {
			a.ReviewReasons = unique(a.ReviewReasons, "incomplete_eligibility")
			explicit = false
		}
		if explicit && matched.Matched && len(a.ReviewReasons) == 0 {
			a.TargetAudience = sp(raw)
		}
	}
	a.ReviewRequired = len(a.ReviewReasons) > 0
	if a.ReviewRequired {
		a.SummaryReviewRequired = true
		a.SummaryIssues = unique(a.SummaryIssues, "audience_claim_requires_review")
	}
	if a.SummaryCharacters > 100 || a.SummaryCharacters == 0 {
		a.SummaryReviewRequired = true
		a.SummaryIssues = unique(a.SummaryIssues, "summary_length")
	}
	// Numeric checking is necessary, not sufficient: benefit eligibility semantics
	// cannot be proved by substring matching. No new wording replaces the summary.
	for _, n := range number.FindAllString(old.Summary, -1) {
		if !strings.Contains(doc.Title+doc.Content, n) {
			a.UnsupportedBenefitNumbers = unique(a.UnsupportedBenefitNumbers, n)
			a.SummaryReviewRequired = true
			a.SummaryIssues = unique(a.SummaryIssues, "unsupported_summary_number")
		}
	}
	return a
}
func isFieldHeading(s string) bool {
	for _, h := range []string{"선발인원", "지급금액", "장학금 유형", "지원 일정", "서류접수", "면접일정", "신청 방법", "문의", "지급기준", "기준성적"} {
		if s == h {
			return true
		}
	}
	return false
}
