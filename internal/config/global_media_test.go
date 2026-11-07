package config

import (
	"example.com/crawlserver/internal/crawler"
	"testing"
	"time"
)

func TestUniversitySourceConfig(t *testing.T) {
	sources, e := LoadSources("../../configs/sources.university.json")
	if e != nil {
		t.Fatal(e)
	}
	if len(sources) != 2 {
		t.Fatal("source missing")
	}
	registry, e := crawler.DefaultRegistry(crawler.NewFetcher(time.Second))
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range sources {
		if s.Enabled || s.Automatic() || s.PolicyApproved {
			t.Fatal("collection enabled", s.Name)
		}
		if _, e := registry.Resolve(s); e != nil {
			t.Fatal(e)
		}
		if s.Name == "ssu_catch" && s.ClassificationMode() != "source" {
			t.Fatal("SSU policy changed")
		}
		if s.Name == "ssu_global_media" && (s.ClassificationMode() != "hybrid" || s.DisplayName != "숭실대학교 글로벌미디어학부" || s.MaxContents != 1) {
			t.Fatal(s)
		}
	}
}
