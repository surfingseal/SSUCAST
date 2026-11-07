// Explicit public structure inspection only. Never runs at startup/in tests.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"flag"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	allow := flag.Bool("allow-public-inspection", false, "explicitly inspect robots and the public landing HTML only")
	dir := flag.String("output-dir", "internal/crawler/testdata/global_media", "capture directory")
	follow := flag.Bool("policy-and-script", false, "inspect only policy and board script links already captured in landing HTML")
	scriptOnly := flag.Bool("script-only", false, "with policy-and-script, inspect the already-observed board bundle only")
	flag.Parse()
	if !*allow {
		return fmt.Errorf("explicit inspection flag required")
	}
	if err := os.MkdirAll(*dir, 0755); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	f := crawler.NewFetcher(12 * time.Second)
	budget := 3
	if *follow && !*scriptOnly {
		budget = 5 // robots + policy redirect + destination + observed script
	}
	s, err := f.NewSession(model.Source{BaseURL: "https://media.ssu.ac.kr", ListURL: "/board/notices", MaxRequests: budget, RequestIntervalMS: 2000, MaxConcurrency: 1, MaxPages: 1, MaxContents: 1})
	if err != nil {
		return err
	}
	defer s.Close()
	defer func() {
		log := s.RequestLog()
		if *follow {
			var previous []crawler.RequestRecord
			if old, e := os.ReadFile(filepath.Join(*dir, "requests.json")); e == nil {
				_ = json.Unmarshal(old, &previous)
			}
			log = append(previous, log...)
		}
		raw, _ := json.MarshalIndent(log, "", "  ")
		_ = os.WriteFile(filepath.Join(*dir, "requests.json"), append(raw, '\n'), 0600)
		fmt.Printf("direct HTTP attempts=%d; policy_approved remains false\n", s.Requests())
	}()
	if *follow {
		raw, err := os.ReadFile(filepath.Join(*dir, "list.actual.html"))
		if err != nil {
			return err
		}
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(raw))
		if err != nil {
			return err
		}
		policy := doc.Find("a[href='/board/service_policy']").First().AttrOr("href", "")
		script := ""
		doc.Find("script[src]").Each(func(_ int, node *goquery.Selection) {
			src := node.AttrOr("src", "")
			if strings.Contains(src, "/board/%5BboardName%5D/page-") {
				script = src
			}
		})
		if policy == "" || script == "" {
			return fmt.Errorf("observed policy/script links unavailable")
		}
		targets := []struct{ url, file string }{{policy, "policy.actual.html"}, {script, "board-page.actual.js"}}
		if *scriptOnly {
			targets = []struct{ url, file string }{{script, "board-page.actual.js"}}
		}
		for _, target := range targets {
			page, err := s.Get(ctx, target.url)
			if len(page.Body) > 0 {
				if e := os.WriteFile(filepath.Join(*dir, target.file), page.Body, 0600); e != nil {
					return e
				}
			}
			if err != nil {
				return err
			}
			fmt.Printf("captured %s status=%d bytes=%d\n", target.file, page.Status, len(page.Body))
		}
		return nil
	}
	robots, err := s.Robots(ctx)
	if len(robots.Body) > 0 {
		name := "robots.actual.txt"
		if robots.Status == 404 {
			name = "robots.interpreted-missing.txt"
		}
		if writeErr := os.WriteFile(filepath.Join(*dir, name), robots.Body, 0600); writeErr != nil {
			return writeErr
		}
	}
	if err != nil {
		return err
	}
	page, err := s.Get(ctx, s.ListURL())
	if len(page.Body) > 0 {
		if e := os.WriteFile(filepath.Join(*dir, "list.actual.html"), page.Body, 0600); e != nil {
			return e
		}
	}
	if err != nil {
		return err
	}
	fmt.Printf("landing status=%d bytes=%d\n", page.Status, len(page.Body))
	return nil
}
