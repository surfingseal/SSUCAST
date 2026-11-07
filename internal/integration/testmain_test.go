package integration_test

import (
	"example.com/crawlserver/internal/config"
	"fmt"
	"os"
	"testing"
)

// Fail before any existing integration test can open/create a DB when a test
// URI mistakenly points at Atlas or another non-isolated target.
func TestMain(m *testing.M) {
	if uri := os.Getenv("TEST_MONGODB_URI"); uri != "" {
		if e := config.ValidateIsolatedTestMongo(uri, "crawltest_000000000000000000000000"); e != nil {
			fmt.Fprintln(os.Stderr, "refusing unsafe TEST_MONGODB_URI (credentials omitted)")
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}
