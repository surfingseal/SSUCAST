package model

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// NormalizeContentURL drops only tracking fields. Identity parameters, including
// slug, remain. SSU's stricter canonicalization is performed by its parser.
func NormalizeContentURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return "", fmt.Errorf("invalid content URL")
	}
	if u.Scheme == "https" && u.Port() == "443" || u.Scheme == "http" && u.Port() == "80" {
		u.Host = u.Hostname()
		if strings.Contains(u.Host, ":") {
			u.Host = "[" + u.Host + "]"
		}
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", err
	}
	for k := range q {
		l := strings.ToLower(k)
		if strings.HasPrefix(l, "utm_") || l == "fbclid" || l == "gclid" {
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode()
	u.Fragment = ""
	// Preserve encoded reserved characters: /a%2Fb and /a/b are different
	// resources. Canonicalize escape case without decoding those boundaries.
	if u.RawPath != "" {
		escaped := []byte(u.RawPath)
		for i := 0; i+2 < len(escaped); i++ {
			if escaped[i] == '%' {
				for j := i + 1; j <= i+2; j++ {
					if escaped[j] >= 'a' && escaped[j] <= 'f' {
						escaped[j] -= 'a' - 'A'
					}
				}
				i += 2
			}
		}
		u.RawPath = string(escaped)
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}
func NormalizeText(s string) string { return strings.Join(strings.Fields(s), " ") }

// Category memberships are discovery metadata, merged independently of content
// revisions. Timestamps and database IDs must never affect the content digest.
func ContentDigest(v Content) string {
	attachments := append([]Attachment{}, v.Attachments...)
	for i := range attachments {
		attachments[i].Name = NormalizeText(attachments[i].Name)
	}
	sort.Slice(attachments, func(i, j int) bool {
		if attachments[i].URL == attachments[j].URL {
			return attachments[i].Name < attachments[j].Name
		}
		return attachments[i].URL < attachments[j].URL
	})
	published := ""
	if v.PublishedAt != nil {
		published = v.PublishedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	images := append([]string{}, v.ImageURLs...)
	sort.Strings(images)
	raw, _ := json.Marshal(struct {
		Title, Body, Department, Status, Published string
		Attachments                                []Attachment
		Author                                     string   `json:",omitempty"`
		ImageURLs                                  []string `json:",omitempty"`
	}{NormalizeText(v.Title), NormalizeText(v.Content), NormalizeText(v.Department), NormalizeText(v.Status), published, attachments, NormalizeText(v.Author), images})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
