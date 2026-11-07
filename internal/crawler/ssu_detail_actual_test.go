package crawler_test

import (
	"bytes"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/testutil/ssudetail"
	"os"
	"testing"
)

func TestSSUActualDetailFixture(t *testing.T) {
	ssudetail.Load(t, "testdata/ssu_catch")
}

func TestSSUActualDetailRejectsMissingOrAmbiguousBody(t *testing.T) {
	item := ssudetail.Load(t, "testdata/ssu_catch")
	raw, err := os.ReadFile("testdata/ssu_catch/detail.actual.html")
	if err != nil {
		t.Fatal(err)
	}
	const selector = "#contents .bg-white > hr + div"
	// Removing the observed structural boundary must not silently fall back to
	// menu/footer content. Duplication must not combine two notices.
	missing := bytes.Replace(raw, []byte(`<hr class="mb-4 mt-4" />`), []byte(`<span></span>`), 1)
	duplicate := append(append([]byte{}, raw...), raw...)
	for name, html := range map[string][]byte{"missing": missing, "duplicate": duplicate} {
		t.Run(name, func(t *testing.T) {
			if _, err := crawler.ParseSSUCatchDetail(html, item.OriginalURL, item, selector); err == nil {
				t.Fatal("invalid body accepted")
			}
		})
	}
}

func TestSSUOfflineImporterUsesReviewedParser(t *testing.T) {
	want := ssudetail.Load(t, "testdata/ssu_catch")
	got, err := crawler.ReadSSULocalFixture("testdata/ssu_catch")
	if err != nil || got.Title != want.Title || got.Department != want.Department || got.Content != want.Content || got.Category != want.Category {
		t.Fatal(got, err)
	}
}
