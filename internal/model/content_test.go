package model

import (
	"strings"
	"testing"
	"time"
)

func TestContentDigestAndURL(t *testing.T) {
	a := Content{Title: "A  title", Content: "one\n two", Category: "academic", Attachments: []Attachment{{Name: "a.pdf", URL: "https://example.org/a.pdf"}}}
	b := a
	b.Title = "A title"
	b.Content = "one two"
	b.CrawledAt = time.Now()
	b.Category = "scholarship"
	if ContentDigest(a) != ContentDigest(b) {
		t.Fatal("whitespace, discovery category or time changed digest")
	}
	b.Content = "modified"
	if ContentDigest(a) == ContentDigest(b) {
		t.Fatal("body mutation ignored")
	}
	b = a
	b.Status = "마감"
	if ContentDigest(a) == ContentDigest(b) {
		t.Fatal("status mutation ignored")
	}
	u, err := NormalizeContentURL("https://EXAMPLE.org:443/a?slug=한글&utm_source=x&fbclid=y#top")
	if err != nil || strings.Contains(u, "utm_") || !strings.Contains(u, "slug=") {
		t.Fatal(u, err)
	}
}

func TestURLPreservesReservedPathAndIPv6(t *testing.T) {
	a, err := NormalizeContentURL("https://example.org/a%2fb?slug=x")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NormalizeContentURL("https://example.org/a/b?slug=x")
	if err != nil || a == b || !strings.Contains(a, "%2F") {
		t.Fatal(a, b, err)
	}
	u, err := NormalizeContentURL("https://[2606:4700:4700::1111]:443/a")
	if err != nil || u != "https://[2606:4700:4700::1111]/a" {
		t.Fatal(u, err)
	}
}
