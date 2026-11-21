package repository

import (
	"context"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/predictions"
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
	"time"
)

func TestMongoHistoricalForecastPublishIsolationAndActualReplacement(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	db := s.contents.Database()
	d, _ := predictions.ParseDate("2025-11-01")
	h := predictions.HistoryRun{ID: "archive", From: "2025-11-01", To: "2026-02-28", Completed: true, Collected: 1, At: time.Now()}
	v := model.Content{Title: "2025 겨울 장학금(기한 연장)", Category: "scholarship", Content: "신청 마감은 2025.11.21.입니다. 성적증명서를 준비하세요.", OriginalURL: "https://scatch.ssu.ac.kr/%EA%B3%B5%EC%A7%80%EC%82%AC%ED%95%AD/?slug=history", PublishedAt: &d, ContentHash: "hash", Metadata: map[string]string{"data_origin": "http"}}
	v.ContentHash = model.ContentDigest(v)
	if e := SaveNoticeHistory(ctx, db, h, []model.Content{v}); e != nil {
		t.Fatal(e)
	}
	_, src, e := ReadNoticeHistory(ctx, db, h.ID)
	if e != nil {
		t.Fatal(e)
	}
	partial := h
	partial.ID = "incomplete"
	partial.Completed = false
	if e = SaveNoticeHistory(ctx, db, partial, []model.Content{v}); e != nil {
		t.Fatal(e)
	}
	if _, _, e = ReadNoticeHistory(ctx, db, partial.ID); e == nil {
		t.Fatal("incomplete archive allowed")
	}
	p := predictions.ForecastProposal{HistoricalID: src[0].ID, HistoricalDate: "2025-11-21", Quote: "신청 마감은 2025.11.21.입니다.", Title: "2026 겨울 장학금", Summary: "겨울 장학금 예상", Reason: "작년 1건에 근거한 추정입니다.", Preparation: "성적증명서 준비"}
	rows, e := predictions.BuildForecasts("forecast", "2026-11-01", "2027-02-28", h, src, []predictions.ForecastProposal{p}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	run := bson.M{"_id": "forecast", "payload_hash": predictions.Hash(rows)}
	for i := 0; i < 2; i++ {
		if e = SaveHistoricalForecasts(ctx, db, run, rows); e != nil {
			t.Fatal(e)
		}
	}
	got, e := s.NoticePredictions(ctx, "2026-11-01", "2027-02-28")
	if e != nil || len(got) != 1 || got[0].DataOrigin != predictions.ForecastOrigin {
		t.Fatalf("read %d %v", len(got), e)
	}
	if n, _ := s.contents.CountDocuments(ctx, bson.M{}); n != 0 {
		t.Fatal("real notices modified")
	}
	// A real publication for this forecast horizon suppresses the matching forecast.
	actual := v
	actual.ID = bson.NewObjectID()
	actual.Title = "2026 겨울 장학금(~11/24 화)"
	date, _ := predictions.ParseDate("2026-11-18")
	actual.PublishedAt = &date
	if _, e = s.contents.InsertOne(ctx, actual); e != nil {
		t.Fatal(e)
	}
	got, e = s.NoticePredictions(ctx, "2026-11-01", "2027-02-28")
	if e != nil || len(got) != 0 {
		t.Fatal("duplicate real/predicted notice", e)
	}
	// Tampering cannot move the active pointer to a broken run.
	rows[0].RunID = "broken"
	rows[0].Title = "tampered"
	rows[0].PayloadHash = predictions.Digest(rows[0])
	if e = SaveHistoricalForecasts(ctx, db, bson.M{"_id": "broken", "payload_hash": predictions.Hash(rows)}, rows); e == nil {
		t.Fatal("mismatched immutable identity accepted")
	}
}
