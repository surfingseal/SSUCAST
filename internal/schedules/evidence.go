// Package schedules converts and validates saved schedule data without I/O,
// model clients, credentials, HTTP, databases or lifecycle side effects.
package schedules

import (
	"example.com/crawlserver/internal/model"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizeWhitespace collapses Unicode whitespace, never punctuation or digits.
// It preserves word boundaries: "AB" and "A B" are different.
func NormalizeWhitespace(s string) string { return strings.Join(strings.Fields(s), " ") }

type mappedText struct {
	text         string
	starts, ends []int
}

func whitespaceMap(s string) mappedText {
	var b strings.Builder
	out := mappedText{}
	pendingStart, pendingEnd := -1, -1
	for pos, r := range s {
		end := pos + utf8.RuneLen(r)
		if unicode.IsSpace(r) {
			if b.Len() > 0 {
				if pendingStart < 0 {
					pendingStart = pos
				}
				pendingEnd = end
			}
			continue
		}
		if pendingStart >= 0 {
			b.WriteByte(' ')
			out.starts = append(out.starts, pendingStart)
			out.ends = append(out.ends, pendingEnd)
			pendingStart = -1
		}
		b.WriteRune(r)
		for j := 0; j < utf8.RuneLen(r); j++ {
			out.starts = append(out.starts, pos)
			out.ends = append(out.ends, end)
		}
	}
	out.text = b.String()
	return out
}
func MatchEvidence(title, body, evidence string) model.EvidenceLocation {
	failed := model.EvidenceLocation{MatchType: "unmatched", ByteStart: -1, ByteEnd: -1, RuneStart: -1, RuneEnd: -1}
	if NormalizeWhitespace(evidence) == "" {
		return failed
	}
	for _, item := range []struct{ field, text string }{{"title", title}, {"content", body}} {
		if at := groundedIndex(item.text, evidence); at >= 0 {
			return location(item.field, item.text, at, at+len(evidence), "exact")
		}
		m := whitespaceMap(item.text)
		needle := NormalizeWhitespace(evidence)
		if at := groundedIndex(m.text, needle); at >= 0 {
			start := m.starts[at]
			end := m.ends[at+len(needle)-1]
			return location(item.field, item.text, start, end, "whitespace_normalized")
		}
	}
	return failed
}
func location(field, source string, start, end int, kind string) model.EvidenceLocation {
	return model.EvidenceLocation{Matched: true, MatchType: kind, Field: field, ByteStart: start, ByteEnd: end, RuneStart: utf8.RuneCountInString(source[:start]), RuneEnd: utf8.RuneCountInString(source[:end]), OriginalText: source[start:end]}
}

// A quoted numeric suffix is not evidence for a different amount/date: 400
// cannot be matched inside 1400, nor 1/12 inside 11/12.
func groundedIndex(source, needle string) int {
	offset := 0
	for offset <= len(source) {
		i := strings.Index(source[offset:], needle)
		if i < 0 {
			return -1
		}
		start := offset + i
		end := start + len(needle)
		first, _ := utf8.DecodeRuneInString(needle)
		last, _ := utf8.DecodeLastRuneInString(needle)
		good := true
		if start > 0 && unicode.IsDigit(first) {
			prev, _ := utf8.DecodeLastRuneInString(source[:start])
			if unicode.IsDigit(prev) {
				good = false
			}
		}
		if end < len(source) && unicode.IsDigit(last) {
			next, _ := utf8.DecodeRuneInString(source[end:])
			if unicode.IsDigit(next) {
				good = false
			}
		}
		if good {
			return start
		}
		offset = start + 1
	}
	return -1
}
