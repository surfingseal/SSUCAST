package crawler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"example.com/crawlserver/internal/model"
)

// ReadSSULocalFixture reads reviewed local evidence without creating a Fetcher
// session, approving a policy, or downloading images/attachments.
func ReadSSULocalFixture(dir string) (model.Content, error) {
	var zero model.Content
	read := func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(dir, name)) }
	raw, err := read("detail.actual.html")
	if err != nil {
		return zero, err
	}
	meta, err := read("detail.actual.expected.json")
	if err != nil {
		return zero, err
	}
	var evidence struct {
		SourceURL string `json:"source_url"`
		SHA256    string `json:"html_sha256"`
		Reviewed  bool   `json:"selectors_reviewed"`
		ListSHA   string `json:"list_sha256"`
		Selectors struct {
			Body string `json:"body"`
		} `json:"selectors"`
		Expected struct {
			Title, Body, Department, Category, Status string
			OriginalCategory                          string             `json:"original_category"`
			Attachments                               []model.Attachment `json:"attachments"`
		} `json:"expected"`
	}
	if err = json.Unmarshal(meta, &evidence); err != nil {
		return zero, err
	}
	digest := sha256.Sum256(raw)
	if !evidence.Reviewed || evidence.SHA256 != hex.EncodeToString(digest[:]) {
		return zero, fmt.Errorf("reviewed detail hash mismatch")
	}
	list, err := read("list.actual.html")
	if err != nil {
		return zero, err
	}
	digest = sha256.Sum256(list)
	if evidence.ListSHA != hex.EncodeToString(digest[:]) {
		return zero, fmt.Errorf("list hash mismatch")
	}
	canonical, err := SSUCatchURL(SSUCatchOrigin+ssuNoticePath, evidence.SourceURL)
	if err != nil {
		return zero, err
	}
	rows, _, err := ParseSSUCatchList(list, SSUCatchOrigin+ssuNoticePath)
	if err != nil {
		return zero, err
	}
	for _, row := range rows {
		if row.OriginalURL == canonical {
			row.SourceName = "ssu_catch"
			item, err := ParseSSUCatchDetail(raw, evidence.SourceURL, row, evidence.Selectors.Body)
			if err != nil {
				return zero, err
			}
			want := evidence.Expected
			if item.Title != want.Title || item.Content != want.Body || item.Department != want.Department || item.Category != want.Category || item.OriginalCategory != want.OriginalCategory || item.Status != want.Status || !reflect.DeepEqual(item.Attachments, want.Attachments) {
				return zero, fmt.Errorf("reviewed fixture expectations mismatch")
			}
			return item, nil
		}
	}
	return zero, fmt.Errorf("detail slug absent from list evidence")
}
