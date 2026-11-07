package imageanalysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/mvp"
	"fmt"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func Download(ctx context.Context, dir string, selected []Candidate, sources []model.Source) error {
	return DownloadCached(ctx, dir, selected, sources, nil)
}
func DownloadCached(ctx context.Context, dir string, selected []Candidate, sources []model.Source, cached map[string]ImageInput) error {
	if len(selected) > 20 {
		return fmt.Errorf("notice cap20")
	}
	if _, e := os.Stat(filepath.Join(dir, "downloaded.json")); e == nil {
		var old []Candidate
		if mvp.Read(filepath.Join(dir, "downloaded.json"), &old) != nil {
			return fmt.Errorf("cache corrupt")
		}
		for _, c := range old {
			for _, im := range c.Images {
				b, e := os.ReadFile(im.File)
				if e != nil || digest(b) != im.SHA256 {
					return fmt.Errorf("cached image hash mismatch")
				}
			}
		}
		fmt.Println("Image checkpoint reused; no school requests")
		return nil
	}
	if e := os.MkdirAll(filepath.Join(dir, "images"), 0700); e != nil {
		return e
	}
	by := map[string]model.Source{}
	for _, s := range sources {
		by[s.Name] = s
	}
	fetcher := crawler.NewFetcher(30 * time.Second)
	fetcher.MaxResponseBytes = 10 << 20
	sessions := map[string]*crawler.Session{}
	defer func() {
		for _, s := range sessions {
			s.Close()
		}
	}()
	out := []Candidate{}
	failures := []map[string]any{}
	count := 0
	for _, c := range selected {
		images := []ImageInput{}
		for imagePosition, im := range c.Images {
			if len(images) >= 3 {
				break
			}
			if !AllowedImage(c.Original, im.URL) {
				failures = append(failures, map[string]any{"id": c.Original.ID.Hex(), "image_index": im.Index, "reason": "unapproved_image_url"})
				continue
			}
			if old, ok := cached[im.URL]; ok {
				file := old.OriginalFile
				if file == "" {
					file = old.File
				}
				body, e := os.ReadFile(file)
				hash := old.OriginalSHA256
				if hash == "" {
					hash = old.SHA256
				}
				if e != nil || digest(body) != hash {
					return fmt.Errorf("prior image cache changed")
				}
				slots := 3 - len(images) - (len(c.Images) - imagePosition - 1)
				if slots < 1 {
					slots = 1
				}
				processed, e := Assets(body, im, filepath.Join(dir, "images"), slots)
				if e != nil {
					return e
				}
				for _, part := range processed {
					part.Index = len(images)
					images = append(images, part)
					count++
				}
				continue
			}
			u, _ := url.Parse(im.URL)
			origin := u.Scheme + "://" + u.Host
			sess := sessions[origin]
			if sess == nil {
				source, ok := by[c.Original.SourceName]
				if !ok || !source.PolicyApproved {
					return fmt.Errorf("source policy not approved")
				}
				source.BaseURL = origin
				source.ListURL = "/"
				source.RequestIntervalMS = 2000
				source.MaxConcurrency = 1
				source.MaxRequests = 100
				zero := 0
				source.MaxRetries = &zero
				var e error
				sess, e = fetcher.NewSession(source)
				if e != nil {
					return e
				}
				sessions[origin] = sess
			}
			page, e := sess.Get(ctx, im.URL)
			if e != nil {
				failures = append(failures, map[string]any{"id": c.Original.ID.Hex(), "image_index": im.Index, "reason": e.Error()})
				continue
			}
			mime := http.DetectContentType(page.Body)
			switch mime {
			case "image/jpeg":
			case "image/png":
			case "image/webp":
			default:
				failures = append(failures, map[string]any{"id": c.Original.ID.Hex(), "image_index": im.Index, "reason": "unsupported_or_nonimage_mime"})
				continue
			}
			slots := 3 - len(images) - (len(c.Images) - imagePosition - 1)
			if slots < 1 {
				slots = 1
			}
			processed, e := Assets(page.Body, im, filepath.Join(dir, "images"), slots)
			if e != nil {
				failures = append(failures, map[string]any{"id": c.Original.ID.Hex(), "image_index": im.Index, "reason": e.Error()})
				continue
			}
			for _, part := range processed {
				part.Index = len(images)
				images = append(images, part)
				count++
			}

		}
		if len(images) > 0 {
			c.Images = images
			out = append(out, c)
		} else {
			failures = append(failures, map[string]any{"id": c.Original.ID.Hex(), "reason": "no_accessible_inline_image"})
		}
	}
	records := []crawler.RequestRecord{}
	for _, s := range sessions {
		records = append(records, s.RequestLog()...)
	}
	if e := mvp.Write(dir, "downloaded.json", out); e != nil {
		return e
	}
	mvp.Write(dir, "image-fetch-failures.json", failures)
	mvp.Write(dir, "image-http-requests.json", records)
	mvp.Write(dir, "download-report.json", map[string]any{"notices": len(out), "images": count, "failed_items": len(failures), "requests": len(records), "attachments_downloaded": 0, "pacing_ms": 2000, "concurrency": 1, "allowed_hosts_only": true, "asset_body_cap_bytes": 10 << 20})
	fmt.Printf("Inline images downloaded notices=%d images=%d issues=%d requests=%d\n", len(out), count, len(failures), len(records))
	return nil
}
