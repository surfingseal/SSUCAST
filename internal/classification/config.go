package classification

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Provider, Project, Location, Model, CredentialsFile string
	AutoEnabled                                         bool
	BatchSize, Concurrency, MaxItems, MaxRetries        int
	Timeout                                             time.Duration
}

func LoadConfig() (Config, error) {
	get := func(k, d string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return d
	}
	c := Config{Provider: get("CLASSIFICATION_PROVIDER", "disabled"), Project: os.Getenv("GOOGLE_CLOUD_PROJECT"), Location: get("GOOGLE_CLOUD_LOCATION", "global"), Model: get("VERTEX_MODEL", "gemini-3.5-flash"), CredentialsFile: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")}
	b, err := strconv.ParseBool(get("CLASSIFICATION_AUTO_ENABLED", "false"))
	if err != nil {
		return c, fmt.Errorf("invalid CLASSIFICATION_AUTO_ENABLED")
	}
	c.AutoEnabled = b
	values := []struct {
		key, def string
		dst      *int
	}{{"CLASSIFICATION_BATCH_SIZE", "1", &c.BatchSize}, {"CLASSIFICATION_CONCURRENCY", "1", &c.Concurrency}, {"CLASSIFICATION_MAX_ITEMS", "1", &c.MaxItems}, {"CLASSIFICATION_MAX_RETRIES", "0", &c.MaxRetries}}
	for _, v := range values {
		n, e := strconv.Atoi(get(v.key, v.def))
		if e != nil {
			return c, fmt.Errorf("invalid %s", v.key)
		}
		*v.dst = n
	}
	n, err := strconv.Atoi(get("CLASSIFICATION_TIMEOUT_SECONDS", "60"))
	if err != nil || n < 1 || n > 60 {
		return c, fmt.Errorf("CLASSIFICATION_TIMEOUT_SECONDS must be 1..60")
	}
	c.Timeout = time.Duration(n) * time.Second
	if c.AutoEnabled || c.BatchSize != 1 || c.Concurrency != 1 || c.MaxItems != 1 || c.MaxRetries != 0 {
		return c, fmt.Errorf("manual one-item mode requires auto=false batch=1 concurrency=1 max_items=1 retries=0")
	}
	if c.Provider != "disabled" && c.Provider != "vertexai" {
		return c, fmt.Errorf("unsupported classification provider")
	}
	if c.Provider == "vertexai" {
		v, e := strconv.ParseBool(get("GOOGLE_GENAI_USE_VERTEXAI", "false"))
		if e != nil || !v {
			return c, fmt.Errorf("GOOGLE_GENAI_USE_VERTEXAI must be true")
		}
		if c.Project == "" || c.Location == "" || c.Model == "" || strings.ContainsAny(c.Project+c.Location+c.Model, "/ ?&#\n") {
			return c, fmt.Errorf("valid Vertex project/location/model required")
		}
	}
	return c, nil
}

// ValidateServiceAccount never returns key material. Empty path means ADC discovery.
func ValidateServiceAccount(path, project string) error {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("credential file unavailable")
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("credential file must be regular and owner-only (0600 or 0400)")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("credential file unreadable")
	}
	var v struct {
		Type       string `json:"type"`
		Project    string `json:"project_id"`
		PrivateKey string `json:"private_key"`
		Email      string `json:"client_email"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return fmt.Errorf("credential file is not valid JSON")
	}
	if v.Type != "service_account" || v.PrivateKey == "" || v.Email == "" {
		return fmt.Errorf("valid service account credential required")
	}
	if v.Project != project {
		return fmt.Errorf("credential project does not match GOOGLE_CLOUD_PROJECT")
	}
	return nil
}
