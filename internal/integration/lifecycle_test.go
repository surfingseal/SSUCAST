package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/lifecycle"
	"example.com/crawlserver/internal/model"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLifecyclePreviewApplyCASAPIAndBackup(t *testing.T) {
	store := strategyStore(t)
	db := store.Client.Database(sourcesDatabaseName(t, store))
	ctx := context.Background()
	now := time.Now().UTC()
	source := model.Source{ID: bson.NewObjectID(), Name: "lifecycle_test", ClassificationStrategy: "source", BaseURL: "https://fixture.invalid"}
	if e := store.SourceRepository.ApplySources(ctx, []model.Source{source}); e != nil {
		t.Fatal(e)
	}
	pub := now.AddDate(0, 0, -10)
	past := now.In(lifecycle.Seoul).AddDate(0, 0, -1).Format("2006.01.02")
	future := now.In(lifecycle.Seoul).AddDate(0, 0, 10).Format("2006.01.02")
	items := []model.Content{{SourceID: source.ID, Title: "신청 안내", Content: "신청마감: " + past, OriginalURL: "https://fixture.invalid/expired", OriginalCategory: "학사", PublishedAt: &pub}, {SourceID: source.ID, Title: "신청 안내", Content: "접수마감: " + future, OriginalURL: "https://fixture.invalid/active", OriginalCategory: "학사", PublishedAt: &pub}}
	if e := store.Save(ctx, items); e != nil {
		t.Fatal(e)
	}
	snapshots, before, e := lifecycle.Read(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	preview := lifecycle.Preview(snapshots, now)
	if preview.Expired != 1 || preview.Active != 1 || preview.Confirmed != 2 {
		t.Fatal(preview)
	}
	_, after, e := lifecycle.Read(ctx, db)
	if e != nil || before != after {
		t.Fatal("Preview wrote to DB", e)
	}
	applied, e := lifecycle.Apply(ctx, db, snapshots, preview)
	if e != nil || applied.Applied != 2 {
		t.Fatal("apply/CAS", applied, e)
	}
	all, e := store.Contents(ctx, model.Filter{IncludeExpired: true, Page: 1, Limit: 10})
	if e != nil || len(all) != 2 {
		t.Fatal(all, e)
	}
	for _, v := range all {
		for _, old := range snapshots {
			if old.Content.ID == v.ID {
				if old.Content.ContentHash != v.ContentHash || old.Content.OriginalURL != v.OriginalURL || old.Content.SourceID != v.SourceID || !old.Content.CrawledAt.Equal(v.CrawledAt) || !old.Content.UpdatedAt.Equal(v.UpdatedAt) || !old.Content.PublishedAt.Equal(*v.PublishedAt) || v.ClassificationMethod != "source" {
					t.Fatal("immutable fields changed")
				}
			}
		}
	}
	handler := (api.API{Store: store, Token: "life-test-token"}).Handler()
	expiredID := ""
	for _, v := range all {
		if v.LifecycleStatus == "expired" {
			expiredID = v.ID.Hex()
		}
	}
	for _, tc := range []struct {
		path          string
		status, count int
	}{{"/api/v1/contents", 200, 1}, {"/api/v1/contents?include_expired=true", 200, 2}, {"/api/v1/contents?lifecycle_status=expired", 200, 1}, {"/api/v1/contents?include_expired=maybe", 400, 0}, {"/api/v1/contents/" + expiredID, 404, 0}, {"/api/v1/contents/" + expiredID + "?include_expired=true", 200, 0}, {"/api/v1/contents/" + expiredID + "/classifications", 404, 0}, {"/api/v1/contents/" + expiredID + "/classifications?include_expired=true", 200, 0}} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Authorization", "Bearer life-test-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatal(tc.path, w.Code, w.Body.String())
		}
		if tc.count > 0 {
			var out struct {
				Items []model.Content `json:"items"`
			}
			if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil || len(out.Items) != tc.count {
				t.Fatal(tc.path, w.Body.String(), e)
			}
		}
	}

	expiredObjectID, _ := bson.ObjectIDFromHex(expiredID)
	rawExpired, e := db.Collection("crawled_contents").FindOne(ctx, bson.M{"_id": expiredObjectID}).Raw()
	if e != nil || rawExpired.Lookup("deadline").Type != bson.TypeDateTime {
		t.Fatal("deadline must be UTC BSON Date", e)
	}
	// A persisted active assessment becomes expired by its confirmed date alone.
	if _, e = db.Collection("crawled_contents").UpdateOne(ctx, bson.M{"_id": expiredObjectID}, bson.M{"$set": bson.M{"lifecycle_status": "active", "lifecycle.status": "active"}}); e != nil {
		t.Fatal(e)
	}
	visible, e := store.Contents(ctx, model.Filter{Page: 1, Limit: 10})
	if e != nil || len(visible) != 1 {
		t.Fatal("elapsed active deadline leaked through API filter", visible, e)
	}
	// Linked classification history protects the parent from cleanup.
	historyID := bson.NewObjectID()
	if _, e = db.Collection("content_classifications").InsertOne(ctx, bson.M{"_id": historyID, "content_id": expiredObjectID, "input_hash": "synthetic", "taxonomy_version": "fixture", "model_version": "mock-only"}); e != nil {
		t.Fatal(e)
	}
	linked, e := lifecycle.PlanCleanup(ctx, db, now)
	if e != nil || len(linked.Candidates) != 0 {
		t.Fatal("referenced content selected for deletion", linked, e)
	}
	if _, e = db.Collection("content_classifications").UpdateOne(ctx, bson.M{"_id": historyID}, bson.M{"$set": bson.M{"content_id": bson.NewObjectID()}}); e != nil {
		t.Fatal(e)
	}
	plan, e := lifecycle.PlanCleanup(ctx, db, now)
	if e != nil || len(plan.Candidates) != 1 {
		t.Fatal(plan, e)
	}
	backupPath := filepath.Join(t.TempDir(), "verified-backup")
	manifest, e := lifecycle.Backup(plan, backupPath)
	if e != nil || len(manifest.CandidateIDs) != 1 {
		t.Fatal(manifest, e)
	}
	for file, hash := range manifest.Files {
		b, e := os.ReadFile(filepath.Join(backupPath, file))
		if e != nil || fmt.Sprintf("%x", sha256.Sum256(b)) != hash {
			t.Fatal("backup hash", e)
		}
	}

	contentBackup, e := os.ReadFile(filepath.Join(backupPath, "contents.extjsonl"))
	if e != nil {
		t.Fatal(e)
	}
	var restored bson.D
	if e = bson.UnmarshalExtJSON(bytes.TrimSpace(contentBackup), true, &restored); e != nil {
		t.Fatal("backup cannot be restored", e)
	}
	restoredRaw, e := bson.Marshal(restored)
	if e != nil || !bytes.Equal(restoredRaw, plan.Snapshots[0].Raw) {
		t.Fatal("backup BSON differs from original", e)
	}
	// Any concurrent metadata change defeats the complete-snapshot delete CAS.
	if _, e = db.Collection("crawled_contents").UpdateOne(ctx, bson.M{"_id": plan.Snapshots[0].Content.ID}, bson.M{"$set": bson.M{"metadata": bson.M{"concurrent": "changed"}}}); e != nil {
		t.Fatal(e)
	}
	count, e := db.Collection("crawled_contents").CountDocuments(ctx, lifecycle.SnapshotFilter(plan.Snapshots[0]))
	if e != nil || count != 0 {
		t.Fatal("changed record matched delete CAS", count, e)
	}
	// Extending the body invalidates the old expired assessment, never the hash policy.
	var expired model.Content
	for _, v := range all {
		if v.ID.Hex() == expiredID {
			expired = v
		}
	}
	expired.Content = "신청마감 연장: " + future
	expired.UpdatedAt = now.Add(time.Second)
	expired.ContentHash = ""
	if e = store.Save(ctx, []model.Content{expired}); e != nil {
		t.Fatal(e)
	}
	current, e := store.Content(ctx, expired.ID)
	if e != nil || current.Deadline != nil || current.LifecycleStatus != "needs_review" {
		t.Fatal("stale expiry survived updated body", current, e)
	}
	fresh, _, e := lifecycle.Read(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	next := lifecycle.Preview(fresh, now)
	if next.Expired != 0 || next.Active != 2 {
		t.Fatal(next)
	}
	// Stale assessments cannot overwrite a newly edited record.
	result, e := lifecycle.Apply(ctx, db, snapshots, preview)
	if e != nil || result.ChangedSkipped != 2 {
		t.Fatal("stale apply accepted", result, e)
	}
}
