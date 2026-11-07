package main

import (
	"context"
	"encoding/json"
	"errors"
	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/repository"
	"flag"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/genai"
	"net/http"
	"net/http/httptest"
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
	execute := flag.Bool("execute", false, "execute source projection or, only if policy requires it, one AI call")
	direct := flag.Bool("direct-vertex", false, "with -execute, do not require management API probes")
	contentID := flag.String("content-id", "", "one existing department/source notice ID; required")
	sourceID := flag.String("source-id", "", "optional expected source ID; never overrides stored ownership")
	reportPath := flag.String("report", "", "safe JSON output path")
	flag.Parse()
	if *direct && !*execute {
		return fmt.Errorf("-direct-vertex requires -execute")
	}
	id, err := bson.ObjectIDFromHex(*contentID)
	if err != nil {
		return fmt.Errorf("-content-id must identify one existing stored notice")
	}
	report := map[string]any{"generation_requests": 0, "live_ssu_requests": 0, "stage": "configuration"}
	defer func() {
		b, _ := json.MarshalIndent(report, "", "  ")
		if *reportPath != "" {
			_ = os.WriteFile(*reportPath, append(b, '\n'), 0600)
		}
		fmt.Println(string(b))
	}()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	store, err := repository.Open(ctx, cfg.URI, cfg.Database)
	if err != nil {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = store.Client.Disconnect(c)
	}()
	item, err := store.Content(ctx, id)
	if err != nil {
		return err
	}
	if *sourceID != "" && *sourceID != item.SourceID.Hex() {
		return fmt.Errorf("content does not belong to requested source")
	}
	source, decision, err := store.CurrentDecision(ctx, item)
	if err != nil {
		return err
	}
	report["content_id"] = id.Hex()
	report["source_id"] = item.SourceID.Hex()
	report["strategy"] = source.ClassificationMode()
	report["needs_ai"] = decision.NeedsAI
	report["category"] = decision.Category
	report["title"] = item.Title
	if !*execute {
		report["stage"] = "policy_preview_only"
		return nil
	}
	if !decision.NeedsAI {
		record, err := store.ClassifyOne(ctx, item, nil, "")
		if err != nil {
			return err
		}
		report["classification"] = record
		report["stage"] = "source_complete_no_vertex_client"
		return nil
	}
	ai, err := classification.LoadConfig()
	if err != nil {
		return err
	}
	vertex, err := classification.NewVertex(ctx, ai)
	if err != nil {
		return err
	}
	defer func() { report["generation_requests"] = vertex.Attempts() }()
	report["requested_model"] = ai.Model
	report["project"] = ai.Project
	report["stage"] = "preflight"
	check, stop := context.WithTimeout(ctx, ai.Timeout)
	if *direct {
		err = vertex.PrepareDirectSingleRequest()
	} else {
		err = vertex.Preflight(check)
	}
	stop()
	if err != nil {
		report["error_code"] = classification.ErrorCode(err)
		return err
	}
	report["stage"] = "generation"
	record, err := store.ClassifyOne(ctx, item, vertex, ai.Model)
	if err != nil {
		report["error_code"] = classification.ErrorCode(err)
		var apiErr genai.APIError
		if errors.As(err, &apiErr) {
			report["http_status"] = apiErr.Code
			report["vertex_error_status"] = apiErr.Status
			report["vertex_error_message"] = apiErr.Message
		}
		return err
	}
	report["classification"] = record
	report["stage"] = "api"
	web := httptest.NewServer((api.API{Store: store, Token: cfg.Token}).Handler())
	defer web.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, web.URL+"/api/v1/contents/"+id.Hex(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	resp, err := web.Client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var final map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&final); err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("final content API verification failed")
	}
	report["api_status"] = resp.StatusCode
	report["final_content"] = final
	report["stage"] = "complete"
	return nil
}
