// Package ssudetail validates user-supplied, offline detail-page evidence.
// It never fetches a URL or enables the live source.
package ssudetail

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"
)

type Selectors struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	Department  string `json:"department"`
	Attachments string `json:"attachments"`
}
type Expected struct {
	Title               string             `json:"title"`
	Body                string             `json:"body"`
	Department          string             `json:"department"`
	OriginalCategory    string             `json:"original_category"`
	Category            string             `json:"category"`
	PublishedDate       string             `json:"published_date"`
	Status              string             `json:"status"`
	AttachmentAreaCount int                `json:"attachment_area_count"`
	Attachments         []model.Attachment `json:"attachments"`
}
type Evidence struct {
	SourceURL        string    `json:"source_url"`
	CapturedAt       string    `json:"captured_at"`
	CaptureNote      string    `json:"capture_note,omitempty"`
	DepartmentSource string    `json:"department_source,omitempty"`
	ListSHA256       string    `json:"list_sha256,omitempty"`
	SHA256           string    `json:"html_sha256"`
	Reviewed         bool      `json:"selectors_reviewed"`
	Selectors        Selectors `json:"selectors"`
	Expected         Expected  `json:"expected"`
}

// Load skips only when both actual files are absent. A partial submission fails.
// SSU_REQUIRE_ACTUAL_DETAIL=1 turns absence into a failure for acceptance/CI.
func Load(t *testing.T, dir string) model.Content {
	t.Helper()
	raw, htmlErr := os.ReadFile(filepath.Join(dir, "detail.actual.html"))
	manifest, metaErr := os.ReadFile(filepath.Join(dir, "detail.actual.expected.json"))
	if os.IsNotExist(htmlErr) && os.IsNotExist(metaErr) {
		if os.Getenv("SSU_REQUIRE_ACTUAL_DETAIL") == "1" {
			t.Fatal("actual detail evidence required but absent")
		}
		t.Skip("actual detail HTML and reviewed expectations absent; real selectors remain UNVERIFIED")
	}
	if htmlErr != nil || metaErr != nil {
		t.Fatalf("incomplete actual detail fixture: HTML=%v expectations=%v", htmlErr, metaErr)
	}
	var e Evidence
	decoder := json.NewDecoder(bytes.NewReader(manifest))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&e); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatal("expectations must contain exactly one JSON object")
	}
	var list []byte
	if e.DepartmentSource == "list.actual.html" {
		list, htmlErr = os.ReadFile(filepath.Join(dir, "list.actual.html"))
		if htmlErr != nil {
			t.Fatal(htmlErr)
		}
	}
	item, err := ValidateWithList(raw, e, list)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

// Validate compares independently reviewed DOM fields with the existing parser.
// Title and department are seeded from the checked detail nodes, while the live
// crawler still gets these fields from its list. This does not change that API.
func Validate(raw []byte, e Evidence) (model.Content, error) {
	return ValidateWithList(raw, e, nil)
}

func ValidateWithList(raw []byte, e Evidence, list []byte) (model.Content, error) {
	var zero model.Content
	fail := func(s string) (model.Content, error) { return zero, fmt.Errorf("actual detail evidence: %s", s) }
	digest := sha256.Sum256(raw)
	if !e.Reviewed || e.SHA256 != hex.EncodeToString(digest[:]) {
		return fail("review flag or HTML SHA256 mismatch")
	}
	if _, err := time.Parse(time.RFC3339, e.CapturedAt); err != nil && !(e.CapturedAt == "" && e.CaptureNote != "") {
		return fail("captured_at must be RFC3339")
	}
	canonical, err := crawler.SSUCatchURL(crawler.SSUCatchOrigin+"/공지사항/", e.SourceURL)
	if err != nil {
		return fail("invalid SSU notice source_url")
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(raw))
	if err != nil {
		return zero, err
	}
	nodes := map[string]*goquery.Selection{}
	for name, selector := range map[string]string{"title": e.Selectors.Title, "body": e.Selectors.Body, "department": e.Selectors.Department, "attachments": e.Selectors.Attachments} {
		if name == "department" && selector == "" && e.DepartmentSource == "list.actual.html" {
			continue
		}
		switch strings.TrimSpace(selector) {
		case "", "html", "body", "main", "article", "*":
			return fail(name + " requires a reviewed narrow selector")
		}
		matcher, err := cascadia.Compile(selector)
		if err != nil {
			return fail(name + " has invalid CSS syntax")
		}
		node := doc.FindMatcher(matcher)
		count := 1
		if name == "attachments" {
			count = e.Expected.AttachmentAreaCount
		}
		if count < 0 || count > 1 || node.Length() != count {
			return fail(name + " node count mismatch")
		}
		nodes[name] = node
	}
	want := e.Expected
	if want.Title == "" || want.Body == "" || want.Department == "" || want.OriginalCategory == "" || want.Status == "" || want.Attachments == nil {
		return fail("expected fields and explicit attachments array required")
	}
	title := model.NormalizeText(nodes["title"].Text())
	department := ""
	var listRow *model.Content
	if e.DepartmentSource == "list.actual.html" {
		if e.Selectors.Department != "" {
			return fail("list department must not claim a detail selector")
		}
		sum := sha256.Sum256(list)
		if len(list) == 0 || e.ListSHA256 != hex.EncodeToString(sum[:]) {
			return fail("list evidence missing or hash mismatch")
		}
		rows, _, err := crawler.ParseSSUCatchList(list, crawler.SSUCatchOrigin+"/공지사항/")
		if err != nil {
			return zero, err
		}
		for _, row := range rows {
			if row.OriginalURL == canonical {
				r := row
				listRow = &r
				break
			}
		}
		if listRow == nil {
			return fail("same slug missing from list evidence")
		}
		department = listRow.Department
	} else {
		if e.DepartmentSource != "" {
			return fail("unsupported department source")
		}
		department = model.NormalizeText(nodes["department"].Text())
	}
	if title != want.Title || department != want.Department {
		return fail("title or department differs from independently reviewed expectation")
	}
	if crawler.SSUCategory(want.OriginalCategory) != want.Category {
		return fail("category mapping mismatch")
	}
	published, err := crawler.SSUDate(want.PublishedDate)
	if err != nil {
		return fail("published_date must be YYYY.MM.DD")
	}
	row := model.Content{Title: title, Department: department, OriginalURL: canonical, SourceName: "ssu_catch", OriginalCategory: want.OriginalCategory, OriginalCategories: []string{want.OriginalCategory}, Category: want.Category, Categories: []string{want.Category}, Status: want.Status, PublishedAt: published}
	if listRow != nil {
		if listRow.Title != title || listRow.Department != want.Department || listRow.Category != want.Category || listRow.OriginalCategory != want.OriginalCategory || listRow.Status != want.Status || !listRow.PublishedAt.Equal(*published) {
			return fail("list/detail metadata mismatch")
		}
		row = *listRow
		row.SourceName = "ssu_catch"
	}
	item, err := crawler.ParseSSUCatchDetail(raw, e.SourceURL, row, e.Selectors.Body)
	if err != nil {
		return zero, err
	}
	if item.Content != want.Body {
		return fail("body differs from independently reviewed exact text")
	}
	// Check the attachment scope separately. If real attachments are outside the
	// body, the comparison below intentionally exposes the current parser gap.
	if len(want.Attachments) > 0 && want.AttachmentAreaCount != 1 {
		return fail("attachments require exactly one attachment area")
	}
	for _, attachment := range want.Attachments {
		found := false
		nodes["attachments"].Find("a[href]").AddSelection(nodes["body"].Find("a[href]:has(img)")).Each(func(_ int, a *goquery.Selection) {
			u, err := model.NormalizeContentURL(resolveURL(e.SourceURL, a.AttrOr("href", "")))
			if err == nil && u == attachment.URL {
				found = true
			}
		})
		if !found {
			return fail("expected attachment URL missing from reviewed area")
		}
	}
	sortAttachments := func(a []model.Attachment) []model.Attachment {
		v := append([]model.Attachment{}, a...)
		sort.Slice(v, func(i, j int) bool {
			if v[i].URL == v[j].URL {
				return v[i].Name < v[j].Name
			}
			return v[i].URL < v[j].URL
		})
		return v
	}
	if !reflect.DeepEqual(sortAttachments(item.Attachments), sortAttachments(want.Attachments)) {
		return fail("attachment metadata mismatch; check whether real attachment area is outside body")
	}
	now := time.Now().UTC()
	item.CrawledAt, item.LastSeenAt, item.UpdatedAt = now, now, now
	item.ContentHash = model.ContentDigest(item)
	return item, nil
}

func resolveURL(base, ref string) string {
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	u, err := b.Parse(ref)
	if err != nil {
		return ""
	}
	return u.String()
}
