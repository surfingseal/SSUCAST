package repository

import (
	"example.com/crawlserver/internal/notices"
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
)

func TestNoticeTitleSearchAndLegacyScope(t *testing.T) {
	f := notices.Filter{Query: "장학.*", SearchIn: "title"}
	match := noticeSearchMatch(f)
	r, ok := match["title"].(bson.Regex)
	if !ok || r.Pattern != `장학\.\*` || r.Options != "i" || len(match) != 1 {
		t.Fatalf("title search must be literal and restricted: %#v", match)
	}
	f.SearchIn = ""
	legacy := noticeSearchMatch(f)["$or"].(bson.A)
	if len(legacy) != 3 {
		t.Fatalf("legacy scope changed: %#v", legacy)
	}
}
