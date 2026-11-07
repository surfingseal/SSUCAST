package service

import (
	"context"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"time"
)

type SSUTransferReport struct {
	Database           string        `json:"database"`
	SnapshotDocuments  int           `json:"snapshot_documents"`
	Inserted           int           `json:"inserted"`
	Existing           int           `json:"existing"`
	Verified           int           `json:"verified"`
	SourceID           bson.ObjectID `json:"source_id"`
	Status             string        `json:"status"`
	ObjectIDsPreserved bool          `json:"object_ids_preserved"`
	CrawledAtPreserved bool          `json:"crawled_at_preserved"`
	AIRequests         int           `json:"vertex_requests"`
	SchoolRequests     int           `json:"school_http_requests"`
}

// TransferSSUSnapshot is an explicitly invoked import, never part of cron/crawl.
// Conflicting bodies and identities abort the entire plan before content writes.
// A retry preserves existing target IDs and never deletes target history.
func TransferSSUSnapshot(ctx context.Context, local, target *repository.Store, targetDB string) (SSUTransferReport, error) {
	r := SSUTransferReport{Database: targetDB, Status: "preflight"}
	items, e := local.Contents(ctx, model.Filter{IncludeCompleted: true, IncludeExpired: true, SourceName: "ssu_catch", Page: 1, Limit: 10000})
	if e != nil {
		return r, e
	}
	r.SnapshotDocuments = len(items)
	if len(items) == 0 {
		return r, fmt.Errorf("empty local snapshot")
	}
	source, e := local.Source(ctx, items[0].SourceID)
	if e != nil {
		return r, e
	}
	if source.Name != "ssu_catch" || source.ClassificationMode() != "source" {
		return r, fmt.Errorf("official SSU source snapshot required")
	}
	targetSources, e := target.Sources(ctx)
	if e != nil {
		return r, e
	}
	sourceExists := false
	for _, s := range targetSources {
		if s.Name == source.Name {
			source = s
			sourceExists = true
			break
		}
	}
	if source.ClassificationMode() != "source" {
		return r, fmt.Errorf("target Source classification policy conflict")
	}
	contents := target.Client.Database(targetDB).Collection("crawled_contents")
	sources := target.Client.Database(targetDB).Collection("crawl_sources")
	if !sourceExists {
		var clash model.Source
		e = sources.FindOne(ctx, bson.M{"_id": source.ID}).Decode(&clash)
		if e == nil {
			return r, fmt.Errorf("Source ObjectID collision")
		}
		if e != mongo.ErrNoDocuments {
			return r, e
		}
	}
	urls := make([]string, 0, len(items))
	ids := make([]bson.ObjectID, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		if item.ID.IsZero() || item.Title == "" || item.Content == "" || item.PublishedAt == nil || model.IsExcludedNotice(item) || item.ClassificationMethod != "source" || !model.IsOfficialCategory(item.Category) || item.ContentHash != model.ContentDigest(item) {
			return r, fmt.Errorf("invalid source snapshot")
		}
		if seen[item.OriginalURL] {
			return r, fmt.Errorf("duplicate source URL")
		}
		seen[item.OriginalURL] = true
		urls = append(urls, item.OriginalURL)
		ids = append(ids, item.ID)
	}
	cur, e := contents.Find(ctx, bson.M{"$or": bson.A{bson.M{"original_url": bson.M{"$in": urls}}, bson.M{"_id": bson.M{"$in": ids}}}})
	if e != nil {
		return r, e
	}
	var existing []model.Content
	if e = cur.All(ctx, &existing); e != nil {
		return r, e
	}
	byURL := map[string]model.Content{}
	byID := map[bson.ObjectID]model.Content{}
	for _, v := range existing {
		byURL[v.OriginalURL] = v
		byID[v.ID] = v
	}
	newDocs := []any{}
	originalIDs := map[string]bson.ObjectID{}
	originalCrawled := map[string]time.Time{}
	for i := range items {
		v := &items[i]
		v.SourceID = source.ID
		if old, ok := byURL[v.OriginalURL]; ok {
			if old.ContentHash != v.ContentHash || old.SourceID != source.ID {
				return r, fmt.Errorf("existing target document conflict; no content changed")
			}
			v.ID = old.ID
			v.CrawledAt = old.CrawledAt
			if old.LastSeenAt.After(v.LastSeenAt) {
				v.LastSeenAt = old.LastSeenAt
			}
			r.Existing++
		} else {
			if _, ok := byID[v.ID]; ok {
				return r, fmt.Errorf("target document ObjectID collision")
			}
			newDocs = append(newDocs, *v)
		}
		originalIDs[v.OriginalURL] = v.ID
		originalCrawled[v.OriginalURL] = v.CrawledAt
	}
	no := false
	if !sourceExists {
		source.Enabled = false
		source.AutoEnabled = &no
		if e = target.SourceRepository.ApplySources(ctx, []model.Source{source}); e != nil {
			return r, e
		}
	}
	r.SourceID = source.ID
	r.Status = "writing"
	if len(newDocs) > 0 {
		result, e := contents.InsertMany(ctx, newDocs)
		if e != nil {
			return r, e
		}
		r.Inserted = len(result.InsertedIDs)
	}
	// Repository normalization, memberships, hash/CAS policy are reused.
	if e = target.Save(ctx, items); e != nil {
		return r, e
	}
	cur, e = contents.Find(ctx, bson.M{"original_url": bson.M{"$in": urls}})
	if e != nil {
		return r, e
	}
	var saved []model.Content
	if e = cur.All(ctx, &saved); e != nil {
		return r, e
	}
	if len(saved) != len(items) {
		return r, fmt.Errorf("target count verification failed")
	}
	expected := map[string]model.Content{}
	for _, v := range items {
		expected[v.OriginalURL] = v
	}
	for _, v := range saved {
		want := expected[v.OriginalURL]
		if v.ID != originalIDs[v.OriginalURL] || !v.CrawledAt.Equal(originalCrawled[v.OriginalURL]) || v.ContentHash != want.ContentHash || v.Title != want.Title || v.Content != want.Content || v.Category != want.Category || v.ClassificationMethod != "source" {
			return r, fmt.Errorf("target field/identity verification failed")
		}
	}
	r.Verified = len(saved)
	r.ObjectIDsPreserved = true
	r.CrawledAtPreserved = true
	r.Status = "completed"
	return r, nil
}
