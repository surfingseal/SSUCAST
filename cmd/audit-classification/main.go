// Read-only audit: never creates a Vertex client, writes a MongoDB document,
// or downloads content. Quality is a derived view of a legacy prediction.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"flag"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	uri := flag.String("uri", "mongodb://127.0.0.1:27029", "MongoDB URI")
	db := flag.String("database", "ssu_vertex_validation", "existing database")
	idString := flag.String("content-id", "", "stored notice ID")
	fixture := flag.String("fixture-dir", "internal/crawler/testdata/ssu_catch", "trusted local media evidence")
	output := flag.String("output", "", "JSON report")
	flag.Parse()
	id, err := bson.ObjectIDFromHex(*idString)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(*uri))
	if err != nil {
		return err
	}
	defer client.Disconnect(context.Background())
	database := client.Database(*db)
	var input model.Content
	if err = database.Collection("crawled_contents").FindOne(ctx, bson.M{"_id": id}).Decode(&input); err != nil {
		return err
	}
	col := database.Collection("content_classifications")
	before, err := col.FindOne(ctx, bson.M{"content_id": id}, options.FindOne().SetSort(bson.D{{Key: "classified_at", Value: -1}})).Raw()
	if err != nil {
		return err
	}
	var stored model.Classification
	if err = bson.Unmarshal(before, &stored); err != nil {
		return err
	}
	local, err := crawler.ReadSSULocalFixture(*fixture)
	if err != nil {
		return err
	}
	if local.OriginalURL != input.OriginalURL || local.Title != input.Title || local.Content != input.Content {
		return fmt.Errorf("stored notice and local evidence mismatch")
	}
	// Enrich a memory copy only; leave the legacy stored document untouched.
	input.Metadata = local.Metadata
	cleaned, q := classification.ApplyQuality(input, stored.ClassificationLabels)
	after, err := col.FindOne(ctx, bson.M{"_id": stored.ID}).Raw()
	if err != nil {
		return err
	}
	a, b := sha256.Sum256(before), sha256.Sum256(after)
	if a != b {
		return fmt.Errorf("stored classification changed during audit")
	}
	report := map[string]any{"mode": "read_only_legacy_audit", "new_vertex_calls": 0, "school_original_category": input.OriginalCategory, "school_standard_category": input.Category, "stored_classification": stored, "derived_validated_labels": cleaned, "derived_quality": q, "legacy_document_unchanged": true, "stored_document_sha256": hex.EncodeToString(a[:]), "media_evidence": "matching local HTML fixture; no downloads; stored metadata not modified"}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if *output != "" {
		return os.WriteFile(*output, raw, 0600)
	}
	fmt.Print(string(raw))
	return nil
}
