package main

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	dir := flag.String("output-dir", "", "new local evidence directory")
	execute := flag.Bool("execute", false, "explicit user-authorized public category HTML inspection only")
	flag.Parse()
	if !*execute || *dir == "" {
		return fmt.Errorf("explicit execute and new directory required")
	}
	if e := os.Mkdir(*dir, 0700); e != nil {
		return e
	}
	f := crawler.NewFetcher(20 * time.Second)
	s, e := f.NewSession(model.Source{BaseURL: crawler.SSUCatchOrigin, ListURL: "/공지사항/", MaxRequests: 12, RequestIntervalMS: 2000, MaxConcurrency: 1})
	if e != nil {
		return e
	}
	defer s.Close()
	defer func() {
		b, _ := json.MarshalIndent(s.RequestLog(), "", "  ")
		_ = os.WriteFile(filepath.Join(*dir, "requests.json"), append(b, '\n'), 0600)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	robot, e := s.Robots(ctx)
	if e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(*dir, "robots.txt"), robot.Body, 0600); e != nil {
		return e
	}
	for _, c := range crawler.SSUOfficialCategories {
		u, e := crawler.SSUCategoryURL(crawler.SSUCatchOrigin, c)
		if e != nil {
			return e
		}
		p, e := s.Get(ctx, u)
		if e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(*dir, c.Code+".actual.html"), p.Body, 0600); e != nil {
			return e
		}
		fmt.Println(c.Code, p.Status, len(p.Body))
	}
	return nil
}
