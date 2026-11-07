package crawler

import (
	"fmt"
	"sync"

	"example.com/crawlserver/internal/model"
)

type Factory func(model.Source) (Crawler, error)

// Registry may be shared safely. Factories must be safe to invoke concurrently.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

func NewRegistry() *Registry { return &Registry{factories: make(map[string]Factory)} }
func (r *Registry) Register(key string, f Factory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if key == "" || f == nil {
		return fmt.Errorf("crawler key and factory are required")
	}
	if _, exists := r.factories[key]; exists {
		return fmt.Errorf("crawler type %q already registered", key)
	}
	r.factories[key] = f
	return nil
}
func (r *Registry) Has(key string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factories[key]
	return ok
}
func (r *Registry) Resolve(s model.Source) (Crawler, error) {
	key := s.CrawlerKey()
	r.mu.RLock()
	f, ok := r.factories[key]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown crawler type %q", key)
	}
	c, err := f(s)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("crawler factory %q returned nil", key)
	}
	return c, nil
}
