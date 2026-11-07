package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validSource = `{"name":"careers","crawler_type":"example","base_url":"https://careers.invalid","list_url":"/posts"}`

func TestMultipleSourcesDefaultsAndValidation(t *testing.T) {
	sources, err := ParseSources([]byte("[" + validSource + `,{"id":"507f1f77bcf86cd799439011","name":"activities","crawler_type":"example","base_url":"https://activities.invalid","enabled":false,"auto_enabled":true,"request_interval":"3s","max_pages":2,"max_contents":5,"max_requests":15,"max_concurrency":2,"schedule":"0 0 * * *"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || !sources[0].Enabled || sources[0].Automatic() || sources[0].MaxPages != 1 || sources[0].MaxContents != 3 || sources[0].MaxRequests != 20 || sources[1].ID.Hex() != "507f1f77bcf86cd799439011" || sources[1].RequestIntervalMS != 3000 || sources[1].Automatic() {
		t.Fatal(sources)
	}
	cases := []string{"null", `{}`, "[" + validSource + "," + validSource + "]", "[" + validSource + "] {}"}
	for _, replacement := range []struct{ old, new string }{{`"/posts"`, `"https://other.invalid/posts"`}, {`"example"`, `""`}, {`"careers"`, `"bad name"`}, {`https://careers.invalid`, `file:///etc/passwd`}, {`https://careers.invalid`, `http://127.0.0.1`}} {
		cases = append(cases, "["+strings.Replace(validSource, replacement.old, replacement.new, 1)+"]")
	}
	for _, field := range []string{`"max_pages":-1`, `"max_requests":100001`, `"max_concurrency":17`, `"request_interval":"0s"`, `"request_interval":"1.5ms"`, `"schedule":"invalid"`, `"id":"invalid"`, `"unknown":1`} {
		cases = append(cases, "["+strings.TrimSuffix(validSource, "}")+","+field+"}]")
	}
	for _, raw := range cases {
		if _, err := ParseSources([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
func TestLoadSourcesAndLegacyEnvironment(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "a-long-test-secret-0123456789")
	t.Setenv("SOURCE_URL", "https://legacy.invalid/list")
	t.Setenv("DEMO_MODE", "false")
	dir := t.TempDir()
	path := filepath.Join(dir, "sources.json")
	if err := os.WriteFile(path, []byte("["+validSource+"]"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CRAWL_SOURCES_FILE", path)
	cfg, err := Load()
	if err != nil || len(cfg.Sources) != 1 || cfg.SourceURL != "https://legacy.invalid/list" {
		t.Fatal(cfg, err)
	}
	if err := os.WriteFile(path, []byte("["+strings.Replace(validSource, "careers", "example", 1)+"]"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("legacy source collision accepted")
	}
	t.Setenv("CRAWL_SOURCES_FILE", filepath.Join(dir, "missing"))
	if _, err := Load(); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestSSUSafetyConfig(t *testing.T) {
	raw, err := os.ReadFile("../../configs/sources.ssu-catch.json")
	if err != nil {
		t.Fatal(err)
	}
	sources, err := ParseSources(raw)
	if err != nil || len(sources) != 1 {
		t.Fatal(sources, err)
	}
	s := sources[0]
	if s.Enabled || s.Automatic() || s.PolicyApproved || s.DetailBodySelector != "" || s.MaxRequests != 5 || s.RequestIntervalMS != 2000 {
		t.Fatal(s)
	}
	for _, pair := range [][2]string{{`"auto_enabled": false`, `"auto_enabled": true`}, {`"max_requests": 5`, `"max_requests": 6`}, {`"max_pages": 1`, `"max_pages": 2`}, {`"request_interval": "2s"`, `"request_interval": "1s"`}, {`"max_concurrency": 1`, `"max_concurrency": 2`}} {
		if _, err := ParseSources([]byte(strings.Replace(string(raw), pair[0], pair[1], 1))); err == nil {
			t.Fatal(pair)
		}
	}
}

func TestClassificationStrategyConfiguration(t *testing.T) {
	for _, strategy := range []string{"source", "ai", "hybrid"} {
		raw := "[" + strings.TrimSuffix(validSource, "}") + `,"classification_strategy":"` + strategy + `"}]`
		sources, err := ParseSources([]byte(raw))
		if err != nil || sources[0].ClassificationMode() != strategy {
			t.Fatal(sources, err)
		}
	}
	raw := "[" + strings.TrimSuffix(validSource, "}") + `,"classification_strategy":"guess"}]`
	if _, err := ParseSources([]byte(raw)); err == nil {
		t.Fatal("invalid classification strategy accepted")
	}
	sources, err := ParseSources([]byte("[" + validSource + "]"))
	if err != nil || sources[0].ClassificationMode() != "source" {
		t.Fatal(sources, err)
	}
}
