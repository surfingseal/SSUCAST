package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/crawlserver/internal/api"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/fixture"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/service"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if _, err := classification.LoadConfig(); err != nil {
		return err
	}
	root, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	initCtx, cancel := context.WithTimeout(root, 20*time.Second)
	defer cancel()
	store, err := repository.Open(initCtx, cfg.URI, cfg.Database)
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := store.Client.Disconnect(ctx); err != nil {
			slog.Error("mongo disconnect", "error", err)
		}
	}()
	fetcher := crawler.NewFetcher(cfg.RequestTimeout)
	registry, err := crawler.DefaultRegistry(fetcher)
	if err != nil {
		return err
	}
	for _, source := range cfg.Sources {
		if !registry.Has(source.CrawlerKey()) {
			return fmt.Errorf("source %s: unknown crawler type %s", source.Name, source.CrawlerKey())
		}
	}
	if os.Getenv("DEMO_MODE") == "true" {
		// The exception is restricted to this exact server, bound before crawling.
		ln, err := net.Listen("tcp", "127.0.0.1:8090")
		if err != nil {
			return err
		}
		demo := &http.Server{Handler: fixture.Handler(), ReadHeaderTimeout: 5 * time.Second}
		defer func() {
			if err := demo.Close(); err != nil {
				slog.Warn("fixture close failed", "error", err)
			}
		}()
		go func() {
			if err := demo.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("fixture server", "error", err)
			}
		}()
		cfg.SourceURL = "http://127.0.0.1:8090"
		fetcher.FixtureOrigin = cfg.SourceURL
	}
	if cfg.SourceURL != "" {
		seed := store.Seed
		if os.Getenv("DEMO_MODE") == "true" {
			seed = store.SeedDemo
		}
		if err := seed(initCtx, cfg.SourceURL, cfg.Schedule); err != nil {
			return err
		}
	}
	if err := store.ApplySources(initCtx, cfg.Sources); err != nil {
		return err
	}
	sources, err := store.Sources(initCtx)
	if err != nil {
		return err
	}
	runner := service.NewWithRegistry(store, registry, cfg.Workers, cfg.JobTimeout)
	defer runner.Stop()
	scheduler, err := service.Schedule(sources, runner)
	if err != nil {
		return err
	}
	defer func() { <-scheduler.Stop().Done() }()
	srv := &http.Server{Addr: cfg.Addr, Handler: (api.API{Store: store, Runner: runner, Token: cfg.Token}).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	result := make(chan error, 1)
	go func() { slog.Info("HTTP listening", "addr", cfg.Addr); result <- srv.ListenAndServe() }()
	select {
	case err = <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-root.Done():
	}
	scheduler.Stop()
	ctx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(ctx); err != nil {
		if closeErr := srv.Close(); closeErr != nil {
			slog.Warn("HTTP close failed", "error", closeErr)
		}
		return err
	}
	runner.Stop()
	return nil
}
