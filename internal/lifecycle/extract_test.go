package lifecycle

import (
	"example.com/crawlserver/internal/model"
	"testing"
	"time"
)

func TestDeadlineEvidenceCases(t *testing.T) {
	pub := time.Date(2026, 10, 1, 0, 0, 0, 0, Seoul)
	now := time.Date(2026, 11, 6, 16, 0, 0, 0, Seoul)
	for _, tc := range []struct{ name, title, body, status, date string }{
		{"period", "접수 안내", "접수기간: 2026.10.01 ~ 2026.11.10", "active", "2026-11-10T23:59:59.999+09:00"},
		{"explicit time", "장학 신청", "신청기간: 2026.11.06.(금) 15:00까지", "expired", "2026-11-06T15:00:00+09:00"},
		{"pm", "장학 안내", "지원기간: 10월 1일 ~ 11월 10일 오후 5시까지", "active", "2026-11-10T17:00:00+09:00"},
		{"same day times", "학사 안내", "대체 수강신청 기간: 2026.11.06.(금) 11:00 ~ 20:00", "active", "2026-11-06T20:00:00+09:00"},
		{"event separate", "참가 신청", "신청 마감: 2026.10.20\n행사일: 2026.11.20\n발표일: 2026.11.25", "expired", "2026-10-20T23:59:59.999+09:00"},
		{"same line event", "신청 안내", "신청마감: 2026.11.10, 행사일: 2026.11.20", "active", "2026-11-10T23:59:59.999+09:00"},
		{"multiple", "신청 안내", "1차 신청마감: 2026.11.01\n2차 신청마감: 2026.11.10", "needs_review", ""},
		{"extension", "접수 안내", "기존 신청마감: 2026.11.01\n신청마감 변경: 2026.11.10", "active", "2026-11-10T23:59:59.999+09:00"},
		{"extension with other action", "접수 안내", "기존 신청마감: 2026.11.01\n신청마감 변경: 2026.11.10\n서류제출 마감: 2026.11.05", "needs_review", ""},
		{"rolling", "참가자 모집", "신청마감: 2026.11.10 선착순 조기 마감 가능", "needs_review", ""},
		{"reference", "교류 안내", "접수마감일(2026.10.02.) 기준 유효기간 2년 이내 성적만 인정", "needs_review", ""},
		{"none", "증명서 발급 안내", "증명서 발급 시스템 주소가 변경되었습니다.", "no_deadline", ""},
		{"missing", "참가자 모집", "신청기간은 붙임 이미지를 확인해 주세요.", "needs_review", ""},
		{"invalid", "신청 안내", "신청마감: 2026.02.30", "needs_review", ""},
		{"survey", "설문 안내", "설문기간: 2026.10.26.(월) ~ 11.09.(월)", "active", "2026-11-09T23:59:59.999+09:00"},
		{"title shorthand", "장학생 모집(~11.10)", "추천 서류를 제출해 주세요.", "active", "2026-11-10T23:59:59.999+09:00"},
		{"title less specific", "신청 안내(~11.10)", "신청마감: 2026.11.10 17:00까지", "active", "2026-11-10T17:00:00+09:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := model.Content{Title: tc.title, Content: tc.body, PublishedAt: &pub}
			r := Extract(v, now)
			if r.Status != tc.status {
				t.Fatalf("got %s want %s: %+v", r.Status, tc.status, r)
			}
			if tc.date != "" {
				if r.Deadline == nil || r.Deadline.In(Seoul).Format(time.RFC3339Nano) != tc.date {
					t.Fatal(r.Deadline, tc.date)
				}
			} else if r.Deadline != nil {
				t.Fatal("uncertain deadline confirmed")
			}
		})
	}
}
func TestDeadlineYearBoundaryAndImages(t *testing.T) {
	pub := time.Date(2026, 12, 20, 0, 0, 0, 0, Seoul)
	now := pub
	for _, tc := range []struct{ body, status string }{{"접수기간: 2026.12.20 ~ 01.05", "active"}, {"신청마감: 01.05", "needs_review"}, {"신청기간: 12.20 ~ 01.05", "needs_review"}} {
		r := Extract(model.Content{Title: "신청 안내", Content: tc.body, PublishedAt: &pub}, now)
		if r.Status != tc.status {
			t.Fatal(tc, r)
		}
		if r.Confirmed && r.Deadline.In(Seoul).Year() != 2027 {
			t.Fatal(r)
		}
	}
	r := Extract(model.Content{Title: "신청 안내", ImageURLs: []string{"https://fixture.invalid/poster.png"}}, now)
	if r.Status != "needs_review" || r.Deadline != nil {
		t.Fatal("OCR inference", r)
	}
}
