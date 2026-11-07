package ssudetail

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"example.com/crawlserver/internal/model"
)

// This is an invented document for testing the evidence harness itself.
// These selectors are never presented as real SSU detail selectors.
func syntheticEvidence() ([]byte, Evidence) {
	raw := []byte(`<h1 id="synthetic-title">테스트 공지</h1><span id="synthetic-department">테스트 부서</span><div id="synthetic-body"><p>테스트 본문</p><div id="synthetic-files"><a href="/download.php?id=1">신청서.hwp</a></div></div>`)
	sum := sha256.Sum256(raw)
	return raw, Evidence{SourceURL: "https://scatch.ssu.ac.kr/공지사항/?slug=synthetic", CapturedAt: "2026-11-06T09:00:00+09:00", SHA256: hex.EncodeToString(sum[:]), Reviewed: true, Selectors: Selectors{Title: "#synthetic-title", Body: "#synthetic-body", Department: "#synthetic-department", Attachments: "#synthetic-files"}, Expected: Expected{Title: "테스트 공지", Body: "테스트 본문\n신청서.hwp", Department: "테스트 부서", OriginalCategory: "장학", Category: "scholarship", PublishedDate: "2026.11.06", Status: "진행", AttachmentAreaCount: 1, Attachments: []model.Attachment{{Name: "신청서.hwp", URL: "https://scatch.ssu.ac.kr/download.php?id=1"}}}}
}
func TestSyntheticEvidenceHarness(t *testing.T) {
	raw, evidence := syntheticEvidence()
	item, err := Validate(raw, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if item.ContentHash == "" || item.SourceName != "ssu_catch" || item.Category != "scholarship" {
		t.Fatal(item)
	}
}
func TestEvidenceRejectsUnreviewedOrMismatchedInput(t *testing.T) {
	cases := map[string]func(*Evidence){
		"unreviewed":             func(e *Evidence) { e.Reviewed = false },
		"changed HTML":           func(e *Evidence) { e.SHA256 = "wrong" },
		"invalid CSS":            func(e *Evidence) { e.Selectors.Body = "[" },
		"missing body":           func(e *Evidence) { e.Selectors.Body = "#missing" },
		"broad body":             func(e *Evidence) { e.Selectors.Body = "body" },
		"wrong title":            func(e *Evidence) { e.Expected.Title = "wrong" },
		"wrong department":       func(e *Evidence) { e.Expected.Department = "wrong" },
		"wrong body":             func(e *Evidence) { e.Expected.Body = "wrong" },
		"wrong category":         func(e *Evidence) { e.Expected.Category = "academic" },
		"wrong attachment name":  func(e *Evidence) { e.Expected.Attachments[0].Name = "wrong.pdf" },
		"wrong attachment scope": func(e *Evidence) { e.Selectors.Attachments = "#synthetic-title" },
		"off origin":             func(e *Evidence) { e.SourceURL = "https://example.org/공지사항/?slug=test" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			raw, e := syntheticEvidence()
			change(&e)
			if _, err := Validate(raw, e); err == nil {
				t.Fatal("invalid evidence accepted")
			}
		})
	}
}
func TestEvidenceDetectsAttachmentOutsideBody(t *testing.T) {
	raw, e := syntheticEvidence()
	raw = []byte(strings.Replace(string(raw), `<div id="synthetic-files">`, `</div><div id="synthetic-files">`, 1))
	sum := sha256.Sum256(raw)
	e.SHA256 = hex.EncodeToString(sum[:])
	e.Expected.Body = "테스트 본문"
	if _, err := Validate(raw, e); err == nil || !strings.Contains(err.Error(), "attachment metadata mismatch") {
		t.Fatalf("expected outside-body parser gap: %v", err)
	}
}
