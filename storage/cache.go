package storage

import (
	"context"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

var _ Storage = (*InMemoryCache)(nil)

// InMemoryCache implements Storage and caches repository configurations in memory using an LRU cache.
type InMemoryCache struct {
	cache *expirable.LRU[string, *RepoConfig]
	next  Storage
}

// NewInMemoryCache creates a new InMemoryCache with the specified cache size and TTL, backed by the next Storage.
func NewInMemoryCache(size int, ttl time.Duration, next Storage) *InMemoryCache {
	cache := expirable.NewLRU[string, *RepoConfig](size, nil, ttl)
	return &InMemoryCache{
		cache: cache,
		next:  next,
	}
}

func (s *InMemoryCache) Get(ctx context.Context, path string) (*RepoConfig, error) {
	ctx, span := tracer.Start(ctx, "InMemoryCache.Get")
	defer span.End()

	if val, ok := s.cache.Get(path); ok {
		return val, nil
	}

	config, err := s.next.Get(ctx, path)
	if err != nil {
		return nil, err
	}

	// Cache the result, even if it is nil (negative caching)
	s.cache.Add(path, config)
	return config, nil
}

func (s *InMemoryCache) Set(ctx context.Context, path string, config *RepoConfig) error {
	ctx, span := tracer.Start(ctx, "InMemoryCache.Set")
	defer span.End()

	if err := s.next.Set(ctx, path, config); err != nil {
		return err
	}
	s.cache.Add(path, config)
	return nil
}

func (s *InMemoryCache) ListAll(ctx context.Context) ([]string, error) {
	ctx, span := tracer.Start(ctx, "InMemoryCache.ListAll")
	defer span.End()

	// For listing, we bypass cache and ask the underlying storage.
	// We could potentially cache the list, but it's complex to invalidate directly.
	return s.next.ListAll(ctx)
}

func (s *InMemoryCache) Delete(ctx context.Context, path string) error {
	ctx, span := tracer.Start(ctx, "InMemoryCache.Delete")
	defer span.End()

	if err := s.next.Delete(ctx, path); err != nil {
		return err
	}
	s.cache.Remove(path)
	return nil
}

// Clear purges all items from the cache.
func (s *InMemoryCache) Clear(ctx context.Context) {
	_, span := tracer.Start(ctx, "InMemoryCache.Clear")
	defer span.End()
	s.cache.Purge()
}

func (s *InMemoryCache) Close(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "InMemoryCache.Close")
	defer span.End()
	s.Clear(ctx)
	if s.next != nil {
		return s.next.Close(ctx)
	}
	return nil
}
