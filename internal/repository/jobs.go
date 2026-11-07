package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"example.com/crawlserver/internal/model"
)

var ErrBusy = errors.New("source already running")

type JobRepository struct {
	jobs             *mongo.Collection
	sourceCollection *mongo.Collection
}

func (s *JobRepository) Start(ctx context.Context, j model.Job) error {
	// A hard job timeout plus a 30-second finalization margin bounds ownership.
	now := time.Now().UTC()
	_, err := s.jobs.UpdateMany(ctx, bson.M{"source_id": j.SourceID, "status": "running", "deadline": bson.M{"$lt": now}}, bson.M{"$set": bson.M{"status": "failed", "finished_at": now, "error_count": 1, "error_message": "job deadline expired; previous process may have stopped"}})
	if err != nil {
		return err
	}
	_, err = s.jobs.InsertOne(ctx, j)
	if mongo.IsDuplicateKeyError(err) {
		return ErrBusy
	}
	return err
}
func (s *JobRepository) Finish(ctx context.Context, j model.Job) error {
	result, err := s.jobs.UpdateOne(ctx, bson.M{"_id": j.ID, "status": "running"}, bson.M{"$set": bson.M{"status": j.Status, "finished_at": j.FinishedAt, "collected_count": j.CollectedCount, "error_count": j.ErrorCount, "error_message": j.ErrorMessage, "error_code": j.ErrorCode, "error_stage": j.ErrorStage}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("job ownership expired")
	}
	_, err = s.sourceCollection.UpdateOne(ctx, bson.M{"_id": j.SourceID}, bson.M{"$set": bson.M{"last_crawled_at": j.FinishedAt, "updated_at": j.FinishedAt}})
	return err
}
func (s *JobRepository) Jobs(ctx context.Context, f model.Filter) ([]model.Job, error) {
	cur, err := s.jobs.Find(ctx, query(f, "started_at"), options.Find().SetSort(bson.D{{Key: "started_at", Value: -1}, {Key: "_id", Value: -1}}).SetSkip(int64((f.Page-1)*f.Limit)).SetLimit(int64(f.Limit)))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := cur.Close(ctx); err != nil {
			slog.Warn("cursor close failed", "error", err)
		}
	}()
	out := []model.Job{}
	err = cur.All(ctx, &out)
	return out, err
}
func (s *JobRepository) Job(ctx context.Context, id bson.ObjectID) (model.Job, error) {
	var v model.Job
	err := s.jobs.FindOne(ctx, bson.M{"_id": id}).Decode(&v)
	return v, err
}
