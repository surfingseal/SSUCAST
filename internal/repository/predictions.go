package repository

import (
	"context"
	"example.com/crawlserver/internal/predictions"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Read-only, separate collection. Real notice queries/counts never read these rows.
func (s *ContentRepository) NoticePredictions(ctx context.Context, from, to string) ([]predictions.Prediction, error) {
	cur, e := s.contents.Database().Collection("notice_predictions").Find(ctx, bson.M{"data_origin": "synthetic", "event_type": "deadline", "date": bson.M{"$gte": from, "$lte": to}}, options.Find().SetSort(bson.D{{Key: "date", Value: 1}, {Key: "_id", Value: 1}}))
	if e != nil {
		return nil, e
	}
	defer cur.Close(ctx)
	out := []predictions.Prediction{}
	for cur.Next(ctx) {
		var p predictions.Prediction
		if e = cur.Decode(&p); e != nil {
			return nil, e
		}
		if predictions.Validate(p) == nil {
			out = append(out, p)
		}
	}
	return out, cur.Err()
}

// SaveNoticePredictions is used only by explicit manual commands, never by cmd/web.
// Immutable hashes and _id make partial failures safely resumable without overwrite.
func SaveNoticePredictions(ctx context.Context, db *mongo.Database, rows []predictions.Prediction, run bson.M) (int, error) {
	seen := map[string]bool{}
	if len(rows) == 0 || len(rows) > 100 {
		return 0, fmt.Errorf("bounded synthetic records required")
	}
	for _, p := range rows {
		if e := predictions.Validate(p); e != nil {
			return 0, e
		}
		if seen[p.ID] || p.RunID != run["_id"] || p.PayloadHash == "" {
			return 0, fmt.Errorf("synthetic identity conflict")
		}
		seen[p.ID] = true
	}
	if run["data_origin"] != "synthetic" || run["payload_hash"] == "" {
		return 0, fmt.Errorf("generation history required")
	}
	if _, e := db.Collection("notice_prediction_runs").UpdateOne(ctx, bson.M{"_id": run["_id"], "payload_hash": run["payload_hash"]}, bson.M{"$setOnInsert": run}, options.UpdateOne().SetUpsert(true)); e != nil {
		return 0, fmt.Errorf("immutable generation history conflict")
	}
	inserted := 0
	for _, p := range rows {
		r, e := db.Collection("notice_predictions").UpdateOne(ctx, bson.M{"_id": p.ID, "payload_hash": p.PayloadHash}, bson.M{"$setOnInsert": p}, options.UpdateOne().SetUpsert(true))
		if e != nil {
			return inserted, fmt.Errorf("immutable synthetic record conflict")
		}
		inserted += int(r.UpsertedCount)
	}
	return inserted, nil
}
