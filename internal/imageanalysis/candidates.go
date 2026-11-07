package imageanalysis

import (
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/noticepolicy"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const Model = "gemini-3.8-flash"
const Version = "schedule-image-v1"

type ImageCrop struct {
	X      int `json:"x" bson:"x"`
	Y      int `json:"y" bson:"y"`
	Width  int `json:"width" bson:"width"`
	Height int `json:"height" bson:"height"`
}
type ImageInput struct {
	OriginalSHA256 string     `json:"original_sha256,omitempty" bson:"original_sha256,omitempty"`
	OriginalFile   string     `json:"original_file,omitempty" bson:"-"`
	Crop           *ImageCrop `json:"crop,omitempty" bson:"crop,omitempty"`
	URL            string     `json:"image_url" bson:"image_url"`
	Index          int        `json:"index" bson:"index"`
	Alt            string     `json:"alt,omitempty" bson:"alt,omitempty"`
	Location       string     `json:"location" bson:"location"`
	Width          int        `json:"width,omitempty" bson:"width,omitempty"`
	Height         int        `json:"height,omitempty" bson:"height,omitempty"`
	SHA256         string     `json:"sha256,omitempty" bson:"sha256,omitempty"`
	MIME           string     `json:"mime,omitempty" bson:"mime,omitempty"`
	File           string     `json:"file,omitempty" bson:"-"`
}
type Candidate struct {
	Original model.Content `json:"original"`
	Images   []ImageInput  `json:"images"`
	Reasons  []string      `json:"reasons"`
	Priority int           `json:"priority"`
}

var decoration = regexp.MustCompile(`(?i)(?:^|[/_.-])(logo|icon|favicon|avatar)(?:[/_.-]|$)`)

func AllowedImage(v model.Content, raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || decoration.MatchString(u.Path) {
		return false
	}
	switch v.SourceName {
	case "ssu_catch":
		return u.Host == "scatch.ssu.ac.kr" && strings.HasPrefix(u.Path, "/wp-content/uploads/")
	case "ssu_global_media":
		return (u.Host == "media.ssu.ac.kr" && strings.HasPrefix(u.Path, "/")) || u.Host == "globalmedia-bucket.s3.ap-northeast-2.amazonaws.com"
	}
	return false
}
func has(text string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}
func Reasons(v model.Content) []string {
	if v.ImageAnalysisUsed && v.ImageAnalysisContentHash == v.ContentHash {
		return nil
	}
	ss := noticepolicy.Effective(v)
	event, application := false, false
	review := false
	for _, s := range ss {
		adequate := !s.ReviewRequired && s.Quality != nil && (s.Quality.State == "verified" || s.Quality.State == "partial") && (s.StartDate != nil || s.EndDate != nil)
		if s.Type == "event" && adequate {
			event = true
		}
		if (s.Type == "application" || s.Type == "submission") && adequate && s.EndDate != nil {
			application = true
		}
		if s.ReviewRequired || s.Quality == nil || s.Quality.State != "verified" {
			review = true
		}
	}
	// Verified text for both key purposes takes priority over generic image hints.
	if event && application {
		return nil
	}
	r := []string{}
	text := v.Title + "\n" + v.Content
	if len(ss) == 0 {
		r = append(r, "no_text_schedules")
	}
	if has(text, "행사", "특강", "설명회", "교육", "세미나", "연수", "봉사") && !event {
		r = append(r, "missing_event")
	}
	if has(text, "신청", "접수", "모집", "제출") && !application {
		r = append(r, "missing_application")
	}
	if has(text, "포스터 참고", "웹포스터", "이미지 참조", "아래 이미지", "안내문 참고", "포스터 참조") {
		r = append(r, "poster_reference")
	}
	if utf8.RuneCountInString(v.Content) < 300 {
		r = append(r, "short_image_body")
	}
	if review {
		r = append(r, "incomplete_text_schedule")
	}
	return r
}

// Read only matching, previously captured Lexical detail JSON; never scrape a
// fresh school detail page or treat attachment links as inline images.
func MediaImages(v model.Content, dir string) []ImageInput {
	if v.SourceName != "ssu_global_media" || v.ExternalPostID == "" || dir == "" {
		return nil
	}
	raw, e := os.ReadFile(filepath.Join(dir, "detail-"+v.ExternalPostID+".json"))
	if e != nil {
		return nil
	}
	var doc struct {
		Data struct {
			ID      int    `json:"id"`
			Title   string `json:"title"`
			Content string `json:"content"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &doc) != nil || fmt.Sprint(doc.Data.ID) != v.ExternalPostID || strings.TrimSpace(doc.Data.Title) != v.Title {
		return nil
	}
	var editor any
	raw = []byte(doc.Data.Content)
	if len(raw) > 0 && raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			raw = []byte(s)
		}
	}
	if json.Unmarshal(raw, &editor) != nil {
		return nil
	}
	out := []ImageInput{}
	var walk func(any, string)
	walk = func(node any, where string) {
		switch x := node.(type) {
		case []any:
			for i, n := range x {
				walk(n, fmt.Sprintf("%s[%d]", where, i))
			}
		case map[string]any:
			if typ, _ := x["type"].(string); typ == "image" {
				src, _ := x["src"].(string)
				alt, _ := x["altText"].(string)
				if AllowedImage(v, src) {
					im := ImageInput{URL: src, Index: len(out), Alt: alt, Location: where}
					if w, ok := x["width"].(float64); ok {
						im.Width = int(w)
					}
					if h, ok := x["height"].(float64); ok {
						im.Height = int(h)
					}
					out = append(out, im)
				}
			}
			for _, k := range []string{"editorState", "root", "children"} {
				if y, ok := x[k]; ok {
					walk(y, where+"."+k)
				}
			}
		}
	}
	walk(editor, "stored_lexical_body")
	return out
}
func Candidates(items []model.Content, mediaDir string) ([]Candidate, map[string]int) {
	out := []Candidate{}
	stats := map[string]int{"contents": len(items)}
	for _, v := range items {
		images := []ImageInput{}
		seen := map[string]bool{}
		for i, u := range v.ImageURLs {
			if AllowedImage(v, u) && !seen[u] {
				seen[u] = true
				images = append(images, ImageInput{URL: u, Index: i, Location: "stored_inline_image_urls"})
			} else {
				stats["unsafe_or_duplicate_images"]++
			}
		}
		for _, im := range MediaImages(v, mediaDir) {
			if !seen[im.URL] {
				seen[im.URL] = true
				images = append(images, im)
			}
		}
		if len(images) == 0 {
			continue
		}
		stats["with_public_inline_images"]++
		stats["inline_images"] += len(images)
		if v.ScheduleAnalysisID == "" {
			stats["without_existing_text_analysis"]++
			continue
		}
		r := Reasons(v)
		if len(r) == 0 {
			stats["sufficient_or_already_image_analyzed"]++
			continue
		}
		priority := len(r)
		if len(noticepolicy.Effective(v)) == 0 {
			priority += 5
		}
		if len(images) > 3 {
			images = images[:3]
		}
		out = append(out, Candidate{Original: v, Images: images, Reasons: r, Priority: priority})
		stats["candidates_"+v.SourceName]++
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].Original.ID.Hex() < out[j].Original.ID.Hex()
	})
	stats["candidates"] = len(out)
	return out, stats
}
