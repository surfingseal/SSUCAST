package crawler

import (
	"example.com/crawlserver/internal/testutil/localfixture"
	"os"
	"strings"
	"testing"
)

func TestGlobalMediaCurrentActualThree(t *testing.T) {
	localfixture.Require(t, "testdata/global_media/current/list-api-0.json")
	data, e := os.ReadFile("testdata/global_media/current/list-api-0.json")
	if e != nil {
		t.Fatal(e)
	}
	rows, pages, e := ParseGlobalMediaList(data)
	if e != nil || len(rows) != 15 || pages != 15 {
		t.Fatal(len(rows), pages, e)
	}
	cases := map[string]struct{ date, quote string }{"1814": {"2026-11-02", "싱가포르 해외 연수"}, "1812": {"2026-10-07", "2026년 10월 6일(화) 09:00"}, "1810": {"2026-09-29", "학부 전공과 관계없이 누구나 지원"}}
	review := GlobalMediaReview{EvidenceKind: "actual", DetailFields: map[string]string{"id": "/data/id", "title": "/data/title", "content": "/data/content", "author": "/data/userName", "published_at": "/data/createdAt"}, BodyFormat: "editor_text_nodes", AttachmentsPointer: "/data/attachments", AttachmentIDsOnly: true}
	checked := 0
	for _, r := range rows {
		c, ok := cases[r.ID]
		if !ok {
			continue
		}
		localfixture.Require(t, "testdata/global_media/current/detail-"+r.ID+".json")
		raw, e := os.ReadFile("testdata/global_media/current/detail-" + r.ID + ".json")
		if e != nil {
			t.Fatal(e)
		}
		v, e := ParseGlobalMediaDetail(raw, "https://media.ssu.ac.kr/board/notices/"+r.ID, r, review)
		if e != nil || v.Title != strings.TrimSpace(r.Title) || v.Author != "글미관리자" || v.PublishedAt == nil || !strings.HasPrefix(r.CreatedAt, c.date) || !strings.Contains(v.Content, c.quote) || len(v.Content) < 100 {
			t.Fatalf("id=%s length=%d err=%v body=%s", r.ID, len(v.Content), e, v.Content)
		}
		checked++
	}
	if checked != 3 {
		t.Fatal(checked)
	}
}
