package lifecycle

import (
	"context"
	"crypto/sha256"
	"example.com/crawlserver/internal/model"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"time"
)

type Snapshot struct {
	Content  model.Content
	Raw      bson.Raw
	Document bson.D
}
type PreviewItem struct {
	ID     bson.ObjectID             `json:"id"`
	Title  string                    `json:"title"`
	URL    string                    `json:"original_url"`
	Result model.LifecycleEvaluation `json:"lifecycle"`
}
type PreviewReport struct {
	CheckedAt             time.Time     `json:"checked_at"`
	Total                 int           `json:"total"`
	Confirmed             int           `json:"confirmed_deadline"`
	Expired               int           `json:"expired"`
	Active                int           `json:"active"`
	NeedsReview           int           `json:"needs_review"`
	NoDeadline            int           `json:"no_deadline"`
	NullOriginalDeadlines int           `json:"null_original_deadlines"`
	DBChanges             int           `json:"database_changes"`
	SourceSnapshotSHA256  string        `json:"source_snapshot_sha256"`
	ReadAfterSHA256       string        `json:"read_after_sha256"`
	Unchanged             bool          `json:"database_unchanged"`
	Items                 []PreviewItem `json:"items"`
}

func Read(ctx context.Context, db *mongo.Database) ([]Snapshot, string, error) {
	cur, e := db.Collection("crawled_contents").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if e != nil {
		return nil, "", e
	}
	defer cur.Close(ctx)
	out := []Snapshot{}
	sum := sha256.New()
	for cur.Next(ctx) {
		raw := append(bson.Raw{}, cur.Current...)
		var item model.Content
		var doc bson.D
		if e = bson.Unmarshal(raw, &item); e != nil {
			return nil, "", e
		}
		if e = bson.Unmarshal(raw, &doc); e != nil {
			return nil, "", e
		}
		sum.Write(raw)
		out = append(out, Snapshot{item, raw, doc})
	}
	if e = cur.Err(); e != nil {
		return nil, "", e
	}
	return out, fmt.Sprintf("%x", sum.Sum(nil)), nil
}
func Preview(snapshots []Snapshot, now time.Time) PreviewReport {
	r := PreviewReport{CheckedAt: now.UTC(), Total: len(snapshots), Items: []PreviewItem{}}
	for _, s := range snapshots {
		v := s.Content
		e := Extract(v, now)
		if v.Deadline == nil {
			r.NullOriginalDeadlines++
		}
		if e.Confirmed {
			r.Confirmed++
		}
		switch e.Status {
		case "active":
			r.Active++
		case "expired":
			r.Expired++
		case "no_deadline":
			r.NoDeadline++
		default:
			r.NeedsReview++
		}
		r.Items = append(r.Items, PreviewItem{v.ID, v.Title, v.OriginalURL, e})
	}
	return r
}

// Full BSON equality is a CAS: even metadata/last_seen changes reject the write.
func SnapshotFilter(s Snapshot) bson.D {
	return bson.D{{Key: "_id", Value: s.Content.ID}, {Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$$ROOT", bson.D{{Key: "$literal", Value: s.Document}}}}}}}
}

type ApplyReport struct {
	Applied        int `json:"applied"`
	ChangedSkipped int `json:"changed_skipped"`
	Deleted        int `json:"deleted"`
}

func Apply(ctx context.Context, db *mongo.Database, snapshots []Snapshot, preview PreviewReport) (ApplyReport, error) {
	var r ApplyReport
	if len(snapshots) != len(preview.Items) {
		return r, fmt.Errorf("snapshot/evaluation mismatch")
	}
	for i, s := range snapshots {
		e := preview.Items[i].Result
		if s.Content.ID != preview.Items[i].ID || e.Version != Version || e.InputHash != InputHash(s.Content) {
			return r, fmt.Errorf("stale/invalid lifecycle evaluation")
		}
		result, x := db.Collection("crawled_contents").UpdateOne(ctx, SnapshotFilter(s), bson.M{"$set": bson.M{"deadline": e.Deadline, "lifecycle_status": e.Status, "lifecycle": e}})
		if x != nil {
			return r, x
		}
		if result.MatchedCount == 1 {
			r.Applied++
		} else {
			r.ChangedSkipped++
		}
	}
	return r, nil
}
func ConfirmedExpired(v model.Content, now time.Time) bool {
	return v.Lifecycle != nil && v.Lifecycle.Confirmed && v.Deadline != nil && v.Lifecycle.Version == Version && v.Lifecycle.InputHash == InputHash(v) && v.ContentHash != "" && model.ContentDigest(v) == v.ContentHash && (v.LifecycleStatus == "expired" || v.LifecycleStatus == "active") && now.After(*v.Deadline) && v.Lifecycle.Deadline != nil && v.Deadline.Equal(*v.Lifecycle.Deadline)
}
