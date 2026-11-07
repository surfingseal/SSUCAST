package crawler

import (
	"example.com/crawlserver/internal/model"
	"fmt"
)

// Add site factory registrations here; Runner, HTTP handlers and main do not change.
func DefaultRegistry(f *Fetcher) (*Registry, error) {
	if f == nil {
		return nil, fmt.Errorf("fetcher is required")
	}
	r := NewRegistry()
	if err := r.Register("example", func(model.Source) (Crawler, error) { return &Example{Timeout: f.Timeout, fetcher: f}, nil }); err != nil {
		return nil, err
	}
	if err := r.Register("ssu_catch", func(model.Source) (Crawler, error) { return &SSUCatchCrawler{Fetcher: f}, nil }); err != nil {
		return nil, err
	}
	if err := r.Register("ssu_global_media", func(model.Source) (Crawler, error) { return &GlobalMediaCrawler{Fetcher: f}, nil }); err != nil {
		return nil, err
	}
	return r, nil
}
