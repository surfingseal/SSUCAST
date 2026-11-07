package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"example.com/crawlserver/internal/lifecycle"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/service"
)

type Reader interface {
	Ping(context.Context) error
	Sources(context.Context) ([]model.Source, error)
	Jobs(context.Context, model.Filter) ([]model.Job, error)
	Job(context.Context, bson.ObjectID) (model.Job, error)
	Contents(context.Context, model.Filter) ([]model.Content, error)
	Content(context.Context, bson.ObjectID) (model.Content, error)
}
type Starter interface {
	Start(context.Context, bson.ObjectID) (model.Job, error)
}
type API struct {
	Store  Reader
	Runner Starter
	Token  string
}

func (a API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if a.Store.Ping(ctx) != nil {
			reply(w, 503, map[string]string{"status": "unavailable"})
			return
		}
		reply(w, 200, map[string]string{"status": "ok"})
	})
	mux.Handle("/api/v1/", a.auth(http.HandlerFunc(a.serve)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}
func (a API) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if a.Token == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare([]byte(token), []byte(a.Token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			reply(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("response write failed", "error", err)
	}
}
func fail(w http.ResponseWriter, err error) {
	status := 500
	msg := "internal server error"
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		status = 404
		msg = "not found"
	case errors.Is(err, repository.ErrBusy), errors.Is(err, service.ErrCapacity):
		status = 409
		msg = err.Error()
	case errors.Is(err, service.ErrDisabled):
		status = 422
		msg = err.Error()
	}
	if status == 500 {
		slog.Error("API operation failed", "error", err)
	}
	reply(w, status, map[string]string{"error": msg})
}
func bad(w http.ResponseWriter) { reply(w, 400, map[string]string{"error": "invalid request"}) }
func parseFilter(r *http.Request) (model.Filter, error) {
	f := model.Filter{Page: 1, Limit: 20}
	q := r.URL.Query()
	allowed := map[string]bool{"page": true, "limit": true, "source_id": true, "source": true, "category": true, "from": true, "to": true, "include_expired": true, "include_completed": true, "lifecycle_status": true}
	for k, v := range q {
		if !allowed[k] || len(v) != 1 {
			return f, errors.New("unknown or repeated query")
		}
	}
	if q.Has("include_completed") {
		value := q.Get("include_completed")
		if value != "true" && value != "false" {
			return f, errors.New("include_completed must be true/false")
		}
		f.IncludeCompleted = value == "true"
	}
	if q.Has("include_expired") {
		v := q.Get("include_expired")
		if v != "true" && v != "false" {
			return f, errors.New("include_expired must be true/false")
		}
		f.IncludeExpired = v == "true"
	}
	f.LifecycleStatus = q.Get("lifecycle_status")
	if f.LifecycleStatus != "" && f.LifecycleStatus != "active" && f.LifecycleStatus != "expired" && f.LifecycleStatus != "needs_review" && f.LifecycleStatus != "no_deadline" {
		return f, errors.New("invalid lifecycle_status")
	}
	if f.LifecycleStatus == "expired" {
		if q.Get("include_expired") == "false" {
			return f, errors.New("conflicting expired filters")
		}
		f.IncludeExpired = true
	}
	var err error
	if v := q.Get("page"); v != "" {
		f.Page, err = strconv.Atoi(v)
		if err != nil {
			return f, err
		}
	}
	if v := q.Get("limit"); v != "" {
		f.Limit, err = strconv.Atoi(v)
		if err != nil {
			return f, err
		}
	}
	if f.Page < 1 || f.Page > 10000 || f.Limit < 1 || f.Limit > 100 {
		return f, errors.New("pagination range")
	}
	if v := q.Get("source_id"); v != "" {
		id, err := bson.ObjectIDFromHex(v)
		if err != nil {
			return f, err
		}
		f.SourceID = &id
	}
	f.Category = q.Get("category")
	f.SourceName = q.Get("source")
	if len(f.SourceName) > 64 {
		return f, errors.New("source too long")
	}
	if len(f.Category) > 64 {
		return f, errors.New("category too long")
	}
	for _, v := range []struct {
		k string
		p **time.Time
	}{{"from", &f.From}, {"to", &f.To}} {
		if raw := q.Get(v.k); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return f, err
			}
			*v.p = &t
		}
	}
	if f.From != nil && f.To != nil && f.From.After(*f.To) {
		return f, errors.New("date range")
	}
	return f, nil
}
func (a API) serve(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	if path == "jobs" && r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		defer r.Body.Close()
		var input struct {
			SourceID string `json:"source_id"`
		}
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if d.Decode(&input) != nil {
			bad(w)
			return
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			bad(w)
			return
		}
		id, err := bson.ObjectIDFromHex(input.SourceID)
		if err != nil {
			bad(w)
			return
		}
		job, err := a.Runner.Start(r.Context(), id)
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/jobs/"+job.ID.Hex())
		reply(w, 202, job)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		if path == "jobs" {
			w.Header().Set("Allow", "GET, POST")
		}
		reply(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	if path == "sources" {
		v, err := a.Store.Sources(r.Context())
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, v)
		return
	}
	if strings.HasPrefix(path, "contents/") && strings.HasSuffix(path, "/classifications") {
		id, err := bson.ObjectIDFromHex(strings.TrimSuffix(strings.TrimPrefix(path, "contents/"), "/classifications"))
		include, filterErr := includeExpiredDetail(r)
		if err != nil || filterErr != nil {
			bad(w)
			return
		}
		reader, ok := a.Store.(interface {
			Classifications(context.Context, bson.ObjectID) ([]model.Classification, error)
		})
		if !ok {
			reply(w, 501, map[string]string{"error": "classifications unavailable"})
			return
		}
		parent, err := a.Store.Content(r.Context(), id)
		if err == nil && !include && lifecycle.EffectiveStatus(parent, time.Now()) == "expired" {
			err = mongo.ErrNoDocuments
		}
		if err != nil {
			fail(w, err)
			return
		}
		items, err := reader.Classifications(r.Context(), id)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, map[string]any{"items": items})
		return
	}
	for _, kind := range []string{"jobs", "contents"} {
		if path == kind {
			f, err := parseFilter(r)
			if err != nil {
				bad(w)
				return
			}
			var v any
			if kind == "jobs" {
				if f.Category != "" || f.SourceName != "" || r.URL.Query().Has("include_expired") || r.URL.Query().Has("lifecycle_status") || r.URL.Query().Has("include_completed") {
					bad(w)
					return
				}
				v, err = a.Store.Jobs(r.Context(), f)
			} else {
				v, err = a.Store.Contents(r.Context(), f)
			}
			if err != nil {
				fail(w, err)
				return
			}
			reply(w, 200, map[string]any{"items": v, "page": f.Page, "limit": f.Limit})
			return
		}
		if strings.HasPrefix(path, kind+"/") {
			id, err := bson.ObjectIDFromHex(strings.TrimPrefix(path, kind+"/"))
			if err != nil {
				bad(w)
				return
			}
			var v any
			if kind == "jobs" {
				v, err = a.Store.Job(r.Context(), id)
			} else {
				include, x := includeExpiredDetail(r)
				if x != nil {
					bad(w)
					return
				}
				item, x := a.Store.Content(r.Context(), id)
				if x == nil && !include && lifecycle.EffectiveStatus(item, time.Now()) == "expired" {
					x = mongo.ErrNoDocuments
				}
				v, err = item, x
			}
			if err != nil {
				fail(w, err)
				return
			}
			reply(w, 200, v)
			return
		}
	}
	reply(w, 404, map[string]string{"error": "not found"})
}

func includeExpiredDetail(r *http.Request) (bool, error) {
	q := r.URL.Query()
	if len(q) == 0 {
		return false, nil
	}
	if len(q) != 1 || len(q["include_expired"]) != 1 {
		return false, errors.New("invalid detail query")
	}
	switch q.Get("include_expired") {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, errors.New("invalid include_expired")
}
