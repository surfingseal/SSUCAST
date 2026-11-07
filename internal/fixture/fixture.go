// Package fixture provides synthetic content containing no real personal data.
package fixture

import (
	_ "embed"
	"log/slog"
	"net/http"
)

//go:embed html/list.html
var list string

//go:embed html/list2.html
var list2 string

//go:embed html/post1.html
var post1 string

//go:embed html/post2.html
var post2 string

func write(w http.ResponseWriter, body string) {
	if _, err := w.Write([]byte(body)); err != nil {
		slog.Debug("fixture response failed", "error", err)
	}
}
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		write(w, "User-agent: *\nAllow: /\nDisallow: /private\n")
	})
	mux.HandleFunc("GET /posts/1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		write(w, post1)
	})
	mux.HandleFunc("GET /posts/2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		write(w, post2)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Query().Get("page") == "2" {
			write(w, list2)
			return
		}
		write(w, list)
	})
	return mux
}
