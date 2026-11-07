package repository

import (
	"context"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/noticepolicy"
	"example.com/crawlserver/internal/notices"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"testing"
	"time"
)

func TestNoticesIsolatedMongo(t *testing.T) {
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("explicit loopback TEST_MONGODB_URI required")
	}
	name := "ssu_schedule_apply_test_" + bson.NewObjectID().Hex()
	if e := config.ValidateIsolatedTestMongo(uri, name); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, e := OpenReadOnly(ctx, uri, name)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Client.Disconnect(context.Background())
	db := store.Client.Database(name)
	cols, e := db.ListCollectionNames(ctx, bson.M{})
	if e != nil || len(cols) > 0 {
		t.Fatal("not isolated")
	}
	owner := bson.NewObjectID()
	db.Collection("test_control").InsertOne(ctx, bson.M{"_id": owner})
	defer func() {
		n, _ := db.Collection("test_control").CountDocuments(context.Background(), bson.M{"_id": owner})
		if n == 1 && config.ValidateIsolatedTestMongo(uri, name) == nil {
			db.Drop(context.Background())
		}
	}()
	ssuid, mediaid := bson.NewObjectID(), bson.NewObjectID()
	db.Collection("crawl_sources").InsertMany(ctx, []any{model.Source{ID: ssuid, Name: "ssu_catch", CrawlerType: "ssu_catch"}, model.Source{ID: mediaid, Name: "ssu_global_media"}})
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, notices.Seoul)
	end := "2026-11-08"
	clock := "09:00"
	late := "2026-11-09"
	published := now
	confirmed := model.Schedule{Type: "application", EndDate: &end, EndTime: &clock, Timezone: "Asia/Seoul", Quality: &model.ScheduleQuality{State: "partial"}}
	items := []model.Content{}
	for i := 0; i < 5; i++ {
		v := model.Content{ID: bson.NewObjectID(), SourceID: ssuid, SourceName: "ssu_catch", Title: "장학 모집", Content: "본문", Category: "scholarship", Categories: []string{"scholarship"}, PublishedAt: &published, Status: "진행", ScheduleMetadata: model.ScheduleMetadata{Summary: "원문 핵심", ScheduleAnalysisID: "test-analysis", ScheduleInputHash: "test-input", ScheduleModelVersion: "test-model", ScheduleExtractionVersion: "schedule-v2"}}
		switch i {
		case 0:
			v.Title = "임박 신청"
			v.Schedules = []model.Schedule{confirmed}
		case 1:
			v.Title = "나중 신청"
			s := confirmed
			s.EndDate = &late
			v.Schedules = []model.Schedule{s}
		case 2:
			v.Title = "불확실 이미지 신청"
			s := confirmed
			earliest := "2026-11-07"
			futureClock := "23:00"
			s.EndDate = &earliest
			s.EndTime = &futureClock
			s.Origin = "image_ai"
			s.Quality = &model.ScheduleQuality{State: "uncertain"}
			s.ImageEvidence = []model.ImageEvidence{{ImageURL: "https://scatch.ssu.ac.kr/wp-content/uploads/synthetic.png", Text: "합성 이미지 근거"}}
			s.ReviewRequired = true
			v.Schedules = []model.Schedule{s}
		case 3:
			v.Status = "완료"
		case 4:
			v.SourceID = mediaid
			v.SourceName = "ssu_global_media"
			v.Content = "한국어 검색대상"
			event := confirmed
			event.Type = "event"
			v.Schedules = []model.Schedule{event}
		}
		items = append(items, v)
		if _, e = db.Collection("crawled_contents").InsertOne(ctx, v); e != nil {
			t.Fatal(e)
		}
	}
	f := notices.Filter{Page: 1, Limit: 2, Sort: "deadline"}
	page, e := store.Notices(ctx, f, now)
	if e != nil || page.Total != 4 || page.Items[0].Title != "임박 신청" || page.Items[1].Title != "나중 신청" {
		t.Fatal(page, e)
	}
	yes := true
	f.Upcoming = &yes
	page, e = store.Notices(ctx, f, now)
	if e != nil || page.Total != 3 {
		t.Fatal(page, e)
	}
	f = notices.Filter{Page: 1, Limit: 12, Sort: "event"}
	page, e = store.Notices(ctx, f, now)
	if e != nil || page.Items[0].Source != "ssu_global_media" {
		t.Fatal("event ordering", page, e)
	}
	f = notices.Filter{Page: 1, Limit: 12, Sort: "latest", Query: "한국어 검색대상"}
	page, e = store.Notices(ctx, f, now)
	if e != nil || page.Total != 1 || page.Items[0].Source != "ssu_global_media" {
		t.Fatal(page, e)
	}
	f.Query = ".*"
	page, e = store.Notices(ctx, f, now)
	if e != nil || page.Total != 0 {
		t.Fatal("regex injection", e)
	}
	f.Query = ""
	f.Status = "완료"
	page, e = store.Notices(ctx, f, now)
	if e != nil || page.Total != 1 {
		t.Fatal(page, e)
	}
	f.Status = ""
	f.Source = "ssu_global_media"
	page, e = store.Notices(ctx, f, now)
	if e != nil || page.Total != 1 {
		t.Fatal(page, e)
	}
	meta, e := store.NoticeMetadata(ctx)
	if e != nil || meta.StoredCount != 5 || meta.VisibleCount != 4 {
		t.Fatal(meta, e)
	}
	n, e := store.Notice(ctx, items[0].ID, now)
	if e != nil || n.Content != "본문" || !n.Analyzed {
		t.Fatal(n, e)
	}
	past := time.Now().Add(-time.Hour)
	policy := model.NoticePolicy{Version: noticepolicy.Version, ContentHash: items[0].ContentHash, InputHash: items[0].ScheduleInputHash, AnalysisID: items[0].ScheduleAnalysisID, ExpiresAt: &past, Schedules: items[0].Schedules}
	if _, e = db.Collection("crawled_contents").UpdateOne(ctx, bson.M{"_id": items[0].ID}, bson.M{"$set": bson.M{"notice_policy": policy}}); e != nil {
		t.Fatal(e)
	}
	f = notices.Filter{Page: 1, Limit: 12, Sort: "latest"}
	page, e = store.Notices(ctx, f, now)
	if e != nil || page.Total != 3 {
		t.Fatal("expired policy not hidden", page, e)
	}
	f.IncludeExpired = true
	page, e = store.Notices(ctx, f, now)
	if e != nil || page.Total != 4 {
		t.Fatal("explicit recovery failed", page, e)
	}
	db.Collection("crawled_contents").UpdateOne(ctx, bson.M{"_id": items[0].ID}, bson.M{"$set": bson.M{"content_hash": "updated input"}})
	f.IncludeExpired = false
	page, e = store.Notices(ctx, f, now)
	if e != nil || page.Total != 4 {
		t.Fatal("stale expiration policy applied", page, e)
	}

}
