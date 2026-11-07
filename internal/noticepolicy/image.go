package noticepolicy

import (
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/schedules"
	"net/url"
	"strings"
	"time"
)

const ImageSourcePolicy = "image-source-accepted-by-user-v1"

// Acceptance changes only the user's derived projection, never the immutable
// model result. Real uncertainty/conflicts remain reviewable regardless of source.
func acceptImage(s *model.Schedule) bool {
	if len(s.ImageEvidence) == 0 || s.Timezone != "Asia/Seoul" || s.StartPartial != nil || s.EndPartial != nil || s.StartDate == nil && s.EndDate == nil || s.Quality != nil && s.Quality.State == "blocked" {
		return false
	}
	for _, e := range s.ImageEvidence {
		u, err := url.Parse(e.ImageURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || strings.TrimSpace(e.Text) == "" || len(e.ImageSHA256) != 64 || e.Confidence != "high" && e.Confidence != "medium" {
			return false
		}
	}
	for i, d := range []*string{s.StartDate, s.EndDate} {
		clock := []*string{s.StartTime, s.EndTime}[i]
		if d == nil {
			if clock != nil {
				return false
			}
			continue
		}
		if _, e := time.Parse("2006-01-02", *d); e != nil {
			return false
		}
		if clock != nil {
			if _, e := time.Parse("15:04", *clock); e != nil {
				return false
			}
		}
	}
	if s.StartDate != nil && s.EndDate != nil {
		if *s.StartDate > *s.EndDate {
			return false
		}
		if *s.StartDate == *s.EndDate && s.StartTime != nil && s.EndTime != nil && *s.StartTime > *s.EndTime {
			return false
		}
	}
	sourceOnly := map[string]bool{
		"image_evidence_not_independently_verified": true, "image_model_review_required": true, "agent_visual_review_not_human_verified": true,
		"image_schedule_auto_review": true, "image_source_verification": true, "image_candidate_review_required": true, "image_source_verification_needed": true,
		"image_schedule_candidate_review_required": true, "image_schedule_confirmation": true, "image_schedule_extracted": true, "image_schedule_detected": true,
		"이미지 추출 일정 검토 필요": true, "이미지에서 추출된 일정으로 검토 필요": true,
	}
	for _, r := range s.ReviewReasons {
		if !sourceOnly[strings.ToLower(strings.TrimSpace(r))] {
			return false
		}
	}
	if len(s.ReviewReasons) == 0 && s.ReviewRequired {
		return false
	} // Unknown review flag is not provenance-only.
	s.ReviewReasons = []string{}
	s.ReviewRequired = false
	s.Corrections = append(s.Corrections, model.ScheduleCorrection{Code: ImageSourcePolicy, Field: "source_trust", Original: "image_provenance_only_review"})
	Refresh(s)
	s.Quality = schedules.DeriveQuality(*s, nil)
	return s.Quality.State == "verified" || s.Quality.State == "partial"
}
