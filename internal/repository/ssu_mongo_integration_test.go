package repository

import (
	"context"
	"example.com/crawlserver/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"sync"
	"testing"
	"time"
)

func TestSSUMongoRevisionsAndMemberships(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	id := bson.NewObjectID()
	v := model.Content{SourceID: id, SourceName: "ssu_catch", Title: "장학 안내", Content: "장학금 신청 내용", OriginalURL: "https://scatch.ssu.ac.kr/공지사항/?slug=notice&utm_source=one", OriginalCategory: "장학", Category: "scholarship", Department: "학생지원팀", Status: "진행", CrawledAt: now, LastSeenAt: now, UpdatedAt: now, Attachments: []model.Attachment{{Name: "신청서.hwp", URL: "https://scatch.ssu.ac.kr/download.php?id=1"}}}
	v.ContentHash = model.ContentDigest(v)
	if err := s.Save(ctx, []model.Content{v}); err != nil {
		t.Fatal(err)
	}
	read := func() model.Content {
		t.Helper()
		a, e := s.Contents(ctx, model.Filter{Page: 1, Limit: 20, SourceName: "ssu_catch"})
		if e != nil || len(a) != 1 {
			t.Fatalf("%v %v", a, e)
		}
		return a[0]
	}
	first := read()
	v.OriginalURL = "https://scatch.ssu.ac.kr/공지사항/?utm_medium=two&slug=notice#top"
	v.CrawledAt = now.Add(time.Hour)
	v.LastSeenAt = v.CrawledAt
	v.UpdatedAt = v.CrawledAt
	if err := s.Save(ctx, []model.Content{v}); err != nil {
		t.Fatal(err)
	}
	again := read()
	if again.ID != first.ID || !again.UpdatedAt.Equal(first.UpdatedAt) || !again.CrawledAt.Equal(first.CrawledAt) || !again.LastSeenAt.Equal(v.LastSeenAt) {
		t.Fatal(first, again)
	}
	// Category membership is independent from a content revision; keep scalar compatibility.
	v.Category = "academic"
	v.OriginalCategory = "학사"
	if err := s.Save(ctx, []model.Content{v}); err != nil {
		t.Fatal(err)
	}
	for _, cat := range []string{"academic", "scholarship"} {
		a, e := s.Contents(ctx, model.Filter{Page: 1, Limit: 20, Category: cat})
		if e != nil || len(a) != 1 {
			t.Fatal(cat, a, e)
		}
	}
	v.Content = "변경된 본문 $literal"
	v.Status = "마감"
	v.ContentHash = model.ContentDigest(v)
	v.UpdatedAt = now.Add(2 * time.Hour)
	v.LastSeenAt = v.UpdatedAt
	if err := s.Save(ctx, []model.Content{v}); err != nil {
		t.Fatal(err)
	}
	changed := read()
	if changed.ID != first.ID || changed.Content != v.Content || changed.ContentHash == first.ContentHash || !changed.UpdatedAt.Equal(v.UpdatedAt) || changed.Deadline != nil || len(changed.Attachments) != 1 || len(changed.Categories) != 2 {
		t.Fatal(changed)
	}
	_, err := s.contents.InsertOne(ctx, changed)
	if err == nil {
		t.Fatal("URL uniqueness not enforced")
	}
	t.Logf("insert=1 unchanged_url_count=1 changed_url_count=1 categories=%v last_seen_advanced=true _id_preserved=true", changed.Categories)
}
func TestMongoConcurrentIdenticalUpserts(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	v := model.Content{SourceID: bson.NewObjectID(), Title: "same", Content: "same", OriginalURL: "https://example.org/concurrent"}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.Save(ctx, []model.Content{v}) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.contents.CountDocuments(ctx, bson.M{})
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
}
func TestMongoContentLegacyFieldsRemain(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id := bson.NewObjectID()
	source := bson.NewObjectID()
	now := time.Now().UTC().Truncate(time.Millisecond)
	_, err := s.contents.InsertOne(ctx, bson.M{"_id": id, "source_id": source, "title": "legacy", "content": "old", "original_url": "https://example.org/legacy", "category": "academic", "crawled_at": now, "updated_at": now})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Save(ctx, []model.Content{{SourceID: source, Title: "new", Content: "changed", OriginalURL: "https://example.org/legacy", Category: "academic"}}); err != nil {
		t.Fatal(err)
	}
	v, err := s.Content(ctx, id)
	if err != nil || v.ID != id || v.Title != "new" || !v.CrawledAt.Equal(now) {
		t.Fatal(v, err)
	}
}
