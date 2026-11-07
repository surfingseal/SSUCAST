// Package web exposes authenticated, read-only notices and an embedded UI.
package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/notices"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed static/*
var assets embed.FS

type Reader interface {
	Notices(context.Context, notices.Filter, time.Time) (notices.Page, error)
	Notice(context.Context, bson.ObjectID, time.Time) (notices.Notice, error)
	NoticeMetadata(context.Context) (notices.Metadata, error)
}
type Server struct {
	PublicOrigin  string
	TimingReviews notices.TimingReviews
	Store         Reader
	Token         string
	Now           func() time.Time
	mu            sync.Mutex
	sessions      map[string]time.Time
	failures      map[string][]time.Time
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func send(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func (s *Server) authorized(r *http.Request) bool {
	if token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") && s.validToken(token) {
		return true
	}
	cookie, e := r.Cookie("ssu_session")
	if e != nil {
		return false
	}
	hash := sha256.Sum256([]byte(cookie.Value))
	key := hex.EncodeToString(hash[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[key].After(s.now())
}
func (s *Server) validToken(token string) bool {
	a, b := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(s.Token))
	return len(s.Token) >= 24 && subtle.ConstantTimeCompare(a[:], b[:]) == 1
}
func (s *Server) Handler() http.Handler {
	static, _ := fs.Sub(assets, "static")
	files := http.FileServer(http.FS(static))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { send(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /api/session", s.login)
	mux.HandleFunc("DELETE /api/session", func(w http.ResponseWriter, r *http.Request) {
		if !s.sameOrigin(r) {
			send(w, 403, map[string]string{"error": "허용되지 않은 요청입니다"})
			return
		}
		if c, e := r.Cookie("ssu_session"); e == nil {
			hash := sha256.Sum256([]byte(c.Value))
			s.mu.Lock()
			delete(s.sessions, hex.EncodeToString(hash[:]))
			s.mu.Unlock()
		}
		http.SetCookie(w, &http.Cookie{Name: "ssu_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil || s.PublicOrigin != "", SameSite: http.SameSiteStrictMode})
		send(w, 200, map[string]bool{"authenticated": false})
	})
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, map[string]bool{"authenticated": s.authorized(r)})
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			send(w, 401, map[string]string{"error": "로그인이 필요합니다"})
			return
		}
		if r.Method != "GET" {
			w.Header().Set("Allow", "GET")
			send(w, 405, map[string]string{"error": "읽기 전용 API입니다"})
			return
		}
		s.api(w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		if r.URL.Path != "/" && r.URL.Path != "/app.js" && r.URL.Path != "/app.css" && r.URL.Path != "/favicon.svg" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}
func (s *Server) sameOrigin(r *http.Request) bool {
	if s.PublicOrigin != "" {
		return r.Header.Get("Origin") == s.PublicOrigin
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return r.Header.Get("Origin") == scheme+"://"+r.Host
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		send(w, 403, map[string]string{"error": "같은 사이트에서 로그인해 주세요"})
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	now := s.now()
	s.mu.Lock()
	if s.failures == nil {
		s.failures = map[string][]time.Time{}
	}
	for k, ts := range s.failures {
		active := []time.Time{}
		for _, t := range ts {
			if now.Sub(t) < 5*time.Minute {
				active = append(active, t)
			}
		}
		if len(active) == 0 {
			delete(s.failures, k)
		} else {
			s.failures[k] = active
		}
	}
	if len(s.failures[ip]) >= 10 || len(s.failures) > 256 {
		s.mu.Unlock()
		w.Header().Set("Retry-After", "300")
		send(w, 429, map[string]string{"error": "잠시 후 다시 로그인해 주세요"})
		return
	}
	s.mu.Unlock()
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	defer r.Body.Close()
	var in struct {
		Token string `json:"token"`
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || !s.validToken(in.Token) {
		s.mu.Lock()
		s.failures[ip] = append(s.failures[ip], now)
		s.mu.Unlock()
		send(w, 401, map[string]string{"error": "접근 토큰을 확인해 주세요"})
		return
	}
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		send(w, 503, map[string]string{"error": "로그인을 처리할 수 없습니다"})
		return
	}
	token := hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(token))
	s.mu.Lock()
	if s.sessions == nil {
		s.sessions = map[string]time.Time{}
	}
	for k, t := range s.sessions {
		if !t.After(now) {
			delete(s.sessions, k)
		}
	}
	if len(s.sessions) >= 256 {
		s.mu.Unlock()
		send(w, 503, map[string]string{"error": "잠시 후 다시 시도해 주세요"})
		return
	}
	s.sessions[hex.EncodeToString(hash[:])] = now.Add(8 * time.Hour)
	delete(s.failures, ip)
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "ssu_session", Value: token, Path: "/", MaxAge: 8 * 3600, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil || s.PublicOrigin != ""})
	send(w, 200, map[string]bool{"authenticated": true})
}
func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/api/categories" {
		if len(r.URL.Query()) != 0 {
			send(w, 400, map[string]string{"error": "지원하지 않는 조건입니다"})
			return
		}
		items := []map[string]string{}
		for _, c := range model.OfficialCategories {
			items = append(items, map[string]string{"id": c, "label": notices.CategoryLabels[c]})
		}
		send(w, 200, map[string]any{"items": items})
		return
	}
	if path == "/api/sources" {
		if len(r.URL.Query()) != 0 {
			send(w, 400, map[string]string{"error": "지원하지 않는 조건입니다"})
			return
		}
		v, e := s.Store.NoticeMetadata(r.Context())
		if e != nil {
			s.failure(w, e)
			return
		}
		send(w, 200, v)
		return
	}
	if path == "/api/notices" {
		f, e := notices.Parse(r.URL.Query())
		if e != nil {
			send(w, 400, map[string]string{"error": e.Error()})
			return
		}
		v, e := s.Store.Notices(r.Context(), f, s.now())
		if e != nil {
			s.failure(w, e)
			return
		}
		for i := range v.Items {
			s.TimingReviews.Apply(&v.Items[i])
		}
		send(w, 200, v)
		return
	}
	if strings.HasPrefix(path, "/api/notices/") {
		if len(r.URL.Query()) != 0 {
			send(w, 400, map[string]string{"error": "지원하지 않는 조건입니다"})
			return
		}
		id, e := bson.ObjectIDFromHex(strings.TrimPrefix(path, "/api/notices/"))
		if e != nil {
			send(w, 400, map[string]string{"error": "공고 ID가 올바르지 않습니다"})
			return
		}
		v, e := s.Store.Notice(r.Context(), id, s.now())
		if e != nil {
			s.failure(w, e)
			return
		}
		s.TimingReviews.Apply(&v)
		send(w, 200, v)
		return
	}
	send(w, 404, map[string]string{"error": "찾을 수 없는 API입니다"})
}
func (s *Server) failure(w http.ResponseWriter, e error) {
	if errors.Is(e, mongo.ErrNoDocuments) {
		send(w, 404, map[string]string{"error": "공고를 찾을 수 없습니다"})
		return
	}
	if errors.Is(e, context.DeadlineExceeded) {
		send(w, 504, map[string]string{"error": "조회 시간이 초과되었습니다. 다시 시도해 주세요"})
		return
	}
	send(w, 503, map[string]string{"error": "공고를 불러오지 못했습니다. 잠시 후 다시 시도해 주세요"})
}
