// Read-only web executable: no crawler, Runner, AI client, source reconciliation,
// index creation or database writes are initialized by this command.
package main

import (
	"context"
	"errors"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/notices"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/web"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "웹 서버를 시작하거나 유지할 수 없습니다. DB/환경 설정과 포트 상태를 확인하세요.")
		os.Exit(1)
	}
}
func run() error {
	cfg, e := config.LoadMongoConnection("", "")
	if e != nil {
		return e
	}
	token := os.Getenv("ADMIN_TOKEN")
	if len(token) < 24 {
		return fmt.Errorf("ADMIN_TOKEN must contain24bytes")
	}
	addr, origin, e := webSettings(os.Getenv)
	if e != nil {
		return e
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	init, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	store, e := repository.OpenReadOnly(init, cfg.URI, cfg.Database)
	if e != nil {
		return e
	}
	defer store.Client.Disconnect(context.Background())
	reviewFile := os.Getenv("WEB_TIMING_REVIEW_FILE")
	explicitReview := reviewFile != ""
	if reviewFile == "" {
		reviewFile = "outputs/missing-schedules-review/annotations.json"
	}
	reviews, e := notices.LoadTimingReviews(reviewFile)
	if e != nil && (explicitReview || !os.IsNotExist(e)) {
		return fmt.Errorf("일정 점검 안내 파일을 확인하세요")
	}
	app := &web.Server{Store: store, Token: token, TimingReviews: reviews, PublicOrigin: origin}
	server := &http.Server{Addr: addr, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { fmt.Println("SSU 공지 모아보기: http://" + addr); done <- server.ListenAndServe() }()
	select {
	case e = <-done:
		if errors.Is(e, http.ErrServerClosed) {
			return nil
		}
		return e
	case <-ctx.Done():
	}
	end, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	return server.Shutdown(end)
}
