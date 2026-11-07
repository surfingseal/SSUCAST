package crawler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"example.com/crawlserver/internal/model"
)

type marker struct{ name string }

func (m marker) Crawl(context.Context, model.Source) ([]model.Content, error) { return nil, nil }
func TestRegistrySelectionAndErrors(t *testing.T) {
	r := NewRegistry()
	if err := r.Register("site", func(s model.Source) (Crawler, error) { return marker{s.Name}, nil }); err != nil {
		t.Fatal(err)
	}
	for _, s := range []model.Source{{Name: "first", CrawlerType: "site"}, {Name: "second", CrawlerType: "site"}, {Name: "site"}} {
		c, err := r.Resolve(s)
		if err != nil || c.(marker).name != s.Name {
			t.Fatal(c, err)
		}
	}
	if err := r.Register("site", func(model.Source) (Crawler, error) { return marker{}, nil }); err == nil {
		t.Fatal("duplicate type accepted")
	}
	if _, err := r.Resolve(model.Source{Name: "unknown"}); err == nil {
		t.Fatal("unknown type accepted")
	}
	if err := r.Register("nil", func(model.Source) (Crawler, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve(model.Source{CrawlerType: "nil"}); err == nil {
		t.Fatal("nil crawler accepted")
	}
	if err := r.Register("bad", func(model.Source) (Crawler, error) { return nil, errors.New("factory error") }); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve(model.Source{CrawlerType: "bad"}); err == nil {
		t.Fatal("factory error lost")
	}
}
func TestRegistryConcurrentAccess(t *testing.T) {
	r, err := DefaultRegistry(NewFetcher(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.Resolve(model.Source{CrawlerType: "example"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
