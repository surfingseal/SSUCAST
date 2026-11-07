package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"github.com/robfig/cron/v3"
	"go.mongodb.org/mongo-driver/v2/bson"

	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
)

type SourceConfig struct {
	CollectionMode      string `json:"collection_mode"`
	PolicyApprovalBasis string `json:"policy_approval_basis"`
	RequestJitter       string `json:"request_jitter"`
	MaxRetries          *int   `json:"max_retries"`

	DisplayName            string `json:"display_name"`
	ClassificationStrategy string `json:"classification_strategy"`
	PolicyApproved         bool   `json:"policy_approved"`
	DetailBodySelector     string `json:"detail_body_selector"`
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	CrawlerType            string `json:"crawler_type"`
	BaseURL                string `json:"base_url"`
	ListURL                string `json:"list_url"`
	Enabled                *bool  `json:"enabled"`
	RequestInterval        string `json:"request_interval"`
	MaxPages               int    `json:"max_pages"`
	MaxContents            int    `json:"max_contents"`
	MaxRequests            int    `json:"max_requests"`
	MaxConcurrency         int    `json:"max_concurrency"`
	AutoEnabled            bool   `json:"auto_enabled"`
	Schedule               string `json:"schedule"`
}

var sourceKey = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func LoadSources(path string) ([]model.Source, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("sources file: %w", err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<20 {
		return nil, fmt.Errorf("sources file exceeds 1 MiB")
	}
	return ParseSources(raw)
}
func ParseSources(raw []byte) ([]model.Source, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var entries []SourceConfig
	if err := d.Decode(&entries); err != nil {
		return nil, fmt.Errorf("sources JSON: %w", err)
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("sources file must contain one JSON array")
	}
	if entries == nil || len(entries) > 100 {
		return nil, fmt.Errorf("sources must be an array of at most 100 entries")
	}
	result := make([]model.Source, 0, len(entries))
	names, ids := map[string]bool{}, map[string]bool{}
	for _, v := range entries {
		if !sourceKey.MatchString(v.Name) || !sourceKey.MatchString(v.CrawlerType) {
			return nil, fmt.Errorf("source name and crawler_type must be 1..64 alphanumeric, underscore or hyphen characters")
		}
		if names[v.Name] {
			return nil, fmt.Errorf("duplicate source name %q", v.Name)
		}
		names[v.Name] = true
		s := model.Source{DisplayName: v.DisplayName, Name: v.Name, CrawlerType: v.CrawlerType, BaseURL: v.BaseURL, ListURL: v.ListURL, Enabled: true, AutoEnabled: &v.AutoEnabled, Schedule: v.Schedule, MaxPages: v.MaxPages, MaxContents: v.MaxContents, MaxRequests: v.MaxRequests, MaxConcurrency: v.MaxConcurrency}
		s.CollectionMode = v.CollectionMode
		s.PolicyApprovalBasis = v.PolicyApprovalBasis
		s.MaxRetries = v.MaxRetries
		if v.CollectionMode != "" && (v.CollectionMode != "recent_three_months" || s.CrawlerKey() != "ssu_catch") {
			return nil, fmt.Errorf("unsupported collection_mode")
		}
		if v.RequestJitter != "" {
			j, e := time.ParseDuration(v.RequestJitter)
			if e != nil || j < 0 || j > time.Minute || j%time.Millisecond != 0 {
				return nil, fmt.Errorf("invalid request_jitter")
			}
			s.RequestJitterMS = j.Milliseconds()
		}
		if v.MaxRetries != nil && (*v.MaxRetries < 0 || *v.MaxRetries > 2) {
			return nil, fmt.Errorf("max_retries must be 0..2")
		}
		s.ClassificationStrategy = v.ClassificationStrategy
		if s.ClassificationStrategy == "" {
			s.ClassificationStrategy = "source"
		}
		if err := s.ValidateClassificationStrategy(); err != nil {
			return nil, fmt.Errorf("source %s: %w", v.Name, err)
		}
		s.PolicyApproved = v.PolicyApproved
		s.DetailBodySelector = v.DetailBodySelector
		if v.Enabled != nil {
			s.Enabled = *v.Enabled
		}
		if v.ID != "" {
			id, err := bson.ObjectIDFromHex(v.ID)
			if err != nil || id.IsZero() {
				return nil, fmt.Errorf("source %s: invalid id", v.Name)
			}
			if ids[id.Hex()] {
				return nil, fmt.Errorf("duplicate source id")
			}
			ids[id.Hex()] = true
			s.ID = id
		}
		if v.RequestInterval != "" {
			interval, err := time.ParseDuration(v.RequestInterval)
			if err != nil || interval < time.Millisecond || interval > 24*time.Hour || interval%time.Millisecond != 0 {
				return nil, fmt.Errorf("source %s: request_interval must be 1ms..24h in whole milliseconds", v.Name)
			}
			s.RequestIntervalMS = interval.Milliseconds()
		}
		if s.CrawlerKey() == "ssu_catch" && s.CollectionMode == "" {
			if v.AutoEnabled || s.MaxPages > 1 || s.MaxContents > 3 || s.MaxRequests > 5 || s.MaxConcurrency > 1 || (s.RequestIntervalMS > 0 && s.RequestIntervalMS < 2000) {
				return nil, fmt.Errorf("source %s: SSU MVP requires manual-only, pages<=1, contents<=3, requests<=5, concurrency=1, interval>=2s", v.Name)
			}
			if s.MaxRequests == 0 {
				s.MaxRequests = 5
			}
			if s.RequestIntervalMS == 0 {
				s.RequestIntervalMS = 2000
			}
		}
		if s.CollectionMode == "recent_three_months" {
			if v.AutoEnabled || s.ClassificationMode() != "source" || s.MaxConcurrency > 1 || s.RequestIntervalMS > 0 && s.RequestIntervalMS < 2000 {
				return nil, fmt.Errorf("SSU recent mode requires manual source strategy, concurrency=1 and interval>=2s")
			}
			if s.MaxPages == 0 {
				s.MaxPages = 30
			}
			if s.MaxContents == 0 {
				s.MaxContents = 1000
			}
			if s.MaxRequests == 0 {
				s.MaxRequests = 1500
			}
			if s.RequestIntervalMS == 0 {
				s.RequestIntervalMS = 2000
			}
		}
		// New sources use small limits. Existing BSON records keep historical defaults.
		if s.MaxPages == 0 {
			s.MaxPages = 1
		}
		if s.MaxContents == 0 {
			s.MaxContents = 3
		}
		if s.MaxRequests == 0 {
			s.MaxRequests = 20
		}
		if s.Schedule == "" {
			s.Schedule = "0 */6 * * *"
		}
		if _, err := cron.ParseStandard(s.Schedule); err != nil {
			return nil, fmt.Errorf("source %s: invalid cron schedule", v.Name)
		}
		s = s.WithDefaults()
		if err := crawler.ValidateSource(s); err != nil {
			return nil, fmt.Errorf("source %s: %w", v.Name, err)
		}
		result = append(result, s)
	}
	return result, nil
}
