package predictions

import (
	"example.com/crawlserver/internal/model"
	"testing"
	"time"
)

func forecastFixture() (HistoryRun, []HistoricalNotice, ForecastProposal) {
	d, _ := ParseDate("2025-11-01")
	h := HistoryRun{ID: "history", From: "2025-11-01", To: "2026-02-28", Completed: true, Collected: 1}
	v := model.Content{Title: "2025 겨울 장학금", Category: "scholarship", Content: "신청 마감은 2025.11.21.입니다. 성적증명서를 준비하세요.", ContentHash: "hash", OriginalURL: "https://scatch.ssu.ac.kr/%EA%B3%B5%EC%A7%80%EC%82%AC%ED%95%AD/?slug=old", PublishedAt: &d, Metadata: map[string]string{"data_origin": "http"}}
	v.ContentHash = model.ContentDigest(v)
	r, _ := HistoryRecord(h, v)
	p := ForecastProposal{HistoricalID: r.ID, HistoricalDate: "2025-11-21", Quote: "신청 마감은 2025.11.21.입니다.", Title: "2026 겨울 장학금", Summary: "겨울 장학금 신청 기회 예상", Reason: "작년 1건에 근거한 추정입니다.", Preparation: "성적증명서를 준비하세요."}
	return h, []HistoricalNotice{r}, p
}
func TestForecastEvidenceAndWindow(t *testing.T) {
	h, src, p := forecastFixture()
	rows, e := BuildForecasts("run", "2026-11-01", "2027-02-28", h, src, []ForecastProposal{p}, time.Now())
	if e != nil || len(rows) != 1 {
		t.Fatal(e)
	}
	r := rows[0]
	if r.Date != "2026-11-21" || r.ExpectedFrom != "2026-11-14" || r.ExpectedTo != "2026-11-28" || r.DataOrigin != ForecastOrigin || r.Evidence.URL != src[0].Content.OriginalURL {
		t.Fatal(r)
	}
	if Validate(r) != nil {
		t.Fatal("roundtrip")
	}
	r.Summary = "tamper"
	if Validate(r) == nil {
		t.Fatal("hash tamper")
	}
}
func TestForecastRejectsInventedDatesAndSyntheticSources(t *testing.T) {
	h, src, p := forecastFixture()
	for _, change := range []func(*ForecastProposal){func(v *ForecastProposal) { v.HistoricalID = "foreign" }, func(v *ForecastProposal) { v.HistoricalDate = "2025-11-22" }, func(v *ForecastProposal) { v.Quote = "마감 2025.11.21. 허구" }, func(v *ForecastProposal) { v.HistoricalDate = "2024-11-21" }} {
		v := p
		change(&v)
		if _, e := BuildForecasts("run", "2026-11-01", "2027-02-28", h, src, []ForecastProposal{v}, time.Now()); e == nil {
			t.Fatal(v)
		}
	}
	h.Completed = false
	if _, e := BuildForecasts("run", "2026-11-01", "2027-02-28", h, src, []ForecastProposal{p}, time.Now()); e == nil {
		t.Fatal("incomplete history")
	}
	h.Completed = true
	src[0].DataOrigin = "synthetic"
	if _, e := BuildForecasts("run", "2026-11-01", "2027-02-28", h, src, []ForecastProposal{p}, time.Now()); e == nil {
		t.Fatal("synthetic evidence")
	}
}
func TestForecastQuoteDatesAndYearMatching(t *testing.T) {
	for _, q := range []string{"2025년 11월 21일 마감", "2025.11.21.", "11/21까지", "11.21.(금)", "11월 21일"} {
		if !DateEvidence(q, "2025-11-21") {
			t.Fatal(q)
		}
	}
	if DateEvidence("2024.11.21.", "2025-11-21") || DateEvidence("11월 중순", "2025-11-21") {
		t.Fatal("invented date accepted")
	}
	if ForecastTitleKey("2025 겨울 장학금") != ForecastTitleKey("2026 겨울 장학금") {
		t.Fatal("year match")
	}
}

func TestForecastTitleKeyIgnoresDeadlineAnnotationButPreservesProgram(t *testing.T) {
	if ForecastTitleKey("★2026학년도 관정재단 장학생 선발 공고(~11/19 수 15시)") != ForecastTitleKey("★2027학년도 관정재단 장학생 선발 공고(~11/24 화 15시)") {
		t.Fatal("deadline annotations differ")
	}
	if ForecastTitleKey("2026학년도 제 29기 SISO 참가자 모집") != ForecastTitleKey("2027학년도 제31기 SISO 참가자 모집") {
		t.Fatal("cohort numbering differs")
	}
	if ForecastTitleKey("2026 관정재단 장학생 선발 공고(기한 연장)") != ForecastTitleKey("2027 관정재단 장학생 선발 공고(~11/24 화 15시)") {
		t.Fatal("deadline extension annotation differs")
	}
	if ForecastTitleKey("장학생 선발(외국인)") == ForecastTitleKey("장학생 선발(내국인)") {
		t.Fatal("different programs merged")
	}
}
