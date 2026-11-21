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

// Publish only after every immutable row is written and read back. Readers keep
// the previous complete run throughout a partial write; synthetic demos are not mixed.
func SaveHistoricalForecasts(ctx context.Context, db *mongo.Database, run bson.M, rows []predictions.Prediction) error {
	id, ok := run["_id"].(string)
	if !ok || id == "" || len(rows) == 0 || len(rows) > 1000 {
		return fmt.Errorf("bounded nonempty forecast required")
	}
	seen := map[string]bool{}
	histories := map[string]predictions.HistoryRun{}
	sources := map[string][]predictions.HistoricalNotice{}
	for _, p := range rows {
		if p.DataOrigin != predictions.ForecastOrigin || p.RunID != id || p.PayloadHash == "" || predictions.Validate(p) != nil || seen[p.ID] {
			return fmt.Errorf("invalid forecast record")
		}
		seen[p.ID] = true
		history, exists := histories[p.Evidence.HistoryRunID]
		if !exists {
			h, src, e := ReadNoticeHistory(ctx, db, p.Evidence.HistoryRunID)
			if e != nil {
				return e
			}
			histories[h.ID] = h
			sources[h.ID] = src
			history = h
		}
		source := sources[history.ID]
		proposal := predictions.ForecastProposal{HistoricalID: p.Evidence.ID, HistoricalDate: p.Evidence.Deadline, Quote: p.Evidence.Quote, Title: p.Title, Summary: p.Summary, Reason: p.Reason, Preparation: p.Preparation}
		rebuilt, e := predictions.BuildForecasts(id, p.HorizonFrom, p.HorizonTo, history, source, []predictions.ForecastProposal{proposal}, p.GeneratedAt)
		if e != nil || len(rebuilt) != 1 || rebuilt[0].PayloadHash != p.PayloadHash {
			return fmt.Errorf("forecast source changed")
		}
	}
	if run["payload_hash"] != predictions.Hash(rows) {
		return fmt.Errorf("forecast run hash mismatch")
	}
	if _, e := db.Collection("notice_forecast_runs").UpdateOne(ctx, bson.M{"_id": id, "payload_hash": run["payload_hash"]}, bson.M{"$setOnInsert": run}, options.UpdateOne().SetUpsert(true)); e != nil {
		return e
	}
	for _, p := range rows {
		if _, e := db.Collection("notice_forecasts").UpdateOne(ctx, bson.M{"_id": p.ID, "payload_hash": p.PayloadHash}, bson.M{"$setOnInsert": p}, options.UpdateOne().SetUpsert(true)); e != nil {
			return e
		}
		var read predictions.Prediction
		if e := db.Collection("notice_forecasts").FindOne(ctx, bson.M{"_id": p.ID}).Decode(&read); e != nil || read.PayloadHash != p.PayloadHash || predictions.Validate(read) != nil {
			return fmt.Errorf("forecast readback mismatch")
		}
	}
	_, e := db.Collection("notice_forecast_state").UpdateOne(ctx, bson.M{"_id": "active"}, bson.M{"$set": bson.M{"run_id": id}}, options.UpdateOne().SetUpsert(true))
	return e
}
func (s *ContentRepository) historicalForecasts(ctx context.Context, from, to string) ([]predictions.Prediction, bool, error) {
	db := s.contents.Database()
	var state struct {
		RunID string `bson:"run_id"`
	}
	e := db.Collection("notice_forecast_state").FindOne(ctx, bson.M{"_id": "active"}).Decode(&state)
	if e == mongo.ErrNoDocuments {
		return nil, false, nil
	}
	if e != nil {
		return nil, true, e
	}
	cur, e := db.Collection("notice_forecasts").Find(ctx, bson.M{"run_id": state.RunID, "date": bson.M{"$gte": from, "$lte": to}}, options.Find().SetSort(bson.D{{Key: "date", Value: 1}, {Key: "_id", Value: 1}}))
	if e != nil {
		return nil, true, e
	}
	defer cur.Close(ctx)
	out := []predictions.Prediction{}
	rows := []predictions.Prediction{}
	if e = cur.All(ctx, &rows); e != nil {
		return nil, true, e
	}
	if len(rows) == 0 {
		return out, true, nil
	}
	for _, p := range rows {
		if predictions.Validate(p) != nil {
			return nil, true, fmt.Errorf("stored forecast invalid")
		}
	}
	start, _ := predictions.ParseDate(rows[0].HorizonFrom)
	end, _ := predictions.ParseDate(rows[0].HorizonTo)
	// Exact title/year normalization only. Suppress a forecast when this year's
	// original notice is already present; no AI similarity guess is used.
	actual, e := s.contents.Find(ctx, bson.M{"published_at": bson.M{"$gte": start, "$lt": end.AddDate(0, 0, 1)}})
	if e != nil {
		return nil, true, e
	}
	defer actual.Close(ctx)
	keys := map[string]bool{}
	for actual.Next(ctx) {
		var v model.Content
		if e = actual.Decode(&v); e != nil {
			return nil, true, e
		}
		keys[v.Category+"|"+predictions.ForecastTitleKey(v.Title)] = true
	}
	if e = actual.Err(); e != nil {
		return nil, true, e
	}
	for _, p := range rows {
		if keys[p.Category+"|"+predictions.ForecastTitleKey(p.Evidence.Title)] {
			continue
		}
		out = append(out, p)
	}
	return out, true, nil
}
