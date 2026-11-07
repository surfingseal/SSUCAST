package main

import (
	"encoding/json"
	"example.com/crawlserver/internal/testutil/localfixture"
	"os"
	"path/filepath"
	"testing"
)

func TestActualCapturedPreviewValidationBeforeDatabase(t *testing.T) {
	dir := "../../internal/crawler/testdata/ssu_catch/live_20261106"
	localfixture.Require(t, dir)
	r, e := verifyCaptured(dir)
	if e != nil || len(r.Items) != 3 {
		t.Fatal("actual preview failed integrity/header validation", e)
	}
	// No DB/server is opened: altered evidence must fail before persistence.
	tmp := t.TempDir()
	entries, e := os.ReadDir(dir)
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			b, e := os.ReadFile(filepath.Join(dir, entry.Name()))
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(tmp, entry.Name()), b, 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e = os.WriteFile(filepath.Join(tmp, "03-detail.actual.html"), []byte("tampered"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = verifyCaptured(tmp); e == nil {
		t.Fatal("modified HTML accepted for saving")
	}
	raw, e := os.ReadFile(filepath.Join(dir, "run.json"))
	if e != nil {
		t.Fatal(e)
	}
	var v map[string]any
	if e = json.Unmarshal(raw, &v); e != nil {
		t.Fatal(e)
	}
	v["items"] = []any{}
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(tmp, "run.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = verifyCaptured(tmp); e == nil {
		t.Fatal("empty live result accepted")
	}
}
