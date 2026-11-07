package integration_test

import (
	"context"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/service"
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
	"time"
)

func TestSSUSnapshotTransferPreservesIDsAndExistingData(t *testing.T) {
	local := strategyStore(t)
	target := strategyStore(t)
	ctx := context.Background()
	targetDB := sourcesDatabaseName(t, target)
	source := model.Source{ID: bson.NewObjectID(), Name: "ssu_catch", CrawlerType: "ssu_catch", BaseURL: "https://scatch.ssu.ac.kr", ClassificationStrategy: "source"}
	if e := local.SourceRepository.ApplySources(ctx, []model.Source{source}); e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	item := model.Content{SourceID: source.ID, Title: "합성 학사 공지", Content: "합성 본문: 실제 학교 데이터 아님", OriginalURL: "https://scatch.ssu.ac.kr/공지사항/?slug=transfer-test", OriginalCategory: "학사", PublishedAt: &now}
	if e := local.Save(ctx, []model.Content{item}); e != nil {
		t.Fatal(e)
	}
	originals, e := local.Contents(ctx, model.Filter{SourceName: "ssu_catch", Page: 1, Limit: 10})
	if e != nil || len(originals) != 1 {
		t.Fatal(e)
	}
	unrelated := model.Content{SourceID: bson.NewObjectID(), SourceName: "keep", Title: "기존 데이터", Content: "보존", OriginalURL: "https://fixture.invalid/keep", OriginalCategory: "학사"}
	if e := target.Save(ctx, []model.Content{unrelated}); e != nil {
		t.Fatal(e)
	}
	if _, e := target.Client.Database(targetDB).Collection("content_classifications").InsertOne(ctx, bson.M{"content_id": bson.NewObjectID(), "input_hash": "synthetic", "taxonomy_version": "test", "model_version": "mock-only"}); e != nil {
		t.Fatal(e)
	}
	for attempt := 0; attempt < 2; attempt++ {
		r, e := service.TransferSSUSnapshot(ctx, local, target, targetDB)
		if e != nil || r.Verified != 1 || !r.ObjectIDsPreserved || !r.CrawledAtPreserved {
			t.Fatal(r, e)
		}
		if attempt == 0 && r.Inserted != 1 || attempt == 1 && (r.Inserted != 0 || r.Existing != 1) {
			t.Fatal("duplicate import", r)
		}
	}
	saved, e := target.Content(ctx, originals[0].ID)
	if e != nil || saved.ID != originals[0].ID || !saved.CrawledAt.Equal(originals[0].CrawledAt) {
		t.Fatal(saved, e)
	}
	all, e := target.Contents(ctx, model.Filter{Page: 1, Limit: 10})
	if e != nil || len(all) != 2 {
		t.Fatal("existing data lost", all, e)
	}
	history, e := target.Client.Database(targetDB).Collection("content_classifications").CountDocuments(ctx, bson.M{})
	if e != nil || history != 1 {
		t.Fatal("history changed", history, e)
	}
	// A different target body must not be silently overwritten by an old snapshot.
	if _, e := target.Client.Database(targetDB).Collection("crawled_contents").UpdateOne(ctx, bson.M{"_id": saved.ID}, bson.M{"$set": bson.M{"content": "newer target body", "content_hash": "conflict"}}); e != nil {
		t.Fatal(e)
	}
	if _, e := service.TransferSSUSnapshot(ctx, local, target, targetDB); e == nil {
		t.Fatal("conflict ignored")
	}
	kept, e := target.Content(ctx, saved.ID)
	if e != nil || kept.Content != "newer target body" {
		t.Fatal("conflict overwrote target", kept, e)
	}
}
