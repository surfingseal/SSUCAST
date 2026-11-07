package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/testutil/ssudetail"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Real page bytes are read locally. Only MongoDB and our httptest API server
// are contacted. No live crawler approval or source configuration is changed.
func TestSSUActualFixtureMongoAPI(t *testing.T) {
	item := ssudetail.Load(t, "../crawler/testdata/ssu_catch")
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("set TEST_MONGODB_URI for actual fixture MongoDB/API test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db := "crawltest_" + bson.NewObjectID().Hex()
	store, err := repository.Open(ctx, uri, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := store.Client.Database(db).Drop(cleanup); err != nil {
			t.Error(err)
		}
		if err := store.Client.Disconnect(cleanup); err != nil {
			t.Error(err)
		}
	}()
	item.SourceID = bson.NewObjectID()
	if err := store.Save(ctx, []model.Content{item}); err != nil {
		t.Fatal(err)
	}
	const token = "actual-fixture-local-test-token"
	web := httptest.NewServer((api.API{Store: store, Token: token}).Handler())
	defer web.Close()
	type response struct {
		Items []model.Content `json:"items"`
	}
	counts := map[string]int{}
	for _, tc := range []struct {
		query string
		count int
	}{
		{"category=scholarship", 1},
		{"category=academic", 0},
		{"source=ssu_catch", 1},
		{"source=ssu_catch&category=scholarship", 1},
		{"source=ssu_catch&category=academic", 0},
	} {
		t.Run(tc.query, func(t *testing.T) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, web.URL+"/api/v1/contents?"+tc.query, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := web.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("HTTP=%d", resp.StatusCode)
			}
			var got response
			if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if len(got.Items) != tc.count {
				t.Fatalf("count=%d want=%d", len(got.Items), tc.count)
			}
			counts[tc.query] = len(got.Items)
			if tc.count == 0 {
				return
			}
			v := got.Items[0]
			if v.ID.IsZero() || v.SourceID != item.SourceID || v.Title != item.Title || v.Content != item.Content || v.Department != item.Department || v.Category != "scholarship" || v.OriginalCategory != "장학" || v.OriginalURL != item.OriginalURL || v.ContentHash != item.ContentHash || !reflect.DeepEqual(v.Attachments, item.Attachments) {
				t.Fatal("API fields differ from reviewed actual fixture")
			}
		})
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, web.URL+"/api/v1/contents?category=scholarship", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := web.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated HTTP=%d", resp.StatusCode)
	}
	t.Log("ACTUAL HTML / LOCAL ONLY: stored=1 scholarship=1 academic=0 source=1 unauthenticated=401 live_SSU_requests=0")
	if output := os.Getenv("SSU_ACTUAL_REPORT"); output != "" {
		report := map[string]any{"mode": "user_supplied_actual_html_local_fixture", "test": t.Name(), "stored_documents": 1, "api_counts": counts, "unauthenticated_status": resp.StatusCode, "source_url": item.OriginalURL, "title": item.Title, "department": item.Department, "category": item.Category, "body": item.Content, "attachments": item.Attachments, "live_ssu_requests": 0}
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(output, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
