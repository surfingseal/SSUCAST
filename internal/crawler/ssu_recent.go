package crawler

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"example.com/crawlserver/internal/model"
	"github.com/PuerkitoBio/goquery"
)

type SSUReviewIssue struct {
	URL    string `json:"url"`
	Code   string `json:"code"`
	Reason string `json:"reason"`
}
type SSUCategoryReport struct {
	URL                 string           `json:"url"`
	VisitedPages        int              `json:"visited_pages"`
	ListNotices         int              `json:"list_notices"`
	DateEligible        int              `json:"date_eligible"`
	DateExcluded        int              `json:"date_excluded"`
	FutureExcluded      int              `json:"future_excluded"`
	DateInvalid         int              `json:"date_invalid"`
	RecruitmentExcluded int              `json:"recruitment_excluded"`
	Duplicates          int              `json:"duplicates"`
	DetailsRequested    int              `json:"details_requested"`
	Collected           int              `json:"collected"`
	NewStored           int              `json:"new_stored"`
	Updated             int              `json:"updated"`
	StopReason          string           `json:"stop_reason"`
	OrderVerified       bool             `json:"order_verified"`
	Issues              []SSUReviewIssue `json:"issues"`
}
type SSURecentReport struct {
	Source          string                        `json:"source"`
	Mode            string                        `json:"mode"`
	ExecutedAt      time.Time                     `json:"executed_at"`
	DateFrom        time.Time                     `json:"date_from"`
	DateTo          time.Time                     `json:"date_to"`
	Status          string                        `json:"status"`
	Categories      map[string]*SSUCategoryReport `json:"categories"`
	HTTPRequests    int                           `json:"http_requests"`
	VertexRequests  int                           `json:"vertex_requests"`
	UniqueCollected int                           `json:"unique_collected"`
	NewStored       int                           `json:"new_stored"`
	Updated         int                           `json:"updated"`
	Requests        []RequestRecord               `json:"requests,omitempty"`
	StopReason      string                        `json:"stop_reason,omitempty"`
}

func PreviewSSURecent(s model.Source, now time.Time) (SSURecentReport, error) {
	from, to := SSURecentRange(now)
	r := SSURecentReport{Source: s.Name, Mode: "preview", ExecutedAt: to, DateFrom: from, DateTo: to, Status: "preview_no_http_no_database", Categories: map[string]*SSUCategoryReport{}}
	for _, spec := range SSUOfficialCategories {
		u, e := SSUCategoryURL(s.BaseURL, spec)
		if e != nil {
			return r, e
		}
		r.Categories[spec.Code] = &SSUCategoryReport{URL: u, Issues: []SSUReviewIssue{}}
	}
	return r, nil
}

type recentRow struct {
	Item   model.Content
	Pinned bool
}
type recentPage struct {
	Blocked       []string
	OldCount      int
	FutureCount   int
	EligibleCount int
	Rows          []recentRow
	Next          string
	RawCount      int
	InvalidDates  int
	Excluded      int
	Issues        []SSUReviewIssue
	Fingerprint   string
	OrderDates    []*time.Time
}

// Only selectors observed in the actual SSU captures are used by default.
// pinned is an optional reviewed adapter (tests use explicitly synthetic attrs).
func parseRecentPage(raw []byte, base string, pinned func(*goquery.Selection) bool, bounds ...time.Time) (recentPage, error) {
	var p recentPage
	doc, e := ssudoc(raw, "recent_list", base)
	if e != nil {
		return p, e
	}
	if doc.Find("ul.notice-lists").Length() != 1 {
		return p, ssuError("recent_list", base, fmt.Errorf("notice list container missing/ambiguous"))
	}
	rows := doc.Find("ul.notice-lists > li:not(.notice_head)")
	p.RawCount = rows.Length()
	identities := []string{}
	rows.Each(func(_ int, row *goquery.Selection) {
		link := row.Find(".notice_col3 a[href]")
		identity, _ := SSUCatchURL(base, link.AttrOr("href", ""))
		identities = append(identities, identity)
		isPinned := pinned != nil && pinned(row)
		date, err := SSUDate(row.Find(".notice_col1 .h2").Text())
		if !isPinned {
			p.OrderDates = append(p.OrderDates, date)
		}
		excluded := false
		link.Find(".label").Each(func(_ int, l *goquery.Selection) {
			if model.IsExcludedCategory(l.Text()) {
				excluded = true
			}
		})
		if excluded {
			p.Excluded++
			p.Blocked = append(p.Blocked, identity)
			return
		}
		if err != nil {
			p.InvalidDates++
			p.Issues = append(p.Issues, SSUReviewIssue{identity, "invalid_date", clean(row.Find(".notice_col1 .h2").Text())})
			return
		}

		if len(bounds) == 2 {
			if date.Before(bounds[0]) {
				p.OldCount++
				return
			}
			if date.After(bounds[1]) {
				p.FutureCount++
				p.Issues = append(p.Issues, SSUReviewIssue{identity, "future_date", "published date after execution time"})
				return
			}
		}
		p.EligibleCount++
		html, err := goquery.OuterHtml(row)
		if err != nil {
			p.Issues = append(p.Issues, SSUReviewIssue{identity, "row_schema", err.Error()})
			return
		}
		items, _, err := ParseSSUCatchList([]byte("<ul class=\"notice-lists\">"+html+"</ul>"), base)
		if err != nil || len(items) != 1 {
			reason := "required fields missing"
			if err != nil {
				reason = err.Error()
			}
			p.Issues = append(p.Issues, SSUReviewIssue{identity, "row_schema", reason})
			return
		}
		p.Rows = append(p.Rows, recentRow{items[0], isPinned})
	})
	p.Fingerprint = fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(identities, "\n"))))
	next := doc.Find("nav.board-pagination a.next-btn")
	if next.Length() > 1 {
		return p, fmt.Errorf("ambiguous next link")
	}
	if next.Length() == 1 {
		p.Next = next.AttrOr("href", "")
	}
	return p, nil
}
func recentNext(base, ref, category string) (string, error) {
	b, e := url.Parse(base)
	if e != nil {
		return "", e
	}
	n, e := b.Parse(ref)
	if e != nil {
		return "", e
	}
	normalizeURL(b)
	normalizeURL(n)
	if n.User != nil || n.Host != b.Host || n.Scheme != b.Scheme || !strings.HasPrefix(n.Path, ssuNoticePath+"page/") {
		return "", fmt.Errorf("unexpected pagination route")
	}
	q, e := url.ParseQuery(n.RawQuery)
	if e != nil {
		return "", e
	}
	if len(q["category"]) != 1 || q.Get("category") != category || q.Get("slug") != "" {
		return "", fmt.Errorf("pagination lost/changed category")
	}
	for k := range q {
		if k != "category" && k != "f" && k != "keyword" {
			return "", fmt.Errorf("unreviewed pagination query %s", k)
		}
	}
	if q.Get("keyword") != "" {
		return "", fmt.Errorf("unexpected search query")
	}
	n.RawPath = ""
	n.RawQuery = q.Encode()
	n.Fragment = ""
	return n.String(), nil
}
func (c *SSUCatchCrawler) crawlRecent(ctx context.Context, s model.Source) (items []model.Content, err error) {
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	r, err := PreviewSSURecent(s, now)
	if err != nil {
		return nil, err
	}
	r.Mode = "manual"
	r.Status = "running"
	defer func() {
		r.UniqueCollected = len(items)
		if err != nil {
			r.Status = "incomplete"
			r.StopReason = err.Error()
		} else {
			r.Status = "completed"
			for _, v := range r.Categories {
				if len(v.Issues) > 0 {
					r.Status = "completed_with_review"
				}
			}
		}
		if c.RecentReport != nil {
			c.RecentReport(r)
		}
	}()
	if !s.PolicyApproved {
		return nil, failure("policy_unreviewed", "policy", s.BaseURL, 0, 0, fmt.Errorf("Source approval required"))
	}
	if s.ClassificationMode() != "source" {
		return nil, fmt.Errorf("SSU recent mode requires source strategy")
	}
	if err = validateSSUSelector(s.DetailBodySelector); err != nil {
		return nil, err
	}
	if c.Fetcher == nil {
		return nil, fmt.Errorf("Fetcher required")
	}
	fixtureInterval := s.RequestIntervalMS
	s = s.WithDefaults()
	s.MaxConcurrency = 1
	s.RequestIntervalMS = max(s.RequestIntervalMS, 2000)
	// Local Fixture tests retain their explicit short pacing; real origins always >=2s.
	if c.Fetcher.FixtureOrigin != "" && s.BaseURL == c.Fetcher.FixtureOrigin {
		s.RequestIntervalMS = max(1, fixtureInterval)
	}
	session, e := c.Fetcher.NewSession(s)
	if e != nil {
		return nil, e
	}
	defer session.Close()
	defer func() {
		r.Requests = session.RequestLog()
		r.HTTPRequests = len(r.Requests)
		if c.Requests != nil {
			c.Requests(r.Requests)
		}
	}()
	origin, _ := url.Parse(session.ListURL())
	if origin.Scheme+"://"+origin.Host != SSUCatchOrigin && origin.Scheme+"://"+origin.Host != c.Fetcher.FixtureOrigin {
		return nil, fmt.Errorf("unexpected SSU origin")
	}
	robots, e := session.Robots(ctx)
	if c.Capture != nil && len(robots.Body) > 0 {
		if x := c.Capture("robots", robots); x != nil {
			return nil, x
		}
	}
	if e != nil {
		return nil, e
	}
	index := map[string]int{}
	excludedURLs := map[string]bool{}
	seenLists := map[string]bool{}
	publish := func() {
		r.UniqueCollected = len(items)
		r.HTTPRequests = session.Requests()
		if c.RecentReport != nil {
			c.RecentReport(r)
		}
	}
	merge := func(i int, row model.Content) {
		for _, v := range row.OriginalCategories {
			items[i].OriginalCategories = uniqueAdd(items[i].OriginalCategories, v)
		}
		for _, v := range row.Categories {
			items[i].Categories = uniqueAdd(items[i].Categories, v)
		}
	}
	for _, spec := range SSUOfficialCategories {
		stats := r.Categories[spec.Code]
		pageURL := stats.URL
		fingerprints := map[string]bool{}
		ordered := true
		var last *time.Time
		for pageNo := 0; pageURL != ""; pageNo++ {
			if pageNo >= s.MaxPages {
				stats.StopReason = "max_pages"
				return items, fmt.Errorf("%s: per-category max_pages reached", spec.Code)
			}
			if seenLists[pageURL] {
				stats.StopReason = "cycle_link"
				return items, fmt.Errorf("%s: cyclic/repeated list URL", spec.Code)
			}
			seenLists[pageURL] = true
			response, e := session.Get(ctx, pageURL)
			if e != nil {
				stats.StopReason = "list_fetch"
				return items, e
			}
			stats.VisitedPages++
			if c.Capture != nil {
				if e = c.Capture("list_"+spec.Code, response); e != nil {
					return items, e
				}
			}
			p, e := parseRecentPage(response.Body, response.URL, c.RecentPinned, r.DateFrom, r.DateTo)
			if e != nil {
				stats.StopReason = "list_schema"
				return items, e
			}
			if p.RawCount > 0 && fingerprints[p.Fingerprint] {
				stats.StopReason = "repeated_page"
				return items, fmt.Errorf("%s: repeated page content", spec.Code)
			}
			fingerprints[p.Fingerprint] = true
			for _, u := range p.Blocked {
				excludedURLs[u] = true
			}
			stats.ListNotices += p.RawCount
			stats.DateInvalid += p.InvalidDates
			stats.DateExcluded += p.OldCount + p.FutureCount
			stats.FutureExcluded += p.FutureCount
			stats.DateEligible += p.EligibleCount
			stats.RecruitmentExcluded += p.Excluded
			stats.Issues = append(stats.Issues, p.Issues...)
			if p.RawCount == 0 {
				stats.StopReason = "empty_page"
				publish()
				break
			}
			allOld := len(p.OrderDates) > 0
			for _, date := range p.OrderDates {
				if date == nil {
					ordered = false
					allOld = false
					continue
				}
				if last != nil && date.After(*last) {
					ordered = false
				}
				if !date.Before(r.DateFrom) {
					allOld = false
				}
				last = date
			}
			stats.OrderVerified = ordered && stats.VisitedPages >= 2 && len(p.Issues) == 0
			for _, row := range p.Rows {
				item := row.Item
				if item.PublishedAt.Before(r.DateFrom) {
					stats.DateExcluded++
					continue
				}
				if item.PublishedAt.After(r.DateTo) {
					stats.DateExcluded++
					stats.FutureExcluded++
					stats.Issues = append(stats.Issues, SSUReviewIssue{item.OriginalURL, "future_date", "published date after execution time"})
					continue
				}
				if excludedURLs[item.OriginalURL] {
					stats.RecruitmentExcluded++
					continue
				}
				if i, ok := index[item.OriginalURL]; ok {
					stats.Duplicates++
					merge(i, item)
					if c.RecentItem != nil {
						if e = c.RecentItem(items[i]); e != nil {
							return items, e
						}
					}
					continue
				}
				if len(items) >= s.MaxContents {
					stats.StopReason = "max_contents"
					return items, fmt.Errorf("max_contents reached")
				}
				detail, e := session.Get(ctx, item.OriginalURL)
				stats.DetailsRequested++
				if e != nil {
					stats.Issues = append(stats.Issues, SSUReviewIssue{item.OriginalURL, "detail_fetch", e.Error()})
					publish()
					var fe *FetchError
					if ctx.Err() != nil || errors.As(e, &fe) && (fe.Status == 401 || fe.Status == 403 || fe.Status == 429 || fe.Code == "request_limit") {
						stats.StopReason = "access_or_budget_stop"
						return items, e
					}
					continue
				}
				final, e := SSUCatchURL(detail.URL, detail.URL)
				if e != nil || final != item.OriginalURL {
					return items, fmt.Errorf("unexpected detail redirect")
				}
				if c.Capture != nil {
					if e = c.Capture("detail_"+spec.Code, detail); e != nil {
						return items, e
					}
				}
				d, e := ssudoc(detail.Body, "recent_detail", detail.URL)
				if e != nil {
					stats.Issues = append(stats.Issues, SSUReviewIssue{item.OriginalURL, "detail_schema", e.Error()})
					continue
				}
				// Captured real SSU details have no official category label in their header.
				// Only a reviewed optional label selector may extend this evidence.
				if c.RecentDetailCategory != "" {
					d.Find(c.RecentDetailCategory).Each(func(_ int, n *goquery.Selection) {
						v := clean(n.Text())
						if model.IsExcludedCategory(v) {
							excludedURLs[item.OriginalURL] = true
						}
						if model.NormalizeOfficialCategory(v) != item.Category {
							stats.Issues = append(stats.Issues, SSUReviewIssue{item.OriginalURL, "category_mismatch", v})
						}
						item.OriginalCategories = uniqueAdd(item.OriginalCategories, v)
						item.Categories = uniqueAdd(item.Categories, SSUCategory(v))
					})
				}
				if excludedURLs[item.OriginalURL] {
					stats.RecruitmentExcluded++
					continue
				}
				parsed, e := ParseSSUCatchDetail(detail.Body, detail.URL, item, s.DetailBodySelector)
				if e != nil {
					stats.Issues = append(stats.Issues, SSUReviewIssue{item.OriginalURL, "detail_parse", e.Error()})
					publish()
					continue
				}
				if why := model.CollectionExclusion(parsed); why != nil {
					excludedURLs[item.OriginalURL] = true
					stats.RecruitmentExcluded++
					stats.Issues = append(stats.Issues, SSUReviewIssue{item.OriginalURL, "excluded_" + why.Code, why.Rule})
					continue
				}
				parsed.ImageURLs = []string{}
				d.Find(s.DetailBodySelector + " img[src]").Each(func(_ int, img *goquery.Selection) {
					base, _ := url.Parse(detail.URL)
					u, e := base.Parse(img.AttrOr("src", ""))
					if e == nil {
						if n, e := model.NormalizeContentURL(u.String()); e == nil {
							parsed.ImageURLs = uniqueAdd(parsed.ImageURLs, n)
						}
					}
				})
				collectedAt := time.Now().UTC()
				parsed.SourceID = s.ID
				parsed.SourceName = s.Name
				parsed.CrawledAt = collectedAt
				parsed.UpdatedAt = collectedAt
				parsed.LastSeenAt = collectedAt
				parsed.Metadata["data_origin"] = "http"
				parsed.Metadata["discovery_category"] = spec.Code
				if e = model.PrepareClassification(s, &parsed); e != nil {
					return items, e
				}
				parsed.ContentHash = model.ContentDigest(parsed)
				index[parsed.OriginalURL] = len(items)
				items = append(items, parsed)
				stats.Collected++
				if c.RecentItem != nil {
					if e = c.RecentItem(parsed); e != nil {
						return items, e
					}
				}
				publish()
			}
			if p.Next == "" {
				stats.StopReason = "no_next"
				publish()
				break
			}
			if stats.OrderVerified && allOld {
				stats.StopReason = "all_general_old_observed_descending"
				publish()
				break
			}
			pageURL, e = recentNext(response.URL, p.Next, spec.Query)
			if e != nil {
				stats.StopReason = "pagination_schema"
				return items, e
			}
			publish()
		}
	}
	// Remove notices whose exclusion was discovered later in another category.
	eligible := items[:0]
	for _, v := range items {
		if !excludedURLs[v.OriginalURL] {
			eligible = append(eligible, v)
		}
	}
	items = eligible
	if len(items) == 0 {
		total := 0
		for _, v := range r.Categories {
			total += v.ListNotices
		}
		if total == 0 {
			return items, fmt.Errorf("all category lists empty; review required")
		}
	}
	return items, nil
}
