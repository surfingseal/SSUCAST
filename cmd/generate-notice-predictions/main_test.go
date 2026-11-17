package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpointMissingAndCorruptDistinction(t *testing.T) {
	p := filepath.Join(t.TempDir(), "manifest.json")
	var v Manifest
	if err := readCheckpoint(p, &v); !os.IsNotExist(err) {
		t.Fatal("missing checkpoint must permit first run", err)
	}
	if err := os.WriteFile(p, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := readCheckpoint(p, &v); err == nil || os.IsNotExist(err) {
		t.Fatal("corrupt checkpoint must block resume", err)
	}
}
