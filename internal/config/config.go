package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/robfig/cron/v3"

	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
)

type Config struct {
	SourcesFile                                     string
	Sources                                         []model.Source
	Addr, URI, Database, Token, SourceURL, Schedule string
	Workers                                         int
	RequestTimeout, JobTimeout                      time.Duration
}

func Load() (Config, error) {
	if err := LoadDotEnv(".env"); err != nil {
		return Config{}, err
	}
	c := Config{Addr: env("HTTP_ADDR", ":8080"), URI: env("MONGODB_URI", "mongodb://localhost:27017"), Database: env("MONGODB_DATABASE", "crawlserver"), Token: os.Getenv("ADMIN_TOKEN"), SourceURL: os.Getenv("SOURCE_URL"), Schedule: env("SOURCE_SCHEDULE", "0 */6 * * *")}
	var err error
	c.Workers, err = strconv.Atoi(env("WORKERS", "2"))
	if err != nil || c.Workers < 1 || c.Workers > 32 {
		return c, fmt.Errorf("WORKERS must be 1..32")
	}
	c.RequestTimeout, err = time.ParseDuration(env("REQUEST_TIMEOUT", "15s"))
	if err != nil || c.RequestTimeout <= 0 {
		return c, fmt.Errorf("invalid REQUEST_TIMEOUT")
	}
	c.JobTimeout, err = time.ParseDuration(env("JOB_TIMEOUT", "10m"))
	if err != nil || c.JobTimeout <= 0 {
		return c, fmt.Errorf("invalid JOB_TIMEOUT")
	}
	if len(c.Token) < 24 {
		return c, fmt.Errorf("ADMIN_TOKEN must contain at least 24 bytes")
	}

	if _, err := cron.ParseStandard(c.Schedule); err != nil {
		return c, fmt.Errorf("invalid SOURCE_SCHEDULE")
	}
	c.SourcesFile = os.Getenv("CRAWL_SOURCES_FILE")
	if c.SourcesFile != "" {
		c.Sources, err = LoadSources(c.SourcesFile)
		if err != nil {
			return c, err
		}
	}
	if c.SourceURL != "" {
		if err := crawlerSource(c.SourceURL); err != nil {
			return c, err
		}
	}
	for _, s := range c.Sources {
		if s.Name == "example" && (c.SourceURL != "" || os.Getenv("DEMO_MODE") == "true") {
			return c, fmt.Errorf("source example conflicts with legacy SOURCE_URL or DEMO_MODE")
		}
	}
	return c, nil
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func crawlerSource(raw string) error { return crawler.ValidateSource(model.Source{BaseURL: raw}) }
