package crawler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"example.com/crawlserver/internal/model"
)

type Crawler interface {
	Crawl(context.Context, model.Source) ([]model.Content, error)
}

// Example is retained as a fixture parser and delegates all networking to Fetcher.
type Example struct {
	Timeout    time.Duration
	DemoOrigin string
	fetcher    *Fetcher
}

func New(timeout time.Duration) *Example {
	return &Example{Timeout: timeout, fetcher: NewFetcher(timeout)}
}
func clean(s string) string { return strings.Join(strings.Fields(s), " ") }
func (e *Example) Crawl(ctx context.Context, s model.Source) ([]model.Content, error) {
	f := e.fetcher
	if f == nil {
		f = NewFetcher(e.Timeout)
	}
	fixture := f.FixtureOrigin
	if e.DemoOrigin != "" {
		fixture = e.DemoOrigin
	}
	session, err := f.newSession(s, fixture)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	s = s.WithDefaults()
	seenPages := map[string]bool{}
	seenPosts := map[string]bool{}
	items := []model.Content{}
	pageURL := session.ListURL()
	for pageURL != "" {
		if seenPages[pageURL] {
			break
		}
		if len(seenPages) >= s.MaxPages {
			return items, nil
		}
		seenPages[pageURL] = true
		response, err := session.Get(ctx, pageURL)
		if err != nil {
			return items, err
		}
		pageURL = response.URL
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(response.Body))
		if err != nil {
			return items, fmt.Errorf("list request: %w", err)
		}
		links := []string{}
		doc.Find("a.post").Each(func(_ int, s *goquery.Selection) {
			if h, ok := s.Attr("href"); ok {
				links = append(links, h)
			}
		})
		for _, link := range links {
			u, err := session.Resolve(pageURL, link)
			if err != nil {
				return items, err
			}
			if seenPosts[u] {
				continue
			}
			seenPosts[u] = true
			if len(seenPosts) > s.MaxContents {
				return items, nil
			}
			detailResponse, err := session.Get(ctx, u)
			if err != nil {
				return items, err
			}
			detail, err := goquery.NewDocumentFromReader(bytes.NewReader(detailResponse.Body))
			if err != nil {
				return items, fmt.Errorf("detail request: %w", err)
			}
			title, body := clean(detail.Find("article h1").Text()), clean(detail.Find("article .body").Text())
			if title == "" || body == "" {
				return items, fmt.Errorf("required article fields missing")
			}
			now := time.Now().UTC()
			v := model.Content{SourceID: s.ID, Title: title, Content: body, OriginalURL: detailResponse.URL, OriginalCategory: clean(detail.Find("article").AttrOr("data-category", "")), Category: model.NormalizeOfficialCategory(detail.Find("article").AttrOr("data-category", "")), Tags: []string{}, CrawledAt: now, UpdatedAt: now, Metadata: map[string]string{}}
			detail.Find(".tag").Each(func(_ int, s *goquery.Selection) {
				if t := clean(s.Text()); t != "" {
					v.Tags = append(v.Tags, t)
				}
			})
			if t := detail.Find("time").AttrOr("datetime", ""); t != "" {
				parsed, err := time.Parse(time.RFC3339, t)
				if err != nil {
					return items, fmt.Errorf("invalid published time")
				}
				v.PublishedAt = &parsed
			}
			for _, k := range []string{"company", "job_role", "experience_type"} {
				if x := clean(detail.Find("article").AttrOr("data-"+strings.ReplaceAll(k, "_", "-"), "")); x != "" {
					v.Metadata[k] = x
				}
			}
			v.ContentHash = fmt.Sprintf("%x", sha256.Sum256([]byte(title+"\n"+body)))
			items = append(items, v)
		}
		if len(items) >= s.MaxContents {
			return items, nil
		}
		next, ok := doc.Find("a.next").First().Attr("href")
		if !ok || next == "" {
			pageURL = ""
		} else {
			pageURL, err = session.Resolve(pageURL, next)
			if err != nil {
				return items, err
			}
		}
	}
	return items, nil
}
