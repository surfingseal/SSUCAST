package main

import (
	"context"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/mvp"
	"flag"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
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
	out := flag.String("output", "", "new local capture directory")
	asOf := flag.String("as-of", time.Now().In(time.FixedZone("Asia/Seoul", 9*3600)).Format("2006-01-02"), "Asia/Seoul collection date")
	baseline := flag.String("baseline", "", "verified original BSON backup")
	flag.Parse()
	if *out == "" || *baseline == "" {
		return fmt.Errorf("output/baseline required")
	}
	if _, e := os.Stat(filepath.Join(*out, "contents.json")); e == nil {
		return fmt.Errorf("capture exists; refusing duplicate school requests")
	}
	if e := os.MkdirAll(*out, 0700); e != nil {
		return e
	}
	var source model.Source
	rows, e := mvp.ReadDump(filepath.Join(*baseline, "crawl_sources.bson"))
	if e != nil {
		return e
	}
	for _, r := range rows {
		var s model.Source
		bson.Unmarshal(r, &s)
		if s.Name == "ssu_global_media" {
			source = s
		}
	}
	if source.ID.IsZero() {
		source.ID = bson.NewObjectID()
	}
	source.Name = "ssu_global_media"
	source.DisplayName = "숭실대학교 글로벌미디어학부"
	source.BaseURL = "https://media.ssu.ac.kr"
	source.ListURL = "/board/notices"
	source.CrawlerType = "ssu_global_media"
	source.ClassificationStrategy = "hybrid"
	source.ClassificationRevision = 1
	source.PolicyApproved = true
	source.Enabled = false
	no := false
	source.AutoEnabled = &no
	source.MaxPages = 20
	source.MaxContents = 100
	source.MaxRequests = 250
	source.RequestIntervalMS = 2000
	source.MaxConcurrency = 1
	zero := 0
	source.MaxRetries = &zero
	source.CreatedAt = time.Now().UTC()
	source.UpdatedAt = source.CreatedAt
	source.PolicyApprovalBasis = "2026-11-07 actual robots Allow:/; linked public API robots 404; verified ROLE_GUEST list/detail without cookie or auth. Terms link redirects home and no collection prohibition observed. manual course research, no attachments downloaded."
	review := crawler.GlobalMediaReview{Reviewed: true, EvidenceKind: "actual", Evidence: "agent direct actual API and browser asset inspection 2026-11-07; captures in media-inspection", WebsiteOrigin: source.BaseURL, APIOrigin: "https://3-37-127-112.sslip.io", ListPath: "/v1/board/", MenuID: "136", PageSize: 15, DetailPath: "/v1/board/{id}", DetailFields: map[string]string{"id": "/data/id", "title": "/data/title", "content": "/data/content", "author": "/data/userName", "published_at": "/data/createdAt"}, BodyFormat: "editor_text_nodes", AttachmentsPointer: "/data/attachments", AttachmentIDsOnly: true, AttachmentSelector: "main ul a[href]", CategoryAbsenceVerified: true}
	// AttachmentIDsOnly uses the actual detail IDs. Direct SSR metadata was
	// unavailable; never pretend names/URLs were parsed when they were not.
	if e := mvp.Write(*out, "source.json", source); e != nil {
		return e
	}
	mvp.Write(*out, "review.json", review)
	date, e := time.ParseInLocation("2006-01-02", *asOf, time.FixedZone("Asia/Seoul", 9*3600))
	if e != nil {
		return fmt.Errorf("invalid as-of date")
	}
	since := date.AddDate(0, -3, 0)
	issues := []map[string]string{}
	records := []crawler.RequestRecord{}
	c := &crawler.GlobalMediaCrawler{Fetcher: crawler.NewFetcher(25 * time.Second), Review: &review, Since: since, Capture: func(name string, p crawler.Page) { os.WriteFile(filepath.Join(*out, name), p.Body, 0600) }, RequestAudit: func(r []crawler.RequestRecord) { records = r }, SkipAudit: func(id, reason string) {
		issues = append(issues, map[string]string{"external_post_id": id, "reason": reason})
	}, Audit: func(u string, r model.ExclusionReason) {
		issues = append(issues, map[string]string{"url": u, "reason": r.Code})
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	items, err := c.Crawl(ctx, source)
	mvp.Write(*out, "contents.json", items)
	mvp.Write(*out, "issues.json", issues)
	mvp.Write(*out, "requests.json", records)
	complete := 0
	for _, v := range items {
		if v.Metadata["collection_completeness"] == "complete" {
			complete++
		}
	}
	mvp.Write(*out, "report.json", map[string]any{"parsed_notices": len(items), "complete_notices": complete, "partial_notices": len(items) - complete, "http_requests": len(records), "since": since, "max_notices": 100, "interval_ms": 2000, "concurrency": 1, "attachments_downloaded": 0, "atlas_writes": 0, "error": func() string {
		if err != nil {
			return err.Error()
		}
		return ""
	}()})
	fmt.Printf("Media parsed=%d complete=%d partial=%d requests=%d files_downloaded=0\n", len(items), complete, len(items)-complete, len(records))
	return err
}
