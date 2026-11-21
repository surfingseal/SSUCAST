package repository

import (
	"context"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/predictions"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Isolated archives are never queried by live notice counts or scheduling.
func SaveNoticeHistory(ctx context.Context, db *mongo.Database, run predictions.HistoryRun, rows []model.Content) error {
	if run.Collected != len(rows) || len(rows) > 10000 {
		return fmt.Errorf("history count mismatch")
	}
	records := make([]predictions.HistoricalNotice, 0, len(rows))
	for _, v := range rows {
		r, e := predictions.HistoryRecord(run, v)
		if e != nil {
			return e
		}
		records = append(records, r)
	}
	pending := run
	pending.Completed = false
	if _, e := db.Collection("notice_history_runs").InsertOne(ctx, pending); e != nil {
		return e
	}
	for _, r := range records {
		if _, e := db.Collection("notice_history").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$setOnInsert": r}, options.UpdateOne().SetUpsert(true)); e != nil {
			return e
		}
	}
	count, e := db.Collection("notice_history").CountDocuments(ctx, bson.M{"run_id": run.ID})
	if e != nil || count != int64(len(rows)) {
		return fmt.Errorf("history readback count mismatch")
	}
	_, e = db.Collection("notice_history_runs").ReplaceOne(ctx, bson.M{"_id": run.ID}, run)
	return e
}
func ReadNoticeHistory(ctx context.Context, db *mongo.Database, id string) (predictions.HistoryRun, []predictions.HistoricalNotice, error) {
	var run predictions.HistoryRun
	if e := db.Collection("notice_history_runs").FindOne(ctx, bson.M{"_id": id, "completed": true}).Decode(&run); e != nil {
		return run, nil, fmt.Errorf("complete historical run required")
	}
	cur, e := db.Collection("notice_history").Find(ctx, bson.M{"run_id": id, "data_origin": "historical_http"}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if e != nil {
		return run, nil, e
	}
	defer cur.Close(ctx)
	var rows []predictions.HistoricalNotice
	if e = cur.All(ctx, &rows); e != nil {
		return run, nil, e
	}
	if len(rows) != run.Collected || len(rows) == 0 {
		return run, nil, fmt.Errorf("historical evidence count mismatch")
	}
	for _, r := range rows {
		v, e := predictions.HistoryRecord(run, r.Content)
		if e != nil || r.ID != v.ID {
			return run, nil, fmt.Errorf("historical evidence invalid")
		}
	}
	return run, rows, nil
}
