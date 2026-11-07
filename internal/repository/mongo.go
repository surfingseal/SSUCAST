package repository

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Store struct {
	Client *mongo.Client
	*SourceRepository
	*JobRepository
	*ContentRepository
	*ClassificationRepository
}

func Open(ctx context.Context, uri, db string) (*Store, error) { return openStore(ctx, uri, db, true) }

// OpenReadOnly connects without creating/updating indexes or collections.
// It is used by audited read-only tools; write methods must not be invoked.
func OpenReadOnly(ctx context.Context, uri, db string) (*Store, error) {
	return openStore(ctx, uri, db, false)
}
func openStore(ctx context.Context, uri, db string, createIndexes bool) (*Store, error) {
	c, err := mongo.Connect(options.Client().ApplyURI(uri).SetMaxPoolSize(50).SetServerSelectionTimeout(5 * time.Second).SetTimeout(10 * time.Second))
	if err != nil {
		return nil, err
	}
	if err = c.Ping(ctx, nil); err != nil {
		disconnect(c)
		return nil, fmt.Errorf("mongo ping: %w", err)
	}
	sources := c.Database(db).Collection("crawl_sources")
	s := &Store{Client: c, SourceRepository: &SourceRepository{sources}, JobRepository: &JobRepository{jobs: c.Database(db).Collection("crawl_jobs"), sourceCollection: sources}, ContentRepository: &ContentRepository{c.Database(db).Collection("crawled_contents")}, ClassificationRepository: &ClassificationRepository{c.Database(db).Collection("content_classifications")}}
	if createIndexes {
		if err = s.indexes(ctx); err != nil {
			disconnect(c)
			return nil, err
		}
	}
	return s, nil
}
func (s *Store) indexes(ctx context.Context) error {
	for _, v := range []struct {
		c *mongo.Collection
		i []mongo.IndexModel
	}{
		{s.classifications, []mongo.IndexModel{{Keys: bson.D{{Key: "content_id", Value: 1}, {Key: "input_hash", Value: 1}, {Key: "taxonomy_version", Value: 1}, {Key: "model_version", Value: 1}}, Options: options.Index().SetUnique(true)}}},
		{s.sources, []mongo.IndexModel{{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true)}}},
		{s.jobs, []mongo.IndexModel{
			{Keys: bson.D{{Key: "source_id", Value: 1}, {Key: "started_at", Value: -1}}},
			{Keys: bson.D{{Key: "source_id", Value: 1}}, Options: options.Index().SetName("one_running_source").SetUnique(true).SetPartialFilterExpression(bson.M{"status": "running"})},
		}},
		{s.contents, []mongo.IndexModel{
			{Keys: bson.D{{Key: "original_url", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "source_id", Value: 1}, {Key: "crawled_at", Value: -1}, {Key: "_id", Value: -1}}},
			{Keys: bson.D{{Key: "category", Value: 1}, {Key: "published_at", Value: -1}}},
			{Keys: bson.D{{Key: "categories", Value: 1}, {Key: "published_at", Value: -1}}},
			{Keys: bson.D{{Key: "source_name", Value: 1}, {Key: "crawled_at", Value: -1}}},
			{Keys: bson.D{{Key: "crawled_at", Value: -1}, {Key: "_id", Value: -1}}},
		}},
	} {
		if _, err := v.c.Indexes().CreateMany(ctx, v.i); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) Ping(ctx context.Context) error { return s.Client.Ping(ctx, nil) }

func disconnect(c *mongo.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Disconnect(ctx); err != nil {
		slog.Warn("mongo cleanup failed", "error", err)
	}
}
