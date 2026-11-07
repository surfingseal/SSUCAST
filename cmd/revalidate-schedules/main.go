// Offline migration and revalidation only. This binary does not load .env,
// credentials, SDK clients, HTTP clients or MongoDB repositories.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/schedules"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type selection struct {
	Items []struct {
		ID string `json:"id"`
		schedules.Document
	} `json:"items"`
}
type savedResult struct {
	ID        string                 `json:"id"`
	Raw       string                 `json:"raw_response"`
	Validated schedules.LegacyOutput `json:"validated_output"`
}
type metrics struct {
	Notices               int `json:"notices"`
	OriginalSchedules     int `json:"original_schedules"`
	GroundedSchedules     int `json:"grounded_schedules"`
	ValidSchedules        int `json:"structurally_valid_grounded_schedules"`
	ReviewSchedules       int `json:"review_schedules"`
	FullDateSchedules     int `json:"full_date_schedules"`
	PartialDateSchedules  int `json:"partial_date_schedules"`
	RestoredBoundaries    int `json:"restored_boundaries"`
	RestoredFullDates     int `json:"restored_full_dates"`
	RestoredPartialDates  int `json:"restored_partial_dates"`
	ResolvedEvidence      int `json:"resolved_whitespace_evidence"`
	OldAIReview           int `json:"old_ai_review_schedules"`
	OldValidatedReview    int `json:"old_validated_review_schedules"`
	AudienceReview        int `json:"audience_review_notices"`
	MissingSessionNotices int `json:"missing_session_notices"`
	RuleSessions          int `json:"separate_rule_sessions"`
	APIRequests           int `json:"gemini_requests"`
	SchoolRequests        int `json:"school_http_requests"`
	MongoReads            int `json:"mongodb_reads"`
	MongoWrites           int `json:"mongodb_writes"`
}

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	input := flag.String("input-dir", "", "existing experiment containing saved data")
	output := flag.String("output-dir", "", "new local output directory")
	flag.Parse()
	if *input == "" || *output == "" {
		return fmt.Errorf("-input-dir and -output-dir required")
	}
	in, e := filepath.Abs(*input)
	if e != nil {
		return e
	}
	out, e := filepath.Abs(*output)
	if e != nil {
		return e
	}
	if in == out {
		return fmt.Errorf("cannot overwrite original experiment")
	}
	hashes := map[string]string{}
	files := map[string][]byte{}
	for _, name := range []string{"selected.json", "results.json", "comparison.json", "report.md"} {
		raw, e := os.ReadFile(filepath.Join(in, name))
		if e != nil {
			return fmt.Errorf("required saved input unavailable: %s", name)
		}
		files[name] = raw
		hashes[name] = fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	var sel selection
	var saved []savedResult
	if e = json.Unmarshal(files["selected.json"], &sel); e != nil {
		return e
	}
	if e = json.Unmarshal(files["results.json"], &saved); e != nil {
		return e
	}
	if len(sel.Items) != 10 || len(saved) != 10 {
		return fmt.Errorf("expected actual ten-notice experiment")
	}
	seen := map[string]bool{}
	notices := []schedules.NoticeResult{}
	diff := []map[string]any{}
	m := metrics{Notices: 10}
	for i, v := range sel.Items {
		r := saved[i]
		if r.ID != v.ID || seen[v.ID] {
			return fmt.Errorf("notice ID mismatch/duplicate")
		}
		seen[v.ID] = true
		var raw schedules.LegacyOutput
		if e = json.Unmarshal([]byte(r.Raw), &raw); e != nil {
			return fmt.Errorf("invalid saved raw response for %s", v.ID)
		}
		n, e := schedules.Revalidate(v.ID, raw, r.Validated, v.Document)
		if e != nil {
			return e
		}
		for j, s := range n.Schedules {
			m.OriginalSchedules++
			if s.EvidenceLocation.Matched {
				m.GroundedSchedules++
			}
			check := s
			if schedules.ValidateSchedule(&check, v.Document) {
				m.ValidSchedules++
			}
			if s.StartDate != nil || s.EndDate != nil {
				m.FullDateSchedules++
			} else if s.StartPartial != nil || s.EndPartial != nil {
				m.PartialDateSchedules++
			}
			if s.ReviewRequired {
				m.ReviewSchedules++
			}
			d := n.Diffs[j]
			if d.OldAIReviewRequired {
				m.OldAIReview++
			}
			if d.OldValidatedReviewRequired {
				m.OldValidatedReview++
			}
			if !d.OldExactEvidenceMatched && s.EvidenceLocation.Matched {
				m.ResolvedEvidence++
			}
			for _, b := range d.Boundaries {
				if b.Restored {
					m.RestoredBoundaries++
					if b.Date != nil {
						m.RestoredFullDates++
					} else {
						m.RestoredPartialDates++
					}
				}
			}
		}
		if n.Audience.ReviewRequired {
			m.AudienceReview++
		}
		if len(n.RuleSupplements) > 0 {
			m.MissingSessionNotices++
			m.RuleSessions += len(n.RuleSupplements)
		}
		for _, child := range n.RuleSupplements {
			if !schedules.ValidateChild(child, n.Schedules, v.Document) {
				return fmt.Errorf("rule session grounding failed")
			}
		}
		if len(n.SetIssues) > 0 {
			return fmt.Errorf("duplicate schedule detected")
		}
		notices = append(notices, n)
		diff = append(diff, map[string]any{"id": v.ID, "title": v.Title, "schedules": n.Diffs, "audience": n.Audience, "separate_rule_supplements": len(n.RuleSupplements)})
	}
	if m.OriginalSchedules != 24 {
		return fmt.Errorf("expected exactly 24 legacy schedules")
	}
	if e = os.MkdirAll(out, 0700); e != nil {
		return e
	}
	data := map[string]any{"version": schedules.Version, "metrics": m, "notices": notices}
	for name, value := range map[string]any{"results-v2.json": data, "diff.json": diff, "provenance.json": map[string]any{"generated_at": time.Now().UTC(), "input_directory": in, "input_sha256": hashes, "conversion": "local deterministic evidence rules; original experiment immutable", "network_clients_created": 0, "gemini_requests": 0, "school_requests": 0, "mongodb_reads": 0, "mongodb_writes": 0}} {
		if e = writeJSON(filepath.Join(out, name), value); e != nil {
			return e
		}
	}
	if e = os.WriteFile(filepath.Join(out, "report.md"), []byte(report(m, notices)), 0600); e != nil {
		return e
	}
	// Verify original experiment inputs were not changed by revalidation.
	for name, hash := range hashes {
		raw, e := os.ReadFile(filepath.Join(in, name))
		if e != nil || fmt.Sprintf("%x", sha256.Sum256(raw)) != hash {
			return fmt.Errorf("original input changed")
		}
	}
	return json.NewEncoder(os.Stdout).Encode(m)
}
func writeJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}
func show(date, clock *string, p *model.CalendarParts) string {
	s := "null"
	if date != nil {
		s = *date
	} else if p != nil {
		v := func(n *int) string {
			if n == nil {
				return "?"
			}
			return fmt.Sprint(*n)
		}
		s = v(p.Year) + "-" + v(p.Month) + "-" + v(p.Day)
	}
	if clock != nil {
		s += " " + *clock
	}
	return s
}
func report(m metrics, ns []schedules.NoticeResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 일정 모델 v2: 기존 10건 오프라인 재검증\n\nGemini/Vertex 0회, 학교 HTTP 0회, MongoDB 읽기·쓰기·삭제 0회. 기존 공고·deadline·lifecycle 상태를 변경하지 않았다. 원본 실험의 raw_response, validated_output, comparison.json 및 원문을 사용했다.\n\n")
	fmt.Fprintf(&b, "| 항목 | 결과 |\n|---|---:|\n| 기존 일정 | %d |\n| 근거 확인 | %d |\n| 구조·달력 검증 성공 | %d |\n| 정밀도를 명시해 보존 | %d/%d |\n| 완전 날짜 보유 일정 / 부분 날짜 일정 | %d / %d |\n| 복원된 경계 | %d (완전 날짜 %d, 부분 날짜 %d) |\n| 공백·줄바꿈 근거 해결 | %d |\n| 검토 필요 일정 | %d |\n| 기존 AI / 기존 보수적 결과 / 새 검토 일정 | %d / %d / %d |\n| 대상 검토 공고 | %d |\n| 누락 세션 발견 공고 / 별도 rule 세션 | %d / %d |\n\n", m.OriginalSchedules, m.GroundedSchedules, m.ValidSchedules, m.FullDateSchedules+m.PartialDateSchedules, m.OriginalSchedules, m.FullDateSchedules, m.PartialDateSchedules, m.RestoredBoundaries, m.RestoredFullDates, m.RestoredPartialDates, m.ResolvedEvidence, m.ReviewSchedules, m.OldAIReview, m.OldValidatedReview, m.ReviewSchedules, m.AudienceReview, m.MissingSessionNotices, m.RuleSessions)
	b.WriteString("검증 성공은 날짜 정밀도와 원문 근거를 구조적으로 표현할 수 있다는 뜻이며, 모든 일정이 최종 확정됐다는 뜻은 아니다. 복원은 기존 validated_output의 null 경계가 matched evidence에 있던 날짜/부분 날짜를 다시 보유한 경우다. 원문에 없는 연도나 시각은 복원하지 않는다.\n\n## 모델과 검토 정책\n\n- start_date/end_date: 완전한 YYYY-MM-DD만 저장. 부분 정보는 start_partial/end_partial의 nullable year/month/day로 보존한다.\n- start_time/end_time: 명시된 HH:mm만 저장. timezone=Asia/Seoul. precision 및 경계별 precision은 minute/date/month/year/month_day/mixed/unknown을 구분한다.\n- 시각이 없다는 정보는 omissions=[missing_time]이며 단독으로 검토 사유가 아니다. missing_year, tentative_schedule, conditional_deadline, source_conflict 등은 review_reasons 배열에 보존한다.\n- start_at/end_at은 완전 날짜와 명시 시각이 모두 있을 때만 생성하는 호환 필드다. 날짜 전용 값을 임의의 자정 BSON Date로 바꾸지 않는다.\n- 새 date/time 값은 원문 인용에서 결정적 규칙으로 재구성하므로 extraction_method=rule이다. 기존 AI 제안 일정은 origin=saved_ai_response로, 원문에서 추가 추출한 세션은 origin=source_rule_supplement로 구분한다. 원래 AI 값은 기존 파일에 그대로 남는다.\n- 근거는 공백 정규화만 허용한다. 날짜·기관명·금액·자격 조건·구두점은 바꾸지 않는다. offsets는 title/content별 UTF-8 byte와 Unicode rune, 0부터 시작하고 끝은 exclusive다.\n\n## 기존 24개 일정 비교\n\n| 공고/일정 | 시작 | 종료 | 정밀도 | 기존 AI→기존 보수→새 검토 | 새 검토 사유 |\n|---|---|---|---|---|---|\n")
	for _, n := range ns {
		for i, s := range n.Schedules {
			d := n.Diffs[i]
			fmt.Fprintf(&b, "| %s / %s | %s | %s | %s | %t→%t→%t | %s |\n", strings.ReplaceAll(n.Title, "|", "\\|"), s.Label, show(s.StartDate, s.StartTime, s.StartPartial), show(s.EndDate, s.EndTime, s.EndPartial), s.Precision, d.OldAIReviewRequired, d.OldValidatedReviewRequired, s.ReviewRequired, strings.Join(s.ReviewReasons, ", "))
		}
	}
	b.WriteString("\n## 참여 대상과 요약\n\n요약 원문은 변경하지 않았다. 100자 제한과 숫자 검사를 유지했다. 대상은 명시된 필수 조건과 우대 사항을 별도 evidence와 함께 기록한다. 일반화/추론/재단 소개를 실제 지원자격으로 판단할 수 없으면 target_audience=null이다. 숫자 검사는 혜택의 의미적 정확성을 보증하지 않으며 사람의 정답 라벨은 사용하지 않았다.\n\n")
	for _, n := range ns {
		fmt.Fprintf(&b, "### %s\n\n원래 요약 (%d자): %s\n\n대상 검토: %t — %s. 근거 없는 요약 숫자: %v.\n\n", n.Title, n.Audience.SummaryCharacters, n.Summary, n.Audience.ReviewRequired, strings.Join(n.Audience.ReviewReasons, ", "), n.Audience.UnsupportedBenefitNumbers)
	}
	b.WriteString("## 원문 세션 보완\n\nAI가 누락한 세션은 기존 24개에 섞지 않고 rule_supplements에 분리했다. parent_id는 날짜 근거가 있는 전체 행사와 연결된다. 서로 다른 기관·차수·scope·parent는 같은 시각만으로 중복 제거하지 않는다.\n\n")
	for _, n := range ns {
		for _, s := range n.RuleSupplements {
			fmt.Fprintf(&b, "- %s: %s ~ %s, evidence=%q, parent=%s, extraction_method=rule\n", s.Label, show(s.StartDate, s.StartTime, s.StartPartial), show(s.EndDate, s.EndTime, s.EndPartial), s.Evidence, *s.ParentID)
		}
	}
	b.WriteString("\n## 검증할 수 없는 정보와 남은 한계\n\n연도 없는 학점인정 표는 월·일을 보존하지만 연도를 확정하지 않는다. 월 단위 발표는 일자와 시각을 생성하지 않는다. 예정·이후·변경 가능 표현은 그대로 검토 대상이다. 겨울 제목과 여름 본문의 충돌은 원문 확인이 필요하다. 기관명과 일정 종류의 의미, 지원조건 전체와 요약 함의는 문자열 검사만으로 완전히 입증할 수 없다. 재단 소개의 청소년과 실제 대학생 지원 조건, SAF의 관심 학생 추론, 국가근로·TOPIK의 복수 자격조건은 별도 검토가 필요하다. 확인된 근거 없는 혜택 수치는 이번 10건에서 없다.\n\n## 시스템 호환성·테스트\n\nContent.schedules는 선택적 JSON/BSON 필드이며 content_hash 계산과 기존 deadline/lifecycle/classification/API/Source 전략을 바꾸지 않는다. DB 호출은 없으며 BSON 왕복으로 날짜 정밀도·timezone을 검증한다. 전체 테스트에서는 데이터 변경 금지에 따라 TEST_MONGODB_URI를 제거해 쓰기 통합 테스트를 명시적으로 건너뛴다. 로컬 DB에 쓰는 테스트를 성공했다고 보고하지 않는다. 테스트 실행 내역은 checks.json에 기록한다.\n")
	return b.String()
}
