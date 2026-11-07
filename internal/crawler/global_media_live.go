package crawler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"example.com/crawlserver/internal/model"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// Public API host is explicitly reviewed from the school's actual client.
// Each origin has independent robots/DNS/TLS checks, plus a single shared start
// limiter for this sequential crawler. No cookie/auth header is ever supplied.
func (c *GlobalMediaCrawler) crawlPublicAPI(ctx context.Context, s model.Source, r GlobalMediaReview) ([]model.Content, error) {
	if r.APIOrigin != "https://3-37-127-112.sslip.io" || s.BaseURL != "https://media.ssu.ac.kr" || r.EvidenceKind != "actual" {
		return nil, fmt.Errorf("cross-origin contract not reviewed")
	}
	api := s
	api.BaseURL = r.APIOrigin
	api.ListURL = r.ListPath
	web := s
	sess, e := c.Fetcher.NewSession(api)
	if e != nil {
		return nil, e
	}
	defer sess.Close()
	site, e := c.Fetcher.NewSession(web)
	if e != nil {
		return nil, e
	}
	defer site.Close()
	defer func() {
		if c.RequestAudit != nil {
			c.RequestAudit(append(sess.RequestLog(), site.RequestLog()...))
		}
	}()
	items := []model.Content{}
	seen := map[string]bool{}
	cutoff := c.Since
	if cutoff.IsZero() {
		cutoff = time.Now().In(time.FixedZone("Asia/Seoul", 9*3600)).AddDate(0, -3, 0)
	}
	last := time.Time{}
	get := func(session *Session, u string) (Page, error) {
		if d := time.Until(last.Add(2 * time.Second)); d > 0 {
			if e := pause(ctx, d); e != nil {
				return Page{}, e
			}
		}
		last = time.Now()
		return session.Get(ctx, u)
	}
	for page := 0; page < s.MaxPages; page++ {
		u, _ := url.Parse(r.APIOrigin + r.ListPath)
		q := u.Query()
		q.Set("menuId", r.MenuID)
		q.Set("size", strconv.Itoa(r.PageSize))
		q.Set("page", strconv.Itoa(page))
		q.Set("content", "")
		u.RawQuery = q.Encode()
		resp, e := get(sess, u.String())
		if e != nil {
			return items, e
		}
		if c.Capture != nil {
			c.Capture(fmt.Sprintf("list-%03d.json", page), resp)
		}
		rows, pages, e := ParseGlobalMediaList(resp.Body)
		if e != nil {
			return items, e
		}
		if len(rows) == 0 && page == 0 {
			return nil, fmt.Errorf("empty actual list")
		}
		oldNormal := 0
		normal := 0
		for _, row := range rows {
			if seen[row.ID] {
				continue
			}
			seen[row.ID] = true
			date, e := mediaDate(row.CreatedAt)
			if e != nil {
				return items, e
			}
			if !row.Pinned {
				normal++
				if date.Before(cutoff) {
					oldNormal++
				}
			}
			if date.Before(cutoff) {
				if row.Pinned && c.SkipAudit != nil {
					c.SkipAudit(row.ID, "old_pinned_candidate")
				}
				continue
			}
			if row.AccessRole != "ROLE_GUEST" || row.Content == "<redacted>" {
				if c.SkipAudit != nil {
					c.SkipAudit(row.ID, "restricted_notice")
				}
				continue
			}
			original, _ := GlobalMediaURL(s.BaseURL, "/board/notices/"+row.ID)
			if c.exclude(original, model.Content{Title: row.Title}) {
				continue
			}
			detailURL := r.APIOrigin + strings.ReplaceAll(r.DetailPath, "{id}", url.PathEscape(row.ID))
			detail, e := get(sess, detailURL)
			if e != nil {
				if c.SkipAudit != nil {
					c.SkipAudit(row.ID, "detail_fetch_failed")
				}
				return items, e
			}
			if c.Capture != nil {
				c.Capture("detail-"+row.ID+".json", detail)
			}
			item, e := ParseGlobalMediaDetail(detail.Body, original, row, r)
			if e != nil {
				if c.SkipAudit != nil {
					c.SkipAudit(row.ID, "detail_parse_failed:"+e.Error())
				}
				continue
			}
			if clean(item.Title) != clean(row.Title) || item.PublishedAt == nil || !item.PublishedAt.Equal(*date) {
				if c.SkipAudit != nil {
					c.SkipAudit(row.ID, "list_detail_conflict")
				}
				continue
			}
			if c.exclude(original, item) {
				continue
			}
			expected, _ := strconv.Atoi(item.Metadata["expected_attachments"])
			if expected > 0 {
				item.Metadata["attachment_metadata_state"] = "unavailable_public_metadata_endpoint"
				item.Metadata["collection_completeness"] = "partial_attachment_metadata"
				if c.SkipAudit != nil {
					c.SkipAudit(row.ID, "partial_attachment_metadata: public file metadata endpoint returned 500 in schema inspection; file downloads disabled")
				}
			} else {
				item.Metadata["collection_completeness"] = "complete"
			}

			now := time.Now().UTC()
			item.SourceID = s.ID
			item.SourceName = s.Name
			item.CrawledAt = now
			item.LastSeenAt = now
			item.UpdatedAt = now
			sum := sha256.Sum256([]byte(s.Name + "\n" + item.OriginalURL))
			copy(item.ID[:], sum[:12])
			item.ExternalPostID = row.ID
			item.IsPinned = row.Pinned
			item.ParserVersion = "global-media-public-api-v1"
			item.Status = "unknown"
			item.Metadata["published_timezone_policy"] = "API naive wall time interpreted as Asia/Seoul; same calendar/date string verified in rendered website; backend timezone not declared"
			item.Metadata["attachment_required"] = strconv.FormatBool(len(item.Attachments) > 0 || expected > 0)
			item.Metadata["category_absence_verified"] = "true"
			if e = model.PrepareClassification(s, &item); e != nil {
				return items, e
			}
			item.ContentHash = model.ContentDigest(item)
			items = append(items, item)
			if len(items) >= s.MaxContents {
				return items, nil
			}
		}
		if page+1 >= pages || normal > 0 && normal == oldNormal {
			break
		}
	}
	return items, nil
}
func ParseGlobalMediaAttachments(raw []byte, title, selector string) ([]model.Attachment, error) {
	if selector != "main ul a[href]" {
		return nil, fmt.Errorf("unreviewed attachment selector")
	}
	doc, e := goquery.NewDocumentFromReader(bytes.NewReader(raw))
	if e != nil {
		return nil, e
	}
	if clean(doc.Find("main h2").First().Text()) != clean(title) {
		return nil, fmt.Errorf("attachment page title mismatch")
	}
	result := []model.Attachment{}
	seen := map[string]bool{}
	var invalid bool
	doc.Find(selector).Each(func(_ int, a *goquery.Selection) {
		name := clean(a.Text())
		href := a.AttrOr("href", "")
		u, e := url.Parse(href)
		if e != nil || u.Scheme != "https" || u.Host != "globalmedia-bucket.s3.ap-northeast-2.amazonaws.com" || !strings.HasPrefix(u.Path, "/attachment/") {
			invalid = true
			return
		}
		if seen[href] {
			return
		}
		seen[href] = true
		if name == "" {
			invalid = true
			return
		}
		result = append(result, model.Attachment{Name: name, URL: href, Extension: strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")})
	})
	if invalid {
		return nil, fmt.Errorf("unexpected attachment metadata link")
	}
	return result, nil
}
