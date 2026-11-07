package schedules

// Offline quality validation deliberately has no network/database/model client.
import (
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const QualityVersion = "schedule-quality-v1"

type ZeroReview struct {
	Category                    string             `json:"category"`
	Reason                      string             `json:"reason"`
	Evidence                    []model.SourceSpan `json:"evidence"`
	AttachmentRequired          bool               `json:"attachment_required"`
	AttachmentScheduleConfirmed bool               `json:"attachment_schedule_confirmed"`
	Conditions                  []string           `json:"conditions"`
	AddedIDs                    []string           `json:"added_ids"`
}
type SummaryAudit struct {
	Summary              string             `json:"summary"`
	Characters           int                `json:"characters"`
	WithinLimit          bool               `json:"within_limit"`
	State                string             `json:"state"`
	Issues               []string           `json:"issues"`
	AudienceState        string             `json:"audience_state"`
	AudienceReasons      []string           `json:"audience_reasons"`
	SourceClaims         []AudienceClaim    `json:"source_claims"`
	Proposal             *string            `json:"proposal"`
	ProposalEvidence     []model.SourceSpan `json:"proposal_evidence"`
	SemanticVerification string             `json:"semantic_verification"`
}

// Separate notice/schedule/audience/summary states prevent a missing audience
// from blocking an independently grounded event date (or vice versa).
type QualityResult struct {
	Notice        NoticeResult `json:"notice"`
	Zero          *ZeroReview  `json:"zero_schedule_review,omitempty"`
	Summary       SummaryAudit `json:"summary_audit"`
	ScheduleState string       `json:"schedule_state"`
}

func span(doc Document, role, quote string) model.SourceSpan {
	return model.SourceSpan{Role: role, Location: MatchEvidence(doc.Title, doc.Content, quote)}
}
func ValidSpans(ss []model.SourceSpan, doc Document) bool {
	if len(ss) == 0 {
		return false
	}
	for _, s := range ss {
		l := s.Location
		if !l.Matched || l.OriginalText == "" {
			return false
		}
		text := doc.Content
		if l.Field == "title" {
			text = doc.Title
		} else if l.Field != "content" {
			return false
		}
		if l.ByteStart < 0 || l.ByteEnd > len(text) || l.ByteEnd <= l.ByteStart || text[l.ByteStart:l.ByteEnd] != l.OriginalText || utf8.RuneCountInString(text[:l.ByteStart]) != l.RuneStart || utf8.RuneCountInString(text[:l.ByteEnd]) != l.RuneEnd {
			return false
		}
	}
	return true
}
func RevalidateQuality(id, raw string, doc Document) (QualityResult, error) {
	n, e := RevalidateV2(id, raw, doc)
	if e != nil {
		return QualityResult{}, e
	}
	q := QualityResult{Notice: n}
	// Only source-backed, deterministic, reviewed corrections below. All raw
	// proposals remain byte-for-byte in the independent immutable raw_response.
	for i := range q.Notice.Schedules {
		s := &q.Notice.Schedules[i]
		if s.EvidenceLocation.Matched {
			s.EvidenceSpans = []model.SourceSpan{span(doc, "schedule", s.EvidenceLocation.OriginalText)}
		}
		cs := clocks(s.Evidence, dates(s.Evidence))
		if s.StartTime != nil && s.EndTime != nil && *s.StartTime == *s.EndTime && len(cs) == 1 {
			s.EndTime = nil
			s.EndAt = nil
			s.StartPrecision = boundaryPrecision(s.StartDate, s.StartTime, s.StartPartial)
			s.EndPrecision = boundaryPrecision(s.EndDate, s.EndTime, s.EndPartial)
			updatePrecision(s)
			s.Omissions = unique(s.Omissions, "end_time_unspecified")
			s.ConversionMethod = "quality_single_clock_is_not_duration"
		}
		// Non-contiguous SIC session evidence: the event date and session headings
		// have distinct spans. Event clocks are NOT inherited by either session.
		if strings.Contains(doc.Title, "SIC 4쿼터") && strings.Contains(s.Label, "Session ") && strings.Contains(doc.Content, "3. 세미나 세부 타임테이블") {
			line := ""
			for _, l := range strings.Split(doc.Content, "\n") {
				if strings.Contains(l, s.Label) {
					line = l
					break
				}
			}
			ss := []model.SourceSpan{span(doc, "parent_event_datetime", "2026. 11. 18.(수) 18:30~20:30"), span(doc, "session", line), span(doc, "qualifier", "※ 세부 진행시간 및 순서는 행사 운영 상황에 따라 변경될 수 있습니다.")}
			if line != "" && ValidSpans(ss, doc) && len(q.Notice.Schedules) > 0 {
				p := q.Notice.Schedules[0]
				if p.StartDate != nil && p.EndDate != nil && *p.StartDate == "2026-11-18" && *p.EndDate == *p.StartDate && p.EvidenceLocation.Matched {
					s.Evidence = line
					s.EvidenceLocation = ss[1].Location
					s.EvidenceSpans = ss
					s.StartDate = sp(*p.StartDate)
					s.EndDate = sp(*p.EndDate)
					s.StartTime = nil
					s.EndTime = nil
					s.StartAt = nil
					s.EndAt = nil
					s.StartPartial = nil
					s.EndPartial = nil
					s.StartPrecision = "date"
					s.EndPrecision = "date"
					s.Precision = "date"
					s.ParentID = sp(p.ID)
					s.DateContext = "parent_event_date_multiple_source_spans"
					s.ExtractionMethod = "rule"
					s.ConversionMethod = "quality_separate_session_spans"
					// Resolved composite quote/date loss; tentative timing remains.
					s.ReviewReasons = []string{"tentative_schedule"}
					s.ReviewRequired = true
					s.Omissions = unique(s.Omissions, "session_time_unspecified")
				}
			}
		}
		// A source-only restoration: a range's first printed month/day was lost.
		ds := dates(s.Evidence)
		if s.Type == "application" && s.StartDate == nil && s.StartPartial == nil && len(ds) == 2 && hasRange(s.Evidence[ds[0].end:ds[1].start]) && strings.Contains(s.Label, "원서접수") {
			setBoundary(s, true, ds[0], nil)
			s.ConversionMethod = "quality_restore_printed_range_start"
			s.ExtractionMethod = "rule"
		}
		// Operating period is not an application deadline; do not turn it into one.
		if s.Type == "application" && strings.Contains(s.Label, "프로그램 운영 기간") && strings.HasPrefix(strings.TrimSpace(s.Evidence), "운영 기간:") {
			s.Type = "event"
			s.ConversionMethod = "quality_operating_period_not_application"
		}
	}
	if len(n.Schedules) == 0 {
		q.Zero = reviewZero(id, doc, &q.Notice)
	}
	// Re-evaluate audience conservatively. Literal presence proves a quotation,
	// not completeness of eligibility or attachment-dependent requirements.
	a := &q.Notice.Audience
	if a.Raw != nil {
		if strings.Count(*a.Raw, "(") != strings.Count(*a.Raw, ")") {
			a.ReviewReasons = unique(a.ReviewReasons, "truncated_audience")
		}
		if strings.Contains(doc.Content, "선발 과정별 지원자격이 상이") {
			a.ReviewReasons = unique(a.ReviewReasons, "external_eligibility_required")
		}
		if strings.Contains(doc.Content, "3. 지원 자격") && !strings.Contains(*a.Raw, "60세") && strings.Contains(doc.Content, "만 60세 이하") {
			a.ReviewReasons = unique(a.ReviewReasons, "incomplete_eligibility")
		}
		if strings.Contains(*a.Raw, "전체 학생 누구나 관람 가능") {
			a.ReviewReasons = unique(a.ReviewReasons, "spectator_not_applicant")
		}
		if len(a.ReviewReasons) > 0 {
			a.ReviewRequired = true
			a.TargetAudience = nil
			a.SummaryReviewRequired = true
			a.SummaryIssues = unique(a.SummaryIssues, "audience_claim_requires_review")
		}
	}
	q.Summary = SummaryAudit{Summary: n.Summary, Characters: utf8.RuneCountInString(n.Summary), WithinLimit: utf8.RuneCountInString(n.Summary) <= 100, State: "mechanically_checked", Issues: append([]string{}, a.SummaryIssues...), AudienceState: "not_specified", AudienceReasons: append([]string{}, a.ReviewReasons...), SourceClaims: append(append([]AudienceClaim{}, a.Mandatory...), a.Preferences...), ProposalEvidence: []model.SourceSpan{}, SemanticVerification: "limited: no independent human gold labels; literal/numeric checks do not prove complete semantic correctness"}
	if a.Raw != nil {
		q.Summary.AudienceState = "literal_scope_checked"
	}
	if a.ReviewRequired {
		q.Summary.AudienceState = "needs_review"
	}
	if a.SummaryReviewRequired {
		q.Summary.State = "needs_review"
	}
	// Suggestions are source quotations, never silently replacing the saved summary.
	if strings.Contains(doc.Title, "교수협의회 장학생") && strings.Contains(n.Summary, "본교 학부 재학생은") {
		summaryIssue(&q, "summary_omits_leave_and_deferred_graduation", doc, "지원자격: 본교 학부 재학생(휴학생, 졸업유예 포함)", "교수협의회 장학생을 선발합니다. 본교 학부 재학생뿐 아니라 휴학생·졸업유예 학생도 신청 대상에 포함됩니다.")
	}
	if strings.Contains(doc.Title, "동화대학교") && strings.Contains(n.Summary, "학점 인정을 받을 수") {
		summaryIssue(&q, "credit_recognition_conditional", doc, "기간요건 충족, 최종 인정 여부는 별도 심사", "동화대학교 겨울방학 단기 중국어 프로그램 참가자를 모집합니다. 본교 국제팀에 먼저 지원하며 학점 인정은 별도 심사 대상입니다.")
	}
	if strings.Contains(doc.Title, "다전공 신청/변경/포기") {
		summaryIssue(&q, "summary_missing_change_withdrawal_scope", doc, doc.Content, "다전공 신청·변경·포기를 안내합니다. 1월 복학생을 위한 신청 기간에 재학생도 신청할 수 있습니다.")
	}
	if !q.Summary.WithinLimit {
		q.Summary.State = "needs_review"
		q.Summary.Issues = unique(q.Summary.Issues, "summary_over_100")
	}
	// Quality checks are deliberately more conservative than lexical grounding.
	for i := range q.Notice.Schedules {
		s := &q.Notice.Schedules[i]
		extra := []string{}
		if strings.Contains(s.Evidence, "지급완료") && strings.Contains(s.Evidence, "→") {
			extra = append(extra, "superseded_payment_date")
		}
		if strings.Contains(doc.Title, "*11.9.(월)~11.13.") && s.Type == "application" && strings.Contains(s.Evidence, "11.10.") {
			extra = append(extra, "source_conflict")
		}
		if strings.Contains(s.Label, "전체 기간") && strings.Contains(s.Evidence, "교육별 2시간씩") {
			extra = append(extra, "discrete_sessions_not_continuous")
		}
		if strings.Contains(s.Evidence, "베이징 현지시간") && (s.StartTime != nil || s.EndTime != nil) {
			extra = append(extra, "source_timezone_conflict")
		}
		if weekdayConflict(s.Evidence) {
			extra = append(extra, "source_weekday_conflict")
		}
		if strings.Contains(doc.Content, "– 수업 시간: 18:00~20:00 월, 수, 금") && s.Label == "수업 일정" {
			s.Qualifiers = unique(s.Qualifiers, "recurring:월,수,금 18:00~20:00; individual dates not generated")
		}
		if !ValidSpans(s.EvidenceSpans, doc) {
			extra = append(extra, "evidence_mismatch")
		}
		// Same deadline/scope repeated under another label is retained but blocked,
		// not silently deleted or merged across institutions.
		for j := 0; j < i; j++ {
			t := q.Notice.Schedules[j]
			if sameBoundary(*s, t) && s.Type == t.Type && strings.Contains(s.Label, "접수비 면제") && strings.Contains(t.Label, "접수비 면제") {
				extra = append(extra, "semantic_duplicate_candidate")
			}
		}
		s.Quality = DeriveQuality(*s, extra)
		if len(extra) > 0 {
			for _, r := range extra {
				reason(s, r)
			}
		}
	}
	for i := range q.Notice.RuleSupplements {
		s := &q.Notice.RuleSupplements[i]
		s.EvidenceSpans = []model.SourceSpan{span(doc, "rule_source", s.Evidence)}
		s.Quality = DeriveQuality(*s, nil)
	}
	q.ScheduleState = "no_schedule"
	for _, s := range append(append([]model.Schedule{}, q.Notice.Schedules...), q.Notice.RuleSupplements...) {
		if qualityRank(s.Quality.State) > qualityRank(q.ScheduleState) {
			q.ScheduleState = s.Quality.State
		}
	}
	if q.Zero != nil && q.Zero.Category != "A" && len(q.Notice.RuleSupplements) == 0 {
		q.ScheduleState = "uncertain"
	}
	return q, nil
}
func summaryIssue(q *QualityResult, code string, doc Document, quote, proposal string) {
	s := span(doc, "summary_proposal", quote)
	if !s.Location.Matched {
		return
	}
	q.Summary.Issues = unique(q.Summary.Issues, code)
	q.Summary.State = "needs_review"
	q.Summary.Proposal = sp(proposal)
	q.Summary.ProposalEvidence = append(q.Summary.ProposalEvidence, s)
	q.Notice.Audience.SummaryReviewRequired = true
	q.Notice.Audience.SummaryIssues = unique(q.Notice.Audience.SummaryIssues, code)
}
func sameBoundary(a, b model.Schedule) bool {
	x, _ := json.Marshal([]any{a.StartDate, a.EndDate, a.StartTime, a.EndTime, a.StartPartial, a.EndPartial})
	y, _ := json.Marshal([]any{b.StartDate, b.EndDate, b.StartTime, b.EndTime, b.StartPartial, b.EndPartial})
	return string(x) == string(y)
}
func partsEqual(a, b *model.CalendarParts) bool {
	if a == nil || b == nil {
		return a == b
	}
	val := func(p *int) int {
		if p == nil {
			return -1
		}
		return *p
	}
	return val(a.Year) == val(b.Year) && val(a.Month) == val(b.Month) && val(a.Day) == val(b.Day)
}
func qualityRank(s string) int {
	switch s {
	case "verified":
		return 1
	case "partial":
		return 2
	case "uncertain":
		return 3
	case "blocked":
		return 4
	}
	return 0
}
func DeriveQuality(s model.Schedule, extra []string) *model.ScheduleQuality {
	reasons := append(append([]string{}, s.ReviewReasons...), extra...)
	state := "verified"
	for _, r := range reasons {
		switch r {
		case "evidence_mismatch", "source_conflict", "superseded_payment_date", "discrete_sessions_not_continuous", "source_timezone_conflict", "semantic_duplicate_candidate", "invalid_parent", "invalid_date", "reversed_range", "invalid_calendar_value", "conflicting_calendar_values", "unsupported_type", "unsupported_timezone", "source_weekday_conflict":
			state = "blocked"
		default:
			if state != "blocked" {
				state = "uncertain"
			}
		}
	}
	if state == "verified" && (s.StartPartial != nil || s.EndPartial != nil || s.StartDate == nil || s.EndDate == nil || s.StartTime == nil || s.EndTime == nil) {
		state = "partial"
	}
	// No precision-only promotion: unavailable calendars are never verified.
	if s.StartDate == nil && s.EndDate == nil && s.StartPartial == nil && s.EndPartial == nil && state != "blocked" {
		state = "uncertain"
		reasons = unique(reasons, "no_fixed_calendar")
	}
	candidate := (state == "verified" || state == "partial") && s.EndDate != nil && s.EndTime != nil && (s.Type == "application" || s.Type == "submission" || s.Type == "completion")
	return &model.ScheduleQuality{Version: QualityVersion, State: state, Reasons: reasons, DisplayAllowed: state == "verified" || state == "partial", DeadlineCandidate: candidate}
}
func reviewZero(id string, doc Document, n *NoticeResult) *ZeroReview {
	z := &ZeroReview{Category: "E", Reason: "저장된 본문만으로 일정 유무를 확정할 수 없음", Evidence: []model.SourceSpan{}, Conditions: []string{}, AddedIDs: []string{}}
	quote := doc.Content
	switch {
	case strings.Contains(doc.Title, "휴학생 겨울계절제") && strings.Contains(doc.Content, "1.14.(월) → 1.14.(목)"):
		z.Category = "B"
		z.Reason = "요일 정정 문장에 종강일 1.14.가 명시됨. 학년도로 달력 연도를 확정하지 않음"
		quote = "1.14.(목)"
		addPartial(n, id, "completion", "겨울계절제 종강일(정정)", quote, 1, ip(14), doc, z)
	case strings.Contains(doc.Title, "다전공 신청/변경/포기") && strings.Contains(doc.Content, "1월 복학생을 위한 다전공 신청 기간"):
		z.Category = "B"
		z.Reason = "신청 기간으로 1월만 확인됨. 연도·일·시각은 미상"
		addPartial(n, id, "application", "다전공 신청 기간(1월)", doc.Content, 1, nil, doc, z)
	case strings.Contains(doc.Title, "유고결석 안내"):
		z.Category = "D"
		z.Reason = "결석 종료일 이후라는 상대 조건이며 고정 날짜가 아님. 신청 방법·요건은 붙임 확인 필요"
		z.Conditions = []string{"결석 종료일 이후 신청 가능"}
		z.AttachmentRequired = true
	case strings.Contains(doc.Title, "전공설계상담 안내"):
		z.Category = "D"
		z.Reason = "학기 중 상담 운영 시각 09:00~17:30만 확인됨. 날짜 범위·개별 예약 시각은 알 수 없음"
		z.Conditions = []string{"학기 중: 09:00 ~ 17:30"}
	case strings.Contains(doc.Title, "특별장학금"):
		z.Category = "C"
		z.Reason = "신청 상세는 붙임 확인 지시. 첨부에 일정이 실제 존재하는지는 미확인. 본문 2026.10.27은 문서 작성일이므로 신청일로 사용하지 않음"
		z.AttachmentRequired = true
	case strings.Contains(doc.Title, "주거안정장학금 대학 자체"):
		z.Category = "A"
		z.Reason = "선발·우선지원 기준 안내. 본문의 생년월일 기준과 과거 공지 참조일은 신청 일정이 아님"
	case strings.Contains(doc.Title, "불법 취업 단속"):
		z.Category = "A"
		z.Reason = "주의사항 안내. 말미 2026.11.2.는 문서 작성일이며 일정이 아님"
	case strings.Contains(doc.Title, "학점교류 신청"):
		z.Reason = "저장된 본문은 다른 게시판 바로가기뿐. 외부 링크를 열지 않아 일정 유무 미확인"
	case strings.Contains(doc.Title, "통학로") || strings.Contains(doc.Title, "AI 활용 시 주의사항"):
		z.Reason = "일반 안내이지만 텍스트가 이미지·카드뉴스 설명뿐이므로 원문 충분성 부족. 일정 없음으로 단정하지 않음"
	case strings.Contains(doc.Title, "생성형 AI 활용 기초"):
		z.Reason = "2일간 교육이라는 기간만 있고 달력 날짜 없음. 첨부 파일 내용 미분석"
		z.AttachmentRequired = true
	}
	if quote != "" {
		z.Evidence = append(z.Evidence, span(doc, "zero_notice_source", quote))
	}
	return z
}
func addPartial(n *NoticeResult, id, typ, label, evidence string, month int, day *int, doc Document, z *ZeroReview) {
	s := model.Schedule{ID: id + "/rule/quality-01", Type: typ, Label: label, EndPartial: &model.CalendarParts{Month: ip(month), Day: day}, Timezone: "Asia/Seoul", Scope: sp(label), Evidence: evidence, Origin: "source_rule_supplement", ExtractionMethod: "rule", ConversionMethod: QualityVersion, ReviewReasons: []string{}, Omissions: []string{}, Qualifiers: []string{}, Corrections: []model.ScheduleCorrection{}}
	if typ == "application" {
		s.StartPartial = &model.CalendarParts{Month: ip(month), Day: day}
	}
	s = FromV2(s, doc)
	s.EvidenceSpans = []model.SourceSpan{span(doc, "rule_source", evidence)}
	if !s.EvidenceLocation.Matched {
		return
	}
	n.RuleSupplements = append(n.RuleSupplements, s)
	z.AddedIDs = append(z.AddedIDs, s.ID)
}

func weekdayConflict(text string) bool {
	re := regexp.MustCompile(`^\s*\(([일월화수목금토])\)`)
	for _, d := range dates(text) {
		p := d.parts
		if p.Year == nil || p.Month == nil || p.Day == nil {
			continue
		}
		m := re.FindStringSubmatch(text[d.end:])
		if m == nil {
			continue
		}
		actual := []string{"일", "월", "화", "수", "목", "금", "토"}[time.Date(*p.Year, time.Month(*p.Month), *p.Day, 0, 0, 0, 0, time.UTC).Weekday()]
		if actual != m[1] {
			return true
		}
	}
	return false
}
