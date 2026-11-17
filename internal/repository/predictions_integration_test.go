package repository

import (
	"context"
	"example.com/crawlserver/internal/predictions"
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
	"time"
)

func TestMongoSyntheticPredictions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	db := s.contents.Database()
	from, _ := predictions.ParseDate("2026-11-17")
	slots := predictions.Slots(from)[:1]
	rows, err := predictions.Build("test-run", "2026-11-17", "2027-02-17", slots, []predictions.Item{{SlotID: slots[0].ID, Title: "겨울 학습 지원 모집", Category: "scholarship", Summary: "가상 모집 공고입니다.", Reason: "합성 시나리오입니다."}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	run := bson.M{"_id": "test-run", "data_origin": "synthetic", "payload_hash": predictions.Hash(rows)}
	for i, want := range []int{1, 0} {
		n, e := SaveNoticePredictions(ctx, db, rows, run)
		if e != nil || n != want {
			t.Fatalf("attempt %d: %d %v", i, n, e)
		}
	}
	got, e := s.NoticePredictions(ctx, "2026-11-17", "2027-02-17")
	if e != nil || len(got) != 1 {
		t.Fatalf("read %d %v", len(got), e)
	}
	got, e = s.NoticePredictions(ctx, "2026-11-18", "2027-02-17")
	if e != nil || len(got) != 0 {
		t.Fatal("range", e)
	}
	rows[0].Title = "변경된 합성 공고"
	rows[0].PayloadHash = predictions.Digest(rows[0])
	if _, e = SaveNoticePredictions(ctx, db, rows, run); e == nil {
		t.Fatal("immutable row overwritten")
	}
	n, e := s.contents.CountDocuments(ctx, bson.M{})
	if e != nil || n != 0 {
		t.Fatal("real notices modified", n, e)
	}
}
