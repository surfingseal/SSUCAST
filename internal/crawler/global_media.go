package crawler

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"example.com/crawlserver/internal/model"
	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"
)

// Contract verified against actual linked JavaScript, public API captures and
// the rendered website on 2026-11-07. Source policy remains a separate gate.
//
//go:embed global_media_review.json
var mediaReviewJSON []byte

type GlobalMediaReview struct {
	WebsiteOrigin      string `json:"website_origin,omitempty"`
	AttachmentIDsOnly  bool   `json:"attachment_ids_only,omitempty"`
	AttachmentSelector string `json:"attachment_selector,omitempty"`

	Reviewed                bool              `json:"reviewed"`
	EvidenceKind            string            `json:"evidence_kind"`
	Evidence                string            `json:"evidence"`
	APIOrigin               string            `json:"api_origin"`
	ListPath                string            `json:"list_path"`
	MenuID                  string            `json:"menu_id"`
	PageSize                int               `json:"page_size"`
	DetailPath              string            `json:"detail_path"`
	DetailFields            map[string]string `json:"detail_fields"`
	BodyFormat              string            `json:"body_format"`
	BodySelector            string            `json:"body_selector"`
	AttachmentsPointer      string            `json:"attachments_pointer"`
	AttachmentFields        map[string]string `json:"attachment_fields"`
	CategoryPointer         string            `json:"category_pointer"`
	CategoryAbsenceVerified bool              `json:"category_absence_verified"`
}

type GlobalMediaCrawler struct {
	Since        time.Time
	Capture      func(string, Page)
	RequestAudit func([]RequestRecord)
	SkipAudit    func(string, string)
	Fetcher      *Fetcher
	Review       *GlobalMediaReview
	// Audit is optional; default structured logging records only URL and rule.
	Audit func(string, model.ExclusionReason)
}

func (c *GlobalMediaCrawler) review(s model.Source) (GlobalMediaReview, error) {
	var r GlobalMediaReview
	if c.Review != nil {
		r = *c.Review
	} else if err := json.Unmarshal(mediaReviewJSON, &r); err != nil {
		return r, err
	}
	if !r.Reviewed || r.Evidence == "" || (r.EvidenceKind != "actual" && r.EvidenceKind != "synthetic") {
		return r, fmt.Errorf("actual API origin and detail response schema have not been reviewed")
	}
	origin, err := model.NormalizeContentURL(r.APIOrigin)
	if err != nil || (strings.TrimRight(origin, "/") != strings.TrimRight(s.BaseURL, "/") && (r.EvidenceKind != "actual" || r.WebsiteOrigin != strings.TrimRight(s.BaseURL, "/") || r.APIOrigin != "https://3-37-127-112.sslip.io" || r.WebsiteOrigin != "https://media.ssu.ac.kr")) {
		return r, fmt.Errorf("reviewed API must use source origin; cross-origin API requires a separate reviewed Fetcher policy")
	}
	if r.EvidenceKind == "synthetic" && (c.Fetcher == nil || c.Fetcher.FixtureOrigin == "" || c.Fetcher.FixtureOrigin != r.APIOrigin) {
		return r, fmt.Errorf("synthetic contracts are restricted to the local Fixture origin")
	}
	if !strings.HasPrefix(r.ListPath, "/") || strings.HasPrefix(r.ListPath, "//") || strings.ContainsAny(r.ListPath, "?#") || r.MenuID == "" || r.PageSize < 1 || r.PageSize > 100 {
		return r, fmt.Errorf("invalid reviewed list contract")
	}
	if !strings.HasPrefix(r.DetailPath, "/") || strings.HasPrefix(r.DetailPath, "//") || strings.ContainsAny(r.DetailPath, "?#") || strings.Count(r.DetailPath, "{id}") != 1 {
		return r, fmt.Errorf("verified detail endpoint required")
	}
	for _, field := range []string{"id", "title", "content", "author", "published_at"} {
		if !validPointer(r.DetailFields[field]) {
			return r, fmt.Errorf("missing reviewed detail pointer: %s", field)
		}
	}
	if r.CategoryPointer == "" && !r.CategoryAbsenceVerified {
		return r, fmt.Errorf("category presence/absence must be reviewed")
	}
	if r.CategoryPointer != "" && !validPointer(r.CategoryPointer) {
		return r, fmt.Errorf("invalid category pointer")
	}
	if !validPointer(r.AttachmentsPointer) {
		return r, fmt.Errorf("reviewed attachments pointer required")
	}
	for _, field := range []string{"name", "url"} {
		if r.AttachmentIDsOnly {
			continue
		}
		if !validPointer(r.AttachmentFields[field]) {
			return r, fmt.Errorf("reviewed attachment fields required")
		}
	}
	switch r.BodyFormat {
	case "html":
		if err := validateSSUSelector(r.BodySelector); err != nil {
			return r, err
		}
	case "text", "editor_text_nodes":
	default:
		return r, fmt.Errorf("reviewed body encoding required")
	}
	return r, nil
}
func validPointer(p string) bool { return strings.HasPrefix(p, "/") && !strings.Contains(p, "//") }

// JSON pointers describe reviewed response fields, not guessed production DTOs.
func mediaValue(v any, p string) (any, error) {
	if !validPointer(p) {
		return nil, fmt.Errorf("invalid JSON pointer")
	}
	for _, key := range strings.Split(p[1:], "/") {
		key = strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~")
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("missing object at %s", p)
		}
		v, ok = m[key]
		if !ok {
			return nil, fmt.Errorf("missing field at %s", p)
		}
	}
	return v, nil
}
func mediaString(v any, p string) (string, error) {
	v, err := mediaValue(v, p)
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("expected string at %s", p)
	}
	return s, nil
}

var mediaID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func mediaIdentifier(v any) (string, error) {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case json.Number:
		s = x.String()
	default:
		return "", fmt.Errorf("invalid board ID")
	}
	if !mediaID.MatchString(s) || s == "new" {
		return "", fmt.Errorf("invalid board ID")
	}
	return s, nil
}

// GlobalMediaURL follows the route observed in the client. All query parameters
// are list navigation state; board ID is the sole identity.
func GlobalMediaURL(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u, err := b.Parse(ref)
	if err != nil {
		return "", err
	}
	if u.User != nil || u.Scheme != b.Scheme || !strings.EqualFold(u.Host, b.Host) || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("off-origin detail URL")
	}
	if !strings.HasPrefix(u.Path, "/board/notices/") || u.RawPath != "" {
		return "", fmt.Errorf("unexpected board route")
	}
	id := strings.TrimPrefix(u.Path, "/board/notices/")
	if _, err := mediaIdentifier(id); err != nil {
		return "", err
	}
	u.RawQuery = ""
	u.Fragment = ""
	return model.NormalizeContentURL(u.String())
}

type GlobalMediaRow struct {
	ID                                string
	AccessRole                        string
	Title, Author, CreatedAt, Content string
	Pinned                            bool
}

// Envelope/fields are observed in client code. Actual response validation is
// still required before this parser can be enabled against the live service.
func ParseGlobalMediaList(raw []byte) ([]GlobalMediaRow, int, error) {
	var envelope struct {
		Data *struct {
			Boards []struct {
				AccessRole string          `json:"accessRole"`
				ID         json.RawMessage `json:"id"`
				Title      *string         `json:"title"`
				UserName   *string         `json:"userName"`
				CreatedAt  *string         `json:"createdAt"`
				Content    *string         `json:"content"`
				IsPinned   *bool           `json:"isPinned"`
			} `json:"boards"`
			TotalPage *int `json:"totalPage"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, 0, fmt.Errorf("list must be a JSON API response: %w", err)
	}
	if envelope.Data == nil || envelope.Data.Boards == nil || envelope.Data.TotalPage == nil || *envelope.Data.TotalPage < 0 {
		return nil, 0, fmt.Errorf("list envelope missing")
	}
	rows := []GlobalMediaRow{}
	for _, b := range envelope.Data.Boards {
		if b.Title == nil || b.UserName == nil || b.CreatedAt == nil || b.Content == nil || b.IsPinned == nil {
			return nil, 0, fmt.Errorf("list row fields missing")
		}
		var idValue any
		d := json.NewDecoder(bytes.NewReader(b.ID))
		d.UseNumber()
		if err := d.Decode(&idValue); err != nil {
			return nil, 0, err
		}
		id, err := mediaIdentifier(idValue)
		if err != nil {
			return nil, 0, err
		}
		if strings.TrimSpace(*b.Title) == "" {
			return nil, 0, fmt.Errorf("empty title")
		}
		rows = append(rows, GlobalMediaRow{ID: id, AccessRole: b.AccessRole, Title: *b.Title, Author: *b.UserName, CreatedAt: *b.CreatedAt, Content: *b.Content, Pinned: *b.IsPinned})
	}
	return rows, *envelope.Data.TotalPage, nil
}
func mediaDate(s string) (*time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		t, err := time.ParseInLocation(layout, s, time.FixedZone("Asia/Seoul", 9*3600))
		if err == nil {
			t = t.UTC()
			return &t, nil
		}
	}
	return nil, fmt.Errorf("unsupported published_at format")
}
func mediaBody(raw string, r GlobalMediaReview) (string, bool, error) {
	if r.BodyFormat == "text" {
		return strings.TrimSpace(raw), false, nil
	}
	if r.BodyFormat == "html" {
		if _, err := cascadia.Compile(r.BodySelector); err != nil {
			return "", false, err
		}
		d, err := goquery.NewDocumentFromReader(strings.NewReader(raw))
		if err != nil {
			return "", false, err
		}
		body := d.Find(r.BodySelector)
		if body.Length() != 1 {
			return "", false, fmt.Errorf("body selector missing or ambiguous")
		}
		images := body.Find("img").Length() > 0
		body = body.Clone()
		body.Find("script,style,nav,header,footer,aside,form,noscript").Remove()
		body.Find("br").ReplaceWithHtml("\n")
		body.Find("p,div,li,h1,h2,h3").Each(func(_ int, n *goquery.Selection) { n.AppendHtml("\n") })
		return strings.TrimSpace(body.Text()), images, nil
	}
	// Optional editor adapter is enabled only by a reviewed schema. Decode the
	// observed double JSON encoding, collect text fields in tree order; ignore src.
	var v any = raw
	for i := 0; i < 3; i++ {
		s, ok := v.(string)
		if !ok {
			break
		}
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			return "", false, fmt.Errorf("invalid editor JSON")
		}
	}
	images := false
	var render func(any) string
	render = func(v any) string {
		switch n := v.(type) {
		case []any:
			var buf strings.Builder
			for _, child := range n {
				buf.WriteString(render(child))
			}
			return buf.String()
		case map[string]any:
			if src, ok := n["src"].(string); ok && src != "" {
				images = true
			}
			if child, ok := n["editorState"]; ok {
				return render(child)
			}
			if child, ok := n["root"]; ok {
				return render(child)
			}
			if text, ok := n["text"].(string); ok {
				return text
			}
			if typ, _ := n["type"].(string); typ == "linebreak" {
				return "\n"
			}
			text := render(n["children"])
			typ, _ := n["type"].(string)
			switch typ {
			case "paragraph", "heading", "listitem", "tablerow", "tablecell":
				return text + "\n"
			}
			return text
		}
		return ""
	}
	return strings.TrimSpace(render(v)), images, nil
}
func ParseGlobalMediaDetail(raw []byte, originalURL string, row GlobalMediaRow, r GlobalMediaReview) (model.Content, error) {
	var v any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return model.Content{}, err
	}
	if d.Decode(new(any)) != io.EOF {
		return model.Content{}, fmt.Errorf("trailing detail JSON")
	}
	if r.WebsiteOrigin != "" {
		role, e := mediaString(v, "/data/accessRole")
		if e != nil || role != "ROLE_GUEST" {
			return model.Content{}, fmt.Errorf("detail not publicly readable")
		}
	}
	idValue, err := mediaValue(v, r.DetailFields["id"])
	if err != nil {
		return model.Content{}, err
	}
	id, err := mediaIdentifier(idValue)
	if err != nil || id != row.ID {
		return model.Content{}, fmt.Errorf("detail ID mismatch")
	}
	read := func(k string) (string, error) { return mediaString(v, r.DetailFields[k]) }
	title, err := read("title")
	if err != nil {
		return model.Content{}, err
	}
	bodyRaw, err := read("content")
	if err != nil {
		return model.Content{}, err
	}
	if bodyRaw == "<redacted>" {
		return model.Content{}, fmt.Errorf("restricted detail content")
	}
	author, err := read("author")
	if err != nil {
		return model.Content{}, err
	}
	dateRaw, err := read("published_at")
	if err != nil {
		return model.Content{}, err
	}
	date, err := mediaDate(dateRaw)
	if err != nil {
		return model.Content{}, err
	}
	body, images, err := mediaBody(bodyRaw, r)
	if err != nil {
		return model.Content{}, err
	}
	if clean(title) == "" || clean(body) == "" {
		return model.Content{}, fmt.Errorf("required detail text missing")
	}
	category := ""
	if r.CategoryPointer != "" {
		category, err = mediaString(v, r.CategoryPointer)
		if err != nil {
			return model.Content{}, err
		}
	}
	originalURL, err = GlobalMediaURL(originalURL, originalURL)
	if err != nil {
		return model.Content{}, err
	}
	item := model.Content{Title: clean(title), Author: clean(author), Content: body, OriginalURL: originalURL, OriginalCategory: clean(category), PublishedAt: date, Attachments: []model.Attachment{}, Tags: []string{}, Metadata: map[string]string{"fixture_kind": r.EvidenceKind, "parser_evidence": r.Evidence, "board_id": id, "is_pinned": strconv.FormatBool(row.Pinned), "has_images": strconv.FormatBool(images), "images_analyzed": "false", "attachments_analyzed": "false"}}
	files, err := mediaValue(v, r.AttachmentsPointer)
	if err != nil {
		return item, err
	}
	if r.AttachmentIDsOnly {
		if files == nil {
			return item, nil
		}
		arr, ok := files.([]any)
		if !ok {
			return item, fmt.Errorf("attachment ID list invalid")
		}
		item.Metadata["expected_attachments"] = strconv.Itoa(len(arr))
		rawIDs, _ := json.Marshal(arr)
		item.Metadata["attachment_records"] = string(rawIDs)
		return item, nil
	}
	attachments, ok := files.([]any)
	if !ok {
		return item, fmt.Errorf("attachments must be an array")
	}
	seen := map[string]bool{}
	base, _ := url.Parse(originalURL)
	for _, file := range attachments {
		name, err := mediaString(file, r.AttachmentFields["name"])
		if err != nil {
			return item, err
		}
		link, err := mediaString(file, r.AttachmentFields["url"])
		if err != nil {
			return item, err
		}
		u, err := base.Parse(link)
		if err != nil {
			return item, err
		}
		normalized, err := model.NormalizeContentURL(u.String())
		if err != nil {
			return item, err
		}
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		ext := strings.ToLower(path.Ext(name))
		if ext == "" {
			ext = strings.ToLower(path.Ext(u.Path))
		}
		item.Attachments = append(item.Attachments, model.Attachment{Name: clean(name), URL: normalized, Extension: strings.TrimPrefix(ext, ".")})
	}
	return item, nil
}
func (c *GlobalMediaCrawler) exclude(u string, v model.Content) bool {
	reason := model.CollectionExclusion(v)
	if reason == nil {
		return false
	}
	if c.Audit != nil {
		c.Audit(u, *reason)
	} else {
		slog.Info("notice excluded", "url", u, "reason", reason.Code, "field", reason.Field, "rule", reason.Rule)
	}
	return true
}
func (c *GlobalMediaCrawler) Crawl(ctx context.Context, s model.Source) ([]model.Content, error) {
	if !s.PolicyApproved {
		return nil, failure("policy_unreviewed", "policy", s.BaseURL, 0, 0, fmt.Errorf("department collection policy remains unverified"))
	}
	r, err := c.review(s)
	if err != nil {
		return nil, failure("schema_unverified", "api_schema", s.BaseURL, 0, 0, err)
	}
	if c.Fetcher == nil {
		return nil, fmt.Errorf("shared Fetcher required")
	}
	s = s.WithDefaults()
	if r.APIOrigin != strings.TrimRight(s.BaseURL, "/") {
		return c.crawlPublicAPI(ctx, s, r)
	}
	session, err := c.Fetcher.NewSession(s)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	seen := map[string]bool{}
	items := []model.Content{}
	for page := 0; page < s.MaxPages; page++ {
		u, err := session.Resolve(s.BaseURL, r.ListPath)
		if err != nil {
			return nil, err
		}
		parsed, _ := url.Parse(u)
		q := parsed.Query()
		q.Set("menuId", r.MenuID)
		q.Set("size", strconv.Itoa(r.PageSize))
		q.Set("page", strconv.Itoa(page))
		q.Set("content", "")
		parsed.RawQuery = q.Encode()
		response, err := session.Get(ctx, parsed.String())
		if err != nil {
			return nil, err
		}
		rows, total, err := ParseGlobalMediaList(response.Body)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if seen[row.ID] {
				continue
			}
			seen[row.ID] = true
			original, err := GlobalMediaURL(s.BaseURL, "/board/notices/"+row.ID)
			if err != nil {
				return nil, err
			}
			if row.Content == "<redacted>" {
				slog.Info("restricted notice skipped", "url", original)
				continue
			}
			if c.exclude(original, model.Content{Title: row.Title}) {
				continue
			}
			detailPath := strings.ReplaceAll(r.DetailPath, "{id}", url.PathEscape(row.ID))
			detailURL, err := session.Resolve(s.BaseURL, detailPath)
			if err != nil {
				return nil, err
			}
			detail, err := session.Get(ctx, detailURL)
			if err != nil {
				return nil, err
			}
			if detail.URL != detailURL {
				return nil, fmt.Errorf("unexpected detail API redirect")
			}
			item, err := ParseGlobalMediaDetail(detail.Body, original, row, r)
			if err != nil {
				return nil, err
			}
			if c.exclude(original, item) {
				continue
			}
			now := time.Now().UTC()
			item.SourceID = s.ID
			item.SourceName = s.Name
			item.CrawledAt = now
			item.LastSeenAt = now
			item.UpdatedAt = now
			if err := model.PrepareClassification(s, &item); err != nil {
				return nil, err
			}
			item.ContentHash = model.ContentDigest(item)
			items = append(items, item)
			if len(items) >= s.MaxContents {
				return items, nil
			}
		}
		if page+1 >= total {
			break
		}
	}
	return items, nil
}
