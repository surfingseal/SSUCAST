package web

import (
	"context"
	"example.com/crawlserver/internal/predictions"
	"fmt"
	"net/http"
	"time"
)

type predictionReader interface {
	NoticePredictions(context.Context, string, string) ([]predictions.Prediction, error)
}

func (s *Server) predictionAPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	for k, v := range q {
		if (k != "from" && k != "to") || len(v) != 1 {
			send(w, 400, map[string]string{"error": "예상 공고 조회 조건이 올바르지 않습니다"})
			return
		}
	}
	today := s.now().In(predictions.Seoul)
	from := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, predictions.Seoul)
	to := predictions.AddMonths(from, 3)
	var e error
	if len(q) > 0 {
		from, e = predictions.ParseDate(q.Get("from"))
		if e == nil {
			to, e = predictions.ParseDate(q.Get("to"))
		}
		if e != nil || to.Before(from) || to.Sub(from) > 123*24*time.Hour || from.Year() < 1970 || to.Year() > 2100 {
			send(w, 400, map[string]string{"error": "유효한 from/to 날짜와 124일 이내 범위가 필요합니다"})
			return
		}
	}
	reader, ok := s.Store.(predictionReader)
	if !ok {
		s.failure(w, fmt.Errorf("synthetic reader unavailable"))
		return
	}
	items, e := reader.NoticePredictions(r.Context(), from.Format("2006-01-02"), to.Format("2006-01-02"))
	if e != nil {
		s.failure(w, e)
		return
	}
	send(w, 200, map[string]any{"items": items, "from": from.Format("2006-01-02"), "to": to.Format("2006-01-02"), "data_origin": "ai_prediction", "disclaimer": "AI 예상 공고는 확정 모집 일정이 아닙니다. 각 공고의 근거와 예상 기간을 확인해 주세요."})
}
