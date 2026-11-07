package crawler

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"example.com/crawlserver/internal/model"
	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"
)

const SSUCatchOrigin = "https://scatch.ssu.ac.kr"
const ssuNoticePath = "/공지사항/"

// SSUCatchCrawler reuses Fetcher; no networking occurs without explicit policy
// approval and a body selector verified against an actual detail-page capture.
// The user-supplied detail fixture verifies one layout; the shipped source is off.
type SSUCatchCrawler struct {
	Fetcher *Fetcher
	// Optional evidence callbacks run synchronously; no callback changes fetching.
	Capture              func(string, Page) error
	Requests             func([]RequestRecord)
	RecentReport         func(SSURecentReport)
	Now                  func() time.Time
	RecentPinned         func(*goquery.Selection) bool
	RecentDetailCategory string
	RecentItem           func(model.Content) error
}

func ssuError(stage, raw string, err error) error {
	e := failure("parse", stage, raw, 0, 0, err)
	e.Reason = err.Error()
	return e
}
func ssuLimits(s model.Source) model.Source {
	s = s.WithDefaults()
	s.MaxPages = min(s.MaxPages, 1)
	s.MaxContents = min(s.MaxContents, 3)
	s.MaxRequests = min(s.MaxRequests, 5)
	s.MaxConcurrency = 1
	s.RequestIntervalMS = max(s.RequestIntervalMS, 2000)
	no := false
	s.AutoEnabled = &no
	return s
}
func (c *SSUCatchCrawler) Crawl(ctx context.Context, s model.Source) ([]model.Content, error) {
	if s.CollectionMode == "recent_three_months" {
		return c.crawlRecent(ctx, s)
	}
	if !s.PolicyApproved {
		return nil, failure("policy_unreviewed", "policy", s.BaseURL, 0, 0, fmt.Errorf("SSU collection policy remains unverified"))
	}
	if err := validateSSUSelector(s.DetailBodySelector); err != nil {
		return nil, failure("schema_unverified", "detail_schema", s.BaseURL, 0, 0, err)
	}
	if c.Fetcher == nil {
		return nil, failure("config", "validate", s.BaseURL, 0, 0, fmt.Errorf("shared Fetcher required"))
	}
	s = ssuLimits(s)
	session, err := c.Fetcher.NewSession(s)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	if c.Requests != nil {
		defer func() { c.Requests(session.RequestLog()) }()
	}
	base, _ := url.Parse(session.ListURL())
	origin := base.Scheme + "://" + base.Host
	if origin != SSUCatchOrigin && origin != c.Fetcher.FixtureOrigin {
		return nil, failure("config", "validate", s.BaseURL, 0, 0, fmt.Errorf("unexpected SSU origin"))
	}
	if base.Path != ssuNoticePath || base.Query().Get("slug") != "" {
		return nil, failure("config", "validate", s.ListURL, 0, 0, fmt.Errorf("expected first notice list page"))
	}
	if c.Capture != nil {
		robots, e := session.Robots(ctx)
		if len(robots.Body) > 0 {
			if captureErr := c.Capture("robots", robots); captureErr != nil {
				return nil, captureErr
			}
		}
		if e != nil {
			return nil, e
		}
	}
	var items []model.Content
	pageURL := session.ListURL()
	seen := map[string]bool{}
	for page := 0; page < s.MaxPages && pageURL != ""; page++ {
		response, err := session.Get(ctx, pageURL)
		if err != nil {
			return nil, fmt.Errorf("SSU list fetch: %w", err)
		}
		if c.Capture != nil {
			if e := c.Capture("list", response); e != nil {
				return nil, e
			}
		}
		rows, next, err := ParseSSUCatchList(response.Body, response.URL)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if seen[row.OriginalURL] {
				continue
			}
			seen[row.OriginalURL] = true
			if len(items) >= s.MaxContents {
				break
			}
			detail, err := session.Get(ctx, row.OriginalURL)
			if err != nil {
				return nil, fmt.Errorf("SSU detail fetch: %w", err)
			}
			if c.Capture != nil {
				if e := c.Capture("detail", detail); e != nil {
					return nil, e
				}
			}
			final, err := SSUCatchURL(detail.URL, detail.URL)
			if err != nil || final != row.OriginalURL {
				return nil, ssuError("detail_url", detail.URL, fmt.Errorf("unexpected detail redirect"))
			}
			item, err := ParseSSUCatchDetail(detail.Body, detail.URL, row, s.DetailBodySelector)
			if err != nil {
				return nil, err
			}
			now := time.Now().UTC()
			item.SourceID = s.ID
			item.SourceName = "ssu_catch"
			item.CrawledAt = now
			item.LastSeenAt = now
			item.UpdatedAt = now
			item.Deadline = nil
			item.ContentHash = model.ContentDigest(item)
			items = append(items, item)
		}
		if len(items) >= s.MaxContents {
			break
		}
		pageURL = next
	}
	return items, nil
}

// SSUCatchURL keeps only the slug identity, independent of list filters/pages.
// It rejects off-origin, duplicate/empty slug, credentials and non-notice paths.
func SSUCatchURL(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("malformed base URL")
	}
	u, err := b.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("malformed detail URL")
	}
	normalizeURL(b)
	normalizeURL(u)
	if u.User != nil || u.Scheme != b.Scheme || u.Host != b.Host || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("off-origin detail URL")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", fmt.Errorf("malformed detail query")
	}
	slugs := q["slug"]
	if len(slugs) != 1 || strings.TrimSpace(slugs[0]) == "" {
		return "", fmt.Errorf("one slug required")
	}
	if u.Path != ssuNoticePath && !strings.HasPrefix(u.Path, ssuNoticePath+"page/") {
		return "", fmt.Errorf("unexpected detail path")
	}
	u.Path = ssuNoticePath
	u.RawPath = ""
	u.RawQuery = url.Values{"slug": {slugs[0]}}.Encode()
	u.Fragment = ""
	return u.String(), nil
}
func SSUCategory(raw string) string { return model.NormalizeOfficialCategory(raw) }

func SSUDate(raw string) (*time.Time, error) {
	t, err := time.ParseInLocation("2006.01.02", model.NormalizeText(raw), time.FixedZone("Asia/Seoul", 9*60*60))
	if err != nil {
		return nil, err
	}
	t = t.UTC()
	return &t, nil
}
func ssudoc(raw []byte, stage, rawURL string) (*goquery.Document, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(raw))
	if err != nil {
		return nil, ssuError(stage, rawURL, err)
	}
	title := strings.ToLower(clean(doc.Find("title").Text()))
	if doc.Find("#challenge-form, .g-recaptcha, input[type='password'], [name='cf-turnstile-response']").Length() > 0 || strings.Contains(title, "captcha") || strings.Contains(title, "access denied") || strings.Contains(title, "로그인") {
		return nil, failure("access_restricted", stage, rawURL, 0, 0, fmt.Errorf("login or challenge detected"))
	}
	return doc, nil
}
func uniqueAdd(values []string, v string) []string {
	if v == "" {
		return values
	}
	for _, x := range values {
		if x == v {
			return values
		}
	}
	return append(values, v)
}

// Selectors below are supported by the unmodified captured list.actual.html.
func ParseSSUCatchList(raw []byte, base string) ([]model.Content, string, error) {
	doc, err := ssudoc(raw, "list_parse", base)
	if err != nil {
		return nil, "", err
	}
	if doc.Find("ul.notice-lists").Length() != 1 {
		return nil, "", ssuError("list_parse", base, fmt.Errorf("notice list container missing or ambiguous"))
	}
	rows := doc.Find("ul.notice-lists > li:not(.notice_head)")
	if rows.Length() == 0 {
		return nil, "", ssuError("list_parse", base, fmt.Errorf("notice rows missing"))
	}
	out := []model.Content{}
	index := map[string]int{}
	excludedURLs := map[string]bool{}
	rows.EachWithBreak(func(_ int, r *goquery.Selection) bool {
		a := r.Find(".notice_col3 a[href]")
		if a.Length() != 1 || r.Find(".notice_col1 .h2").Length() != 1 || r.Find(".notice_col2").Length() != 1 || r.Find(".notice_col2 .tag").Length() > 1 || r.Find(".notice_col4").Length() != 1 {
			err = fmt.Errorf("required list fields missing or ambiguous")
			return false
		}
		u, e := SSUCatchURL(base, a.AttrOr("href", ""))
		if e != nil {
			err = e
			return false
		}
		excluded := false
		a.Find(".label").Each(func(_ int, label *goquery.Selection) {
			if model.IsExcludedCategory(label.Text()) {
				excluded = true
			}
		})
		if excluded {
			excludedURLs[u] = true
			return true
		}
		if excludedURLs[u] {
			return true
		}
		titleNode := a.Clone()
		titleNode.Find(".label").Remove()
		title := clean(titleNode.Text())
		department := clean(r.Find(".notice_col4").Text())
		status := clean(r.Find(".notice_col2 .tag").Text())
		if status == "" {
			status = "unknown"
		}
		published, e := SSUDate(r.Find(".notice_col1 .h2").Text())
		if e != nil {
			err = fmt.Errorf("invalid notice date; expected YYYY.MM.DD")
			return false
		}
		if title == "" || department == "" {
			err = fmt.Errorf("required notice text empty")
			return false
		}
		originals, categories := []string{}, []string{}
		a.Find(".label").Each(func(_ int, label *goquery.Selection) {
			v := clean(label.Text())
			originals = uniqueAdd(originals, v)
			categories = uniqueAdd(categories, SSUCategory(v))
		})
		if len(categories) == 0 {
			categories = []string{"unknown"}
		}
		if i, ok := index[u]; ok {
			for _, v := range originals {
				out[i].OriginalCategories = uniqueAdd(out[i].OriginalCategories, v)
			}
			for _, v := range categories {
				out[i].Categories = uniqueAdd(out[i].Categories, v)
			}
			return true
		}
		original := ""
		if len(originals) > 0 {
			original = originals[0]
		}
		index[u] = len(out)
		out = append(out, model.Content{Title: title, OriginalURL: u, OriginalCategory: original, OriginalCategories: originals, Category: categories[0], Categories: categories, Department: department, Status: status, PublishedAt: published, Attachments: []model.Attachment{}, Tags: []string{}, Metadata: map[string]string{}})
		return true
	})
	if err != nil {
		return nil, "", ssuError("list_parse", base, err)
	}
	next := ""
	link := doc.Find("nav.board-pagination a.next-btn")
	if link.Length() > 1 {
		return nil, "", ssuError("pagination", base, fmt.Errorf("ambiguous next link"))
	}
	if link.Length() == 1 {
		b, _ := url.Parse(base)
		n, e := b.Parse(link.AttrOr("href", ""))
		if e != nil || n.Scheme != b.Scheme || !strings.EqualFold(n.Host, b.Host) || n.User != nil || !strings.HasPrefix(n.Path, ssuNoticePath+"page/") || n.Query().Get("slug") != "" {
			return nil, "", ssuError("pagination", base, fmt.Errorf("invalid next page URL"))
		}
		n.RawPath = ""
		n.Fragment = ""
		next = n.String()
	}
	eligible := out[:0]
	for _, item := range out {
		if !excludedURLs[item.OriginalURL] {
			eligible = append(eligible, item)
		}
	}
	return eligible, next, nil
}
func validateSSUSelector(selector string) error {
	switch strings.TrimSpace(selector) {
	case "", "html", "body", "main", "article", "*":
		return fmt.Errorf("verified, narrow detail_body_selector required")
	}
	if _, err := cascadia.Compile(selector); err != nil {
		return fmt.Errorf("invalid detail_body_selector CSS syntax")
	}
	return nil
}

// The caller supplies a reviewed selector. No real SSU detail selector has been
// inferred from the list template or substituted with a whole-page fallback.
func ParseSSUCatchDetail(raw []byte, base string, row model.Content, selector string) (model.Content, error) {
	if err := validateSSUSelector(selector); err != nil {
		return row, ssuError("detail_schema", base, err)
	}
	doc, err := ssudoc(raw, "detail_parse", base)
	if err != nil {
		return row, err
	}
	body := doc.Find(selector)
	if body.Length() != 1 {
		return row, ssuError("detail_parse", base, fmt.Errorf("detail body missing or ambiguous"))
	}
	// Record media presence without downloading it. Copy the map so parsing
	// does not mutate the caller's list metadata.
	metadata := map[string]string{}
	for k, v := range row.Metadata {
		metadata[k] = v
	}
	row.Metadata = metadata
	row.Metadata["has_images"] = "false"
	if body.Find("img").Length() > 0 {
		row.Metadata["has_images"] = "true"
	}
	row.Metadata["images_analyzed"] = "false"
	row.Metadata["attachments_analyzed"] = "false"
	body = body.Clone()
	body.Find("script,style,nav,footer,header,aside,form,noscript,.board-pagination").Remove()
	row.Attachments = []model.Attachment{}
	seen := map[string]bool{}
	body.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		b, e := url.Parse(base)
		if e != nil {
			return
		}
		u, e := b.Parse(a.AttrOr("href", ""))
		if e != nil {
			return
		}
		raw, e := model.NormalizeContentURL(u.String())
		if e != nil {
			return
		}
		name := clean(a.AttrOr("download", ""))
		if name == "" {
			name = clean(a.Text())
		}
		ext := strings.ToLower(path.Ext(u.Path))
		file := false
		for _, v := range []string{".pdf", ".hwp", ".hwpx", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".zip", ".jpg", ".jpeg", ".png"} {
			if ext == v || strings.HasSuffix(strings.ToLower(name), v) {
				file = true
			}
		}
		if strings.HasSuffix(u.Path, "/download.php") || a.Is("[download]") {
			file = true
		}
		if !file || seen[raw] {
			return
		}
		seen[raw] = true
		if name == "" {
			name = path.Base(u.Path)
		}
		row.Attachments = append(row.Attachments, model.Attachment{Name: name, URL: raw})
	})
	// Attachment labels belong to metadata, not the notice prose. This class
	// is verified in the user-supplied actual detail fixture.
	body.Find("ul.download-list").Remove()
	// Preserve block boundaries before whitespace normalization, including <br>.
	body.Find("br").ReplaceWithHtml("\n")
	body.Find("p,div,li,tr,h1,h2,h3,h4").Each(func(_ int, s *goquery.Selection) { s.AppendHtml("\n") })
	lines := []string{}
	for _, line := range strings.Split(body.Text(), "\n") {
		if v := clean(line); v != "" {
			lines = append(lines, v)
		}
	}
	row.Content = strings.Join(lines, "\n")
	if row.Content == "" {
		return row, ssuError("detail_parse", base, fmt.Errorf("empty notice body"))
	}
	return row, nil
}
