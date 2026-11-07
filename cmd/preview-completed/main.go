// Read-only Atlas preview: no collection/index creation or document writes.
package main

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/lifecycle"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"flag"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"time"
)

type row struct {
	ID         bson.ObjectID `json:"id"`
	Title      string        `json:"title"`
	Category   string        `json:"category"`
	Categories []string      `json:"categories"`
	Status     string        `json:"status"`
}

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "completed visibility preview failed; credentials omitted")
		os.Exit(1)
	}
}

const token = "read-only-completed-preview-token"

func readAPI(ctx context.Context, h http.Handler, ids []bson.ObjectID, values url.Values) ([]model.Content, error) {
	out := []model.Content{}
	for _, id := range ids {
		q := url.Values{}
		for k, v := range values {
			q[k] = v
		}
		q.Set("source_id", id.Hex())
		q.Set("limit", "100")
		for page := 1; page <= 100; page++ {
			q.Set("page", fmt.Sprint(page))
			req := httptest.NewRequest(http.MethodGet, "/api/v1/contents?"+q.Encode(), nil).WithContext(ctx)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != 200 {
				return nil, fmt.Errorf("API verification status=%d", w.Code)
			}
			var response struct {
				Items []model.Content `json:"items"`
			}
			if e := json.Unmarshal(w.Body.Bytes(), &response); e != nil {
				return nil, e
			}
			out = append(out, response.Items...)
			if len(response.Items) < 100 {
				break
			}
			if page == 100 {
				return nil, fmt.Errorf("preview pagination limit")
			}
		}
	}
	return out, nil
}
func run() error {
	out := flag.String("output", "", "safe JSON report path")
	flag.Parse()
	cfg, e := config.LoadMongoConnection("", "")
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	store, e := repository.OpenReadOnly(ctx, cfg.URI, cfg.Database)
	if e != nil {
		return e
	}
	defer store.Client.Disconnect(context.Background())
	db := store.Client.Database(cfg.Database)
	snapshots, before, e := lifecycle.Read(ctx, db)
	if e != nil {
		return e
	}
	ids, e := repository.SSUCatchSourceIDs(ctx, db)
	if e != nil {
		return e
	}
	sourceIDs := map[bson.ObjectID]bool{}
	for _, id := range ids {
		sourceIDs[id] = true
	}
	counts := map[string]int{}
	hidden := []row{}
	total := 0
	for _, s := range snapshots {
		v := s.Content
		if !sourceIDs[v.SourceID] {
			continue
		}
		total++
		counts[v.Status]++
		if v.Status == "완료" {
			hidden = append(hidden, row{v.ID, v.Title, v.Category, v.Categories, v.Status})
		}
	}
	handler := (api.API{Store: store, Token: token}).Handler()
	defaults, e := readAPI(ctx, handler, ids, url.Values{})
	if e != nil {
		return e
	}
	completed, e := readAPI(ctx, handler, ids, url.Values{"include_completed": {"true"}})
	if e != nil {
		return e
	}
	all, e := readAPI(ctx, handler, ids, url.Values{"include_completed": {"true"}, "include_expired": {"true"}})
	if e != nil {
		return e
	}
	categories := map[string]any{}
	for _, spec := range crawler.SSUOfficialCategories {
		normal, e := readAPI(ctx, handler, ids, url.Values{"category": {spec.Code}})
		if e != nil {
			return e
		}
		complete, e := readAPI(ctx, handler, ids, url.Values{"category": {spec.Code}, "include_completed": {"true"}})
		if e != nil {
			return e
		}
		count := 0
		hiddenCount := 0
		for _, s := range snapshots {
			v := s.Content
			if !sourceIDs[v.SourceID] {
				continue
			}
			member := v.Category == spec.Code
			for _, c := range v.Categories {
				if c == spec.Code {
					member = true
				}
			}
			if member {
				count++
				if v.Status == "완료" {
					hiddenCount++
				}
			}
		}
		categories[spec.Code] = map[string]any{"label": spec.Label, "total_memberships": count, "completed_hidden": hiddenCount, "default_count": len(normal), "include_completed_count": len(complete), "api_http_status": 200}
	}
	unauth := httptest.NewRecorder()
	handler.ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/api/v1/contents", nil))
	if unauth.Code != 401 {
		return fmt.Errorf("API authentication regression")
	}
	cleanup, e := lifecycle.PlanCleanup(ctx, db, time.Now())
	if e != nil {
		return e
	}
	_, after, e := lifecycle.Read(ctx, db)
	if e != nil {
		return e
	}
	if before != after {
		return fmt.Errorf("dataset changed concurrently; retry")
	}
	report := map[string]any{"database": cfg.Database, "source": "ssu_catch", "source_ids": ids, "checked_at": time.Now().In(lifecycle.Seoul), "total": total, "status_counts": counts, "hidden_completed": len(hidden), "kept_before_lifecycle_filter": total - len(hidden), "default_count": len(defaults), "include_completed_count": len(completed), "include_completed_and_expired_count": len(all), "categories": categories, "hidden_items": hidden, "snapshot_before_sha256": before, "snapshot_after_sha256": after, "database_unchanged": true, "database_changes": 0, "api_authenticated_status": 200, "api_without_auth_status": unauth.Code, "cleanup_dry_run_candidates": len(cleanup.Candidates), "actual_delete_executed": false, "school_http_requests": 0, "vertex_requests": 0}
	b, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		return e
	}
	if *out != "" {
		if e = os.WriteFile(*out, append(b, '\n'), 0600); e != nil {
			return e
		}
	}
	fmt.Printf("SSU total=%d completed_hidden=%d default=%d include_completed=%d database_changes=0\n", total, len(hidden), len(defaults), len(completed))
	return nil
}
