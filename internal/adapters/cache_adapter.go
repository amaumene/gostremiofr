package adapters

import "github.com/amaumene/gostremiofr/internal/cache"

// CacheAdapter adapts the internal LRUCache to the torrentsearch Cache interface
type CacheAdapter struct {
	cache *cache.LRUCache
}

func NewCacheAdapter(c *cache.LRUCache) *CacheAdapter {
	return &CacheAdapter{cache: c}
}

func (c *CacheAdapter) Get(key string) (interface{}, bool) {
	return c.cache.Get(key)
}

func (c *CacheAdapter) Set(key string, value interface{}) {
	c.cache.Set(key, value)
}