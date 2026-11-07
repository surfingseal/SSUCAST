package repository

import (
	"context"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/noticepolicy"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"time"
)

// SSUCatchSourceIDs uses authoritative Source records, not a title/domain or a
// caller-supplied source_name label. Missing Source references are not hidden.
func SSUCatchSourceIDs(ctx context.Context, db *mongo.Database) ([]bson.ObjectID, error) {
	cur, e := db.Collection("crawl_sources").Find(ctx, bson.M{"$or": bson.A{bson.M{"crawler_type": "ssu_catch"}, bson.M{"name": "ssu_catch", "crawler_type": bson.M{"$in": bson.A{"", nil}}}}})
	if e != nil {
		return nil, e
	}
	defer cur.Close(ctx)
	ids := []bson.ObjectID{}
	for cur.Next(ctx) {
		var s model.Source
		if e = cur.Decode(&s); e != nil {
			return nil, e
		}
		if s.CrawlerKey() == "ssu_catch" && !s.ID.IsZero() {
			ids = append(ids, s.ID)
		}
	}
	return ids, cur.Err()
}

// Visibility is composed with, never substituted for, lifecycle/category rules.
// Applying it in MongoDB before skip/limit keeps pagination correct.
func (s *ContentRepository) ListVisibilityQuery(ctx context.Context, f model.Filter) (bson.M, error) {
	q := query(f, "crawled_at")
	if !f.IncludeExpired {
		clauses, _ := q["$nor"].(bson.A)
		q["$nor"] = append(clauses, bson.M{"notice_policy.expires_at": bson.M{"$ne": nil, "$lte": time.Now().UTC()}, "$expr": noticePolicyValid()})
	}
	if f.IncludeCompleted {
		return q, nil
	}
	ids, e := SSUCatchSourceIDs(ctx, s.contents.Database())
	if e != nil {
		return nil, e
	}
	if len(ids) == 0 {
		return q, nil
	}
	exclusion := bson.M{"source_id": bson.M{"$in": ids}, "status": "완료"}
	clauses, _ := q["$nor"].(bson.A)
	q["$nor"] = append(clauses, exclusion)
	return q, nil
}

func noticePolicyValid() bson.M {
	return bson.M{"$and": bson.A{bson.M{"$eq": bson.A{"$notice_policy.version", noticepolicy.Version}}, bson.M{"$eq": bson.A{"$notice_policy.content_hash", "$content_hash"}}, bson.M{"$eq": bson.A{"$notice_policy.input_hash", "$schedule_input_hash"}}, bson.M{"$eq": bson.A{"$notice_policy.analysis_id", "$schedule_analysis_id"}}}}
}
