// Package mediafixture loads explicitly synthetic local department payloads.
package mediafixture

import (
	"encoding/json"
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func Load(t *testing.T, dir string) model.Content {
	t.Helper()
	read := func(name string) []byte {
		b, e := os.ReadFile(filepath.Join(dir, "synthetic", name))
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	var review crawler.GlobalMediaReview
	if e := json.Unmarshal(read("review.json"), &review); e != nil {
		t.Fatal(e)
	}
	rows, _, e := crawler.ParseGlobalMediaList(read("list.page0.json"))
	if e != nil {
		t.Fatal(e)
	}
	item, e := crawler.ParseGlobalMediaDetail(read("detail.1001.json"), "https://fixture.invalid/board/notices/1001?previous_page=2", rows[0], review)
	if e != nil {
		t.Fatal(e)
	}
	return item
}
