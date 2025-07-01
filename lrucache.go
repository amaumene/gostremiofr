package main

import (
	"container/list"
	"sync"
	"time"
)

// LRUCacheItem represents an item in the LRU cache
type LRUCacheItem struct {
	Key        string
	Value      *TMDBData
	Expiration time.Time
}

// LRUCache implements a thread-safe LRU cache with TTL
type LRUCache struct {
	capacity int
	items    map[string]*list.Element
	evictList *list.List
	mu       sync.RWMutex
	ttl      time.Duration
}

// NewLRUCache creates a new LRU cache with the given capacity and TTL
func NewLRUCache(capacity int, ttl time.Duration) *LRUCache {
	return &LRUCache{
		capacity:  capacity,
		items:     make(map[string]*list.Element),
		evictList: list.New(),
		ttl:       ttl,
	}
}

// Get retrieves a value from the cache
func (c *LRUCache) Get(key string) (*TMDBData, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		item := elem.Value.(*LRUCacheItem)
		
		// Check if item has expired
		if time.Now().After(item.Expiration) {
			c.removeElement(elem)
			return nil, false
		}
		
		// Move to front
		c.evictList.MoveToFront(elem)
		return item.Value, true
	}
	
	return nil, false
}

// Set adds or updates a value in the cache
func (c *LRUCache) Set(key string, value *TMDBData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiration := time.Now().Add(c.ttl)
	
	if elem, ok := c.items[key]; ok {
		// Update existing item
		item := elem.Value.(*LRUCacheItem)
		item.Value = value
		item.Expiration = expiration
		c.evictList.MoveToFront(elem)
		return
	}

	// Add new item
	item := &LRUCacheItem{
		Key:        key,
		Value:      value,
		Expiration: expiration,
	}
	
	elem := c.evictList.PushFront(item)
	c.items[key] = elem

	// Evict oldest if over capacity
	if c.evictList.Len() > c.capacity {
		c.removeOldest()
	}
}

// removeOldest removes the oldest item from the cache
func (c *LRUCache) removeOldest() {
	elem := c.evictList.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

// removeElement removes a specific element from the cache
func (c *LRUCache) removeElement(elem *list.Element) {
	c.evictList.Remove(elem)
	item := elem.Value.(*LRUCacheItem)
	delete(c.items, item.Key)
}

// CleanExpired removes all expired items from the cache
func (c *LRUCache) CleanExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var toRemove []*list.Element
	
	for elem := c.evictList.Back(); elem != nil; elem = elem.Prev() {
		item := elem.Value.(*LRUCacheItem)
		if now.After(item.Expiration) {
			toRemove = append(toRemove, elem)
		}
	}
	
	for _, elem := range toRemove {
		c.removeElement(elem)
	}
}

// Global TMDB cache instance
var tmdbMemoryCache = NewLRUCache(1000, 6*time.Hour)