package web

import (
	"context"
	"encoding/json"
	"errors"
	"example.com/crawlserver/internal/notices"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fake struct {
	filter notices.Filter
	fail   bool
}

func (f *fake) Notices(_ context.Context, q notices.Filter, now time.Time) (notices.Page, error) {
	f.filter = q
	if f.fail {
		return notices.Page{}, errors.New("mongodb://private:secret@internal")
	}
	return notices.Page{Items: []notices.Notice{{ID: bson.NewObjectID().Hex(), Title: "<script>alert(1)</script>"}}, Total: 1}, nil
}
func (f *fake) Notice(context.Context, bson.ObjectID, time.Time) (notices.Notice, error) {
	return notices.Notice{}, mongo.ErrNoDocuments
}
func (f *fake) NoticeMetadata(context.Context) (notices.Metadata, error) {
	return notices.Metadata{}, nil
}

const testToken = "test-only-token-more-than-24chars"

func call(h http.Handler, method, path, body string, auth bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.Header.Set("Origin", "http://localhost")
	if auth {
		r.Header.Set("Authorization", "Bearer "+testToken)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestProtectedReadOnlyAPI(t *testing.T) {
	f := &fake{}
	s := &Server{Store: f, Token: testToken}
	h := s.Handler()
	if w := call(h, "GET", "/", "", false); w.Code != 200 || !strings.Contains(w.Body.String(), "SSU 공지 모아보기") || strings.Contains(w.Body.String(), testToken) {
		t.Fatal("shell")
	}
	if w := call(h, "GET", "/api/notices", "", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	w := call(h, "GET", "/api/notices?query=장학&source=ssu_global_media&page=2", "", true)
	if w.Code != 200 || f.filter.Query != "장학" || f.filter.Page != 2 || strings.Contains(w.Body.String(), "<script>") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call(h, "GET", "/api/notices?limit=500", "", true); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = call(h, "GET", "/api/notices/000000000000000000000000", "", true); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w = call(h, "GET", "/api/notices/not-an-id", "", true); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = call(h, "POST", "/api/notices", "", true); w.Code != 405 {
		t.Fatal(w.Code)
	}
	f.fail = true
	w = call(h, "GET", "/api/notices", "", true)
	if w.Code != 503 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal("error leak")
	}
}
func TestSessionCookieAndLogout(t *testing.T) {
	s := &Server{Store: &fake{}, Token: testToken}
	h := s.Handler()
	b, _ := json.Marshal(map[string]string{"token": testToken})
	w := call(h, "POST", "/api/session", string(b), false)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatal(w.Code)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Value == testToken {
		t.Fatal("unsafe cookie")
	}
	r := httptest.NewRequest("GET", "http://localhost/api/notices", nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("POST", "http://localhost/api/session", strings.NewReader(string(b)))
	r.Header.Set("Origin", "https://attacker.invalid")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("CSRF")
	}
	r = httptest.NewRequest("DELETE", "http://localhost/api/session", nil)
	r.Header.Set("Origin", "http://localhost")
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	r = httptest.NewRequest("GET", "http://localhost/api/notices", nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("logout failed")
	}
}

func TestHTTPSProxyOriginAndHealth(t *testing.T) {
	app := &Server{Store: &fake{}, Token: testToken, PublicOrigin: "https://ssucast.onrender.com"}
	h := app.Handler()
	req := httptest.NewRequest("POST", "http://internal/api/session", strings.NewReader(`{"token":"`+testToken+`"}`))
	req.Header.Set("Origin", "https://ssucast.onrender.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 || !w.Result().Cookies()[0].Secure || !w.Result().Cookies()[0].HttpOnly {
		t.Fatal("proxy HTTPS login/cookie failed", w.Code)
	}
	req = httptest.NewRequest("POST", "http://internal/api/session", strings.NewReader(`{"token":"`+testToken+`"}`))
	req.Header.Set("Origin", "https://attacker.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("cross-origin login allowed")
	}
	req = httptest.NewRequest("GET", "http://internal/healthz", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 || strings.Contains(w.Body.String(), testToken) {
		t.Fatal("healthcheck failed")
	}
	req = httptest.NewRequest("GET", "http://internal/api/notices", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal("deployment disabled authentication")
	}
}
