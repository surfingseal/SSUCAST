package model

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestLegacySourceCompatibility(t *testing.T) {
	raw, err := bson.Marshal(bson.M{"_id": bson.NewObjectID(), "name": "example", "base_url": "https://site.invalid/list", "enabled": true, "schedule": "* * * * *"})
	if err != nil {
		t.Fatal(err)
	}
	var s Source
	if err := bson.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	if !s.Automatic() || s.CrawlerKey() != "example" {
		t.Fatal(s)
	}
	d := s.WithDefaults()
	if d.ListURL != s.BaseURL || d.MaxPages != 100 || d.MaxContents != 1000 || d.RequestIntervalMS != 1000 {
		t.Fatal(d)
	}
	s.CrawlerType = "example"
	if s.Automatic() {
		t.Fatal("new typed source automatically enabled without explicit opt-in")
	}
	yes := true
	s.AutoEnabled = &yes
	if !s.Automatic() {
		t.Fatal("explicit opt-in ignored")
	}
	no := false
	s.AutoEnabled = &no
	raw, err = bson.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var restored Source
	if err := bson.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.AutoEnabled == nil || restored.Automatic() {
		t.Fatal("explicit false lost in BSON", restored)
	}
}
