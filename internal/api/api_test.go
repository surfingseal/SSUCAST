package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"example.com/crawlserver/internal/model"
)

type fakeReader struct{ last model.Filter }

func (*fakeReader) Ping(context.Context) error                      { return nil }
func (*fakeReader) Sources(context.Context) ([]model.Source, error) { return []model.Source{}, nil }
func (*fakeReader) Jobs(context.Context, model.Filter) ([]model.Job, error) {
	return []model.Job{}, nil
}
func (*fakeReader) Job(context.Context, bson.ObjectID) (model.Job, error) {
	return model.Job{}, mongo.ErrNoDocuments
}
func (f *fakeReader) Contents(_ context.Context, q model.Filter) ([]model.Content, error) {
	f.last = q
	return []model.Content{}, nil
}
func (*fakeReader) Content(context.Context, bson.ObjectID) (model.Content, error) {
	return model.Content{}, mongo.ErrNoDocuments
}

type fakeStarter struct{}

func (fakeStarter) Start(_ context.Context, id bson.ObjectID) (model.Job, error) {
	return model.Job{ID: bson.NewObjectID(), SourceID: id, Status: "running"}, nil
}
func TestAPIValidationAndAuth(t *testing.T) {
	store := &fakeReader{}
	h := (API{store, fakeStarter{}, "secret"}).Handler()
	id := bson.NewObjectID().Hex()
	for _, tc := range []struct {
		method, path, body, auth string
		status                   int
	}{
		{"GET", "/health", "", "", 200}, {"GET", "/api/v1/sources", "", "", 401}, {"GET", "/api/v1/sources", "", "Bearer secret", 200},
		{"POST", "/api/v1/jobs", `{"source_id":"` + id + `"}`, "Bearer secret", 202},
		{"POST", "/api/v1/jobs", `{"source_id":"` + id + `","url":"http://localhost"}`, "Bearer secret", 400},
		{"POST", "/api/v1/jobs", `{"source_id":"` + id + `"} {}`, "Bearer secret", 400},
		{"GET", "/api/v1/contents?source_id[$ne]=x", "", "Bearer secret", 400},
		{"GET", "/api/v1/contents?page=0", "", "Bearer secret", 400},
		{"GET", "/api/v1/contents?limit=101", "", "Bearer secret", 400},
		{"GET", "/api/v1/contents?from=tomorrow", "", "Bearer secret", 400},
		{"GET", "/api/v1/contents?page=1&page=2", "", "Bearer secret", 400},
		{"GET", "/api/v1/contents/invalid", "", "Bearer secret", 400},
		{"GET", "/api/v1/contents/" + id, "", "Bearer secret", 404},
		{"GET", "/api/v1/contents?category=employment&limit=5&source_id=" + id, "", "Bearer secret", 200},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", tc.auth)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
			var v any
			if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
				t.Fatal(err)
			}
		})
	}
	if store.last.Limit != 5 || store.last.Category != "employment" || store.last.SourceID.Hex() != id {
		t.Fatal(store.last)
	}
}
func TestFilterRange(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/contents?from=2026-02-01T00:00:00Z&to=2026-01-01T00:00:00Z", nil)
	if _, err := parseFilter(r); err == nil {
		t.Fatal("reversed range accepted")
	}
}

func TestSourceNameFilter(t *testing.T) {
	store := &fakeReader{}
	h := (API{store, fakeStarter{}, "secret"}).Handler()
	for _, tc := range []struct {
		path string
		code int
	}{{"/api/v1/contents?source=ssu_catch", 200}, {"/api/v1/jobs?source=ssu_catch", 400}, {"/api/v1/contents?source=one&source=two", 400}, {"/api/v1/contents?source[$ne]=x", 400}} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
	}
	if store.last.SourceName != "ssu_catch" {
		t.Fatal(store.last)
	}
}
