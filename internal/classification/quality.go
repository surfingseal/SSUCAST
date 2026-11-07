package classification

import (
	"example.com/crawlserver/internal/model"
	"regexp"
	"strings"
	"unicode/utf8"
)

const QualityVersion = "quality-v1"

var activityTerms = map[string][]string{
	"internship": {"인턴", "internship"}, "field_training": {"현장실습", "field training"}, "lecture": {"특강", "강연", "강의", "lecture"}, "mentoring": {"멘토링", "mentoring"}, "contest": {"공모전", "경진대회", "contest"}, "scholarship": {"장학", "scholarship"}, "recruitment": {"채용", "모집", "recruitment"}, "exchange_program": {"교환학생", "교환 프로그램", "exchange"}, "volunteering": {"봉사", "volunteer"}, "other": {"기타 활동", "other activity"},
}
var topicTerms = map[string][]string{"software": {"소프트웨어", "프로그래밍", "software"}, "ai": {"인공지능", " ai ", "ai"}, "design": {"디자인", "design"}, "business": {"경영", "business"}, "finance": {"금융", "finance"}, "language": {"어학", "언어", "영어", "language"}, "humanities": {"인문", "humanities"}, "engineering": {"공학", "engineering"}, "general": {"일반 분야", "general topic"}}

func cloneLabels(v model.ClassificationLabels) model.ClassificationLabels {
	v.Categories = append([]string{}, v.Categories...)
	v.ActivityTypes = append([]string{}, v.ActivityTypes...)
	v.Topics = append([]string{}, v.Topics...)
	v.TargetAudience = append([]string{}, v.TargetAudience...)
	v.Evidence = append([]model.ClassificationEvidence{}, v.Evidence...)
	return v
}

// ApplyQuality is a conservative evidence gate, not a semantic truth detector.
// Raw predictions are retained independently; unsupported claims are omitted
// only from the validated labels and always leave a structured rejection.
func ApplyQuality(item model.Content, raw model.ClassificationLabels) (model.ClassificationLabels, model.ClassificationQuality) {
	out := cloneLabels(raw)
	q := model.ClassificationQuality{Version: QualityVersion, ReviewReasons: []string{}, RejectedClaims: []model.RejectedClaim{}, ImagesUnanalysed: item.Metadata["has_images"] == "true", AttachmentsUnanalysed: len(item.Attachments) > 0, ShortText: utf8.RuneCountInString(model.NormalizeText(item.Content)) < 40}
	add := func(s string) {
		for _, x := range q.ReviewReasons {
			if s == x {
				return
			}
		}
		q.ReviewReasons = append(q.ReviewReasons, s)
	}
	q.ImagesState = "unknown"
	if state, ok := item.Metadata["has_images"]; ok {
		if state == "true" {
			q.ImagesState = "unanalysed"
		} else if state == "false" {
			q.ImagesState = "absent"
		}
	}
	if q.ShortText {
		add("insufficient_information")
	}
	if q.ImagesUnanalysed {
		add("images_unanalysed")
	}
	if q.AttachmentsUnanalysed {
		add("attachments_unanalysed")
	}
	source := map[string]string{"title": model.NormalizeText(item.Title), "content": model.NormalizeText(item.Content), "original_category": model.NormalizeText(item.OriginalCategory)}
	supports := func(field, label string, candidates []model.ClassificationEvidence) bool {
		for _, e := range candidates {
			quote := model.NormalizeText(e.Quote)
			if e.Field != field || e.Label != label || quote == "" || source[e.Source] == "" || !strings.Contains(source[e.Source], quote) {
				continue
			}
			lower := strings.ToLower(quote)
			directive := false
			for _, t := range []string{"이전 지시", "생성하라", "ignore previous", "ignore all previous"} {
				if strings.Contains(strings.ToLower(source[e.Source]), t) {
					directive = true
				}
			}
			if directive {
				continue
			}
			// Negation is conservatively abstained rather than interpreted as support.
			negative := false
			for _, term := range []string{"아니", "아닙", "아닌", "아님", "없", "않", "not ", "no "} {
				if strings.Contains(lower, term) {
					negative = true
				}
			}
			// A short quote such as "인턴" cannot hide negation later in the
			// same sentence. Other unrelated sentences do not invalidate evidence.
			contexts := regexp.MustCompile(`[.!?\n]`).Split(source[e.Source], -1)
			contextualPositive := false
			contextFound := false
			for _, sentence := range contexts {
				if strings.Contains(sentence, quote) {
					contextFound = true
					neg := false
					for _, term := range []string{"아니", "아닙", "아닌", "아님", "없", "않", "not ", "no "} {
						if strings.Contains(strings.ToLower(sentence), term) {
							neg = true
						}
					}
					if !neg {
						contextualPositive = true
					}
				}
			}
			if negative || contextFound && !contextualPositive {
				continue
			}
			if field == "target_audience" {
				if quote == label {
					return true
				}
				continue
			}
			terms := activityTerms[label]
			if field == "topics" {
				terms = topicTerms[label]
			}
			for _, term := range terms {
				matched := strings.Contains(lower, strings.ToLower(term))
				if term == "ai" {
					matched = regexp.MustCompile(`(?i)(^|[^a-z])ai([^a-z]|$)`).MatchString(quote)
				}
				if matched {
					return true
				}
			}
		}
		return false
	}
	filter := func(field string, values []string) []string {
		accepted := []string{}
		for _, label := range values {
			if supports(field, label, raw.Evidence) {
				accepted = append(accepted, label)
			} else {
				q.RejectedClaims = append(q.RejectedClaims, model.RejectedClaim{Field: field, Label: label, Reason: "missing_or_unsupported_text_evidence"})
				add("unsupported_claim")
			}
		}
		return accepted
	}
	out.ActivityTypes = filter("activity_types", raw.ActivityTypes)
	out.Topics = filter("topics", raw.Topics)
	out.TargetAudience = filter("target_audience", raw.TargetAudience)
	out.Evidence = []model.ClassificationEvidence{}
	for _, e := range raw.Evidence {
		values := out.ActivityTypes
		if e.Field == "topics" {
			values = out.Topics
		}
		if e.Field == "target_audience" {
			values = out.TargetAudience
		}
		if e.Field == "categories" {
			continue
		}
		for _, label := range values {
			if label == e.Label && supports(e.Field, label, []model.ClassificationEvidence{e}) {
				out.Evidence = append(out.Evidence, e)
				break
			}
		}
	}
	school := model.NormalizeOfficialCategory(item.OriginalCategory)
	if school != "unknown" && school != "other" {
		contains := false
		for _, v := range raw.Categories {
			if v == school {
				contains = true
			}
		}
		if !contains {
			q.SourceCategoryDisagreement = true
			add("source_category_disagreement")
		}
	}
	if raw.PrimaryCategory == "unknown" {
		add("classification_ambiguity")
	}
	// A model's unstructured needs_review is kept, but never interpreted as an
	// objective ambiguity claim unless the reason is known.
	if raw.ClassificationStatus == "needs_review" && len(q.ReviewReasons) == 0 {
		add("model_review_unspecified")
	}
	out.ReasoningSummary = "원문 인용과 검토 규칙에 따른 결과입니다. 모델의 원래 근거는 raw_labels.reasoning_summary에 보존했습니다."
	if len(q.ReviewReasons) > 0 {
		out.ClassificationStatus = "needs_review"
		out.ReasoningSummary += " 검토 사유: " + strings.Join(q.ReviewReasons, ", ")
	}
	return out, q
}
