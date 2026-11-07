package imageapply

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/imageanalysis"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/schedules"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"testing"
	"time"
)

func s(v string) *string { return &v }
func synthetic(t *testing.T) imageanalysis.Prepared {
	t.Helper()
	v := model.Content{ID: bson.NewObjectID(), SourceID: bson.NewObjectID(), SourceName: "ssu_catch", Title: "합성 이미지 공고", Content: "포스터 참고", OriginalURL: "https://scatch.ssu.ac.kr/synthetic", ContentHash: "synthetic-hash", ImageURLs: []string{"https://scatch.ssu.ac.kr/wp-content/uploads/synthetic.png"}, ScheduleMetadata: model.ScheduleMetadata{ScheduleAnalysisID: "text-analysis", ScheduleExtractionVersion: "schedule-v2", ScheduleModelVersion: imageanalysis.Model}}
	v.ScheduleInputHash = scheduleapply.InputHash(v)
	im := imageanalysis.ImageInput{URL: v.ImageURLs[0], Index: 0, SHA256: "synthetic-image-hash", Location: "stored_inline_image_urls"}
	o := imageanalysis.Output{Candidates: []imageanalysis.ImageCandidate{{V2Schedule: schedules.V2Schedule{Type: "application", Label: "합성 접수", EndDate: s("2026-11-20"), Timezone: "Asia/Seoul", Precision: "date", ReviewReasons: []string{}}, Source: "image", ImageIndex: 0, EvidenceText: "신청마감:2026년11월20일", Confidence: "high", Notes: []string{}}}, Transcriptions: []imageanalysis.Transcription{{Index: 0, Text: "신청마감:2026년11월20일", Readable: true}}, SummarySupport: []string{}, AudienceSupport: []string{}, Reasons: []string{}}
	raw, _ := json.Marshal(o)
	r := imageanalysis.Result{ID: v.ID.Hex(), Original: v, Images: []imageanalysis.ImageInput{im}, InputHash: imageanalysis.InputHash(imageanalysis.Candidate{Original: v, Images: []imageanalysis.ImageInput{im}}), Model: imageanalysis.Model, Raw: string(raw), At: time.Now().UTC().Truncate(time.Millisecond)}
	p, e := imageanalysis.Prepare(r)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestImageApplyIsolatedMongo(t *testing.T) {
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("explicit loopback temporary Mongo required")
	}
	name := "ssu_schedule_apply_test_" + bson.NewObjectID().Hex()
	if e := config.ValidateIsolatedTestMongo(uri, name); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, e := repository.OpenReadOnly(ctx, uri, name)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Client.Disconnect(context.Background())
	db := store.Client.Database(name)
	names, e := db.ListCollectionNames(ctx, bson.M{})
	if e != nil || len(names) != 0 {
		t.Fatal("not empty test DB")
	}
	owner := bson.NewObjectID()
	db.Collection("test_control").InsertOne(ctx, bson.M{"_id": owner})
	defer func() {
		n, _ := db.Collection("test_control").CountDocuments(context.Background(), bson.M{"_id": owner})
		if n == 1 && config.ValidateIsolatedTestMongo(uri, name) == nil {
			db.Drop(context.Background())
		}
	}()
	p := synthetic(t)
	db.Collection("crawled_contents").InsertOne(ctx, p.Original)
	db.Collection("schedule_analyses").InsertOne(ctx, bson.M{"_id": p.PreviousAnalysisID, "mode": "original_text"})
	plan, e := Preview(ctx, db, p)
	if e != nil || plan.Outcome != "eligible" {
		t.Fatal(plan, e)
	}
	out, e := Apply(ctx, db, p, plan)
	if e != nil || out != "updated" {
		t.Fatal(out, e)
	}
	var now model.Content
	if e = db.Collection("crawled_contents").FindOne(ctx, bson.M{"_id": p.Original.ID}).Decode(&now); e != nil {
		t.Fatal(e)
	}
	if now.Title != p.Original.Title || now.Content != p.Original.Content || now.ContentHash != p.Original.ContentHash || !now.ImageAnalysisUsed || len(now.Schedules) != 1 || now.Schedules[0].EndTime != nil || len(now.Schedules[0].ImageEvidence) != 1 {
		t.Fatal("BSON projection failed", now)
	}
	plan, e = Preview(ctx, db, p)
	if e != nil || plan.Outcome != "duplicate" {
		t.Fatal(plan, e)
	}
	out, e = Apply(ctx, db, p, plan)
	if e != nil || out != "duplicate" {
		t.Fatal(out, e)
	}
	n, _ := db.Collection("schedule_analyses").CountDocuments(ctx, bson.M{})
	if n != 2 {
		t.Fatal("old history/duplicate invariant", n)
	}
	p2 := synthetic(t)
	db.Collection("crawled_contents").InsertOne(ctx, p2.Original)
	plan, e = Preview(ctx, db, p2)
	if e != nil {
		t.Fatal(e)
	}
	db.Collection("crawled_contents").UpdateOne(ctx, bson.M{"_id": p2.Original.ID}, bson.M{"$set": bson.M{"content": "concurrently changed"}})
	out, e = Apply(ctx, db, p2, plan)
	if e != nil || out != "cas_conflict" && out != "input_hash_conflict" {
		t.Fatal("CAS not protected", out, e)
	}
	p3 := synthetic(t)
	fresh := p3.Original
	fresh.ScheduleAnalysisID = "newer-analysis"
	db.Collection("crawled_contents").InsertOne(ctx, fresh)
	plan, e = Preview(ctx, db, p3)
	if e != nil || plan.Outcome != "analysis_state_conflict" {
		t.Fatal("newer projection overwritten", plan, e)
	}
}

func TestEmptyScheduleSnapshotJSONCompatibility(t *testing.T) {
	a := model.Content{Schedules: []model.Schedule{}}
	raw, _ := json.Marshal(a)
	var b model.Content
	if e := json.Unmarshal(raw, &b); e != nil {
		t.Fatal(e)
	}
	if !sameProjection(a, b) {
		t.Fatal("JSON omitted empty schedules caused false conflict")
	}
	b.Summary = "different analysis"
	if sameProjection(a, b) {
		t.Fatal("real analysis mutation accepted")
	}
}
