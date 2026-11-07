package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "a-long-test-secret-0123456789")
	t.Setenv("WORKERS", "2")
	c, err := Load()
	if err != nil || c.Workers != 2 {
		t.Fatal(c, err)
	}
	t.Setenv("WORKERS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("invalid workers accepted")
	}
	t.Setenv("WORKERS", "2")
	t.Setenv("ADMIN_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing token accepted")
	}
}
