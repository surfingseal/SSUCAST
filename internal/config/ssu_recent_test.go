package config

import (
	"os"
	"strings"
	"testing"
)

func TestRecentSourceProfileAndSafety(t *testing.T) {
	raw, e := os.ReadFile("../../configs/sources.ssu-recent.json")
	if e != nil {
		t.Fatal(e)
	}
	sources, e := ParseSources(raw)
	if e != nil || len(sources) != 1 {
		t.Fatal(sources, e)
	}
	s := sources[0]
	if s.Enabled || s.Automatic() || s.ClassificationMode() != "source" || s.CollectionMode != "recent_three_months" || s.MaxPages != 30 || s.RequestJitterMS != 200 || s.MaxRetries == nil || *s.MaxRetries != 1 || s.PolicyApprovalBasis == "" {
		t.Fatal(s)
	}
	for _, pair := range [][2]string{{`"max_concurrency": 1`, `"max_concurrency": 2`}, {`"request_interval": "2s"`, `"request_interval": "1s"`}, {`"auto_enabled": false`, `"auto_enabled": true`}, {`"classification_strategy": "source"`, `"classification_strategy": "ai"`}, {`"max_retries": 1`, `"max_retries": 5`}} {
		if _, e := ParseSources([]byte(strings.Replace(string(raw), pair[0], pair[1], 1))); e == nil {
			t.Fatal("unsafe recent config accepted", pair)
		}
	}
}
