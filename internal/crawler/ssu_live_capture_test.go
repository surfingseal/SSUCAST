package crawler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"example.com/crawlserver/internal/testutil/localfixture"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"example.com/crawlserver/internal/model"
	"github.com/PuerkitoBio/goquery"
)

// These are unmodified HTTP captures from an explicitly approved live run,
// replayed offline. No real school requests occur in ordinary/race tests.
func TestSSULiveCapturedRegression(t *testing.T) {
	dir := "testdata/ssu_catch/live_20261106"
	localfixture.Require(t, filepath.Join(dir, "run.json"))
	raw, e := os.ReadFile(filepath.Join(dir, "run.json"))
	if e != nil {
		t.Fatal(e)
	}
	var r struct {
		Mode      string              `json:"mode"`
		Error     string              `json:"error"`
		Requests  []RequestRecord     `json:"requests"`
		Items     []model.Content     `json:"items"`
		Excluded  []map[string]string `json:"excluded"`
		Artifacts []struct {
			Stage, File, URL, SHA256 string
			Status                   int
		} `json:"artifacts"`
	}
	if e = json.Unmarshal(raw, &r); e != nil {
		t.Fatal(e)
	}
	if r.Mode != "live_ssu_manual_user_approved" || r.Error != "" || len(r.Items) != 3 || len(r.Requests) != 5 || len(r.Excluded) != 5 {
		t.Fatal("unexpected actual capture provenance")
	}
	byStage := map[string][][]byte{}
	byURL := map[string][]byte{}
	for _, a := range r.Artifacts {
		b, e := os.ReadFile(filepath.Join(dir, a.File))
		if e != nil {
			t.Fatal(e)
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != a.SHA256 || a.Status != 200 {
			t.Fatal("actual capture changed")
		}
		byStage[a.Stage] = append(byStage[a.Stage], b)
		byURL[a.URL] = b
	}
	for i, v := range r.Requests {
		if v.Status != 200 {
			t.Fatal(v)
		}
		if i > 0 && v.StartedAt.Sub(r.Requests[i-1].StartedAt) < 2*time.Second {
			t.Fatal("live pacing below 2s")
		}
	}
	list := byStage["list"][0]
	rows, next, e := ParseSSUCatchList(list, SSUCatchOrigin+ssuNoticePath)
	if e != nil || len(rows) != 10 || !strings.HasSuffix(next, "/page/2/") {
		t.Fatal(len(rows), next, e)
	}
	for _, want := range r.Items {
		var row model.Content
		for _, v := range rows {
			if v.OriginalURL == want.OriginalURL {
				row = v
				break
			}
		}
		body := byURL[want.OriginalURL]
		got, e := ParseSSUCatchDetail(body, want.OriginalURL, row, "#contents .bg-white > hr + div")
		if e != nil {
			t.Fatal(e)
		}
		if got.Title != want.Title || got.Content != want.Content || got.OriginalCategory != want.OriginalCategory || got.Category != want.Category || !got.PublishedAt.Equal(*want.PublishedAt) || !reflect.DeepEqual(got.Attachments, want.Attachments) {
			t.Fatal("live replay differs")
		}
		if len(got.Content) < 100 || len(got.Attachments) != 1 {
			t.Fatal("live body/metadata missing")
		}
		doc, e := goquery.NewDocumentFromReader(bytes.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		if clean(doc.Find("#contents .bg-white > h1").Text()) != got.Title {
			t.Fatal("actual header title mismatch")
		}
		date, e := time.ParseInLocation("2006년 1월 2일", clean(doc.Find("#contents .bg-white > h1 + .clearfix > .float-left.mr-4").Text()), time.FixedZone("Asia/Seoul", 9*3600))
		if e != nil || !date.Equal(*got.PublishedAt) {
			t.Fatal("list/detail date mismatch")
		}
	}
	// A deliberate local mutation repeats an actual row. This verifies the same
	// identity handling needed for repeated pinned rows, without claiming that a
	// pinned notice or a second page was actually fetched.
	doc, e := goquery.NewDocumentFromReader(bytes.NewReader(list))
	if e != nil {
		t.Fatal(e)
	}
	rowHTML, e := goquery.OuterHtml(doc.Find("ul.notice-lists > li:not(.notice_head)").Eq(1))
	if e != nil {
		t.Fatal(e)
	}
	doc.Find("ul.notice-lists").AppendHtml(rowHTML)
	mutated, e := doc.Html()
	if e != nil {
		t.Fatal(e)
	}
	dedup, _, e := ParseSSUCatchList([]byte(mutated), SSUCatchOrigin+ssuNoticePath)
	if e != nil || len(dedup) != len(rows) {
		t.Fatal("repeated actual row was not deduplicated", e)
	}
}

func TestSSUEvidenceCaptureCannotRequestOutsideListScope(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); allow(w) }))
	defer srv.Close()
	f := NewFetcher(time.Second)
	f.FixtureOrigin = srv.URL
	observed := false
	logged := -1
	c := SSUCatchCrawler{Fetcher: f, Capture: func(string, Page) error { observed = true; return nil }, Requests: func(r []RequestRecord) { logged = len(r) }}
	_, err := c.Crawl(context.Background(), model.Source{BaseURL: srv.URL, ListURL: "/wrong-list", PolicyApproved: true, DetailBodySelector: ".reviewed-body"})
	code(t, err, "config")
	if observed || hits.Load() != 0 || logged != 0 {
		t.Fatal("capture hook weakened request scope", observed, hits.Load(), logged)
	}
}
