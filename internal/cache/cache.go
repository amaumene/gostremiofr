// Package cache provides a thread-safe LRU cache implementation with TTL support.
package cache

import (
	"container/list"
	"context"
	"sync"
	"time"
)

const (
	// Default cleanup interval for expired items
	defaultCleanupInterval = 1 * time.Hour
)

// Cache defines the interface for cache implementations.
type Cache interface {
	// Get retrieves a value by key, returns value and whether it was found
	Get(key string) (interface{}, bool)
	// Set stores a key-value pair with default TTL
	Set(key string, value interface{})
	// Delete removes a key from the cache
	Delete(key string)
	// Clear removes all items from the cache
	Clear()
}

// Item represents a cached item with expiration.
type Item struct {
	Key        string
	Value      interface{}
	Expiration time.Time
}

// LRUCache implements a Least Recently Used cache with time-to-live support.
// It is thread-safe and automatically evicts the least recently used items
// when the capacity is exceeded.
type LRUCache struct {
	capacity  int                       // Maximum number of items
	items     map[string]*list.Element // Map for O(1) lookups
	evictList *list.List               // Doubly linked list for LRU ordering
	mu        sync.RWMutex             // Protects concurrent access
	ttl       time.Duration            // Time-to-live for items
}

// New creates a new LRU cache with the specified capacity and TTL.
// capacity: maximum number of items to store
// ttl: time-to-live for each item
func New(capacity int, ttl time.Duration) *LRUCache {
	return &LRUCache{
		capacity:  capacity,
		items:     make(map[string]*list.Element),
		evictList: list.New(),
		ttl:       ttl,
	}
}

// Get retrieves a value from the cache by key.
// Returns the value and true if found and not expired, nil and false otherwise.
// Updates the item's position to mark it as recently used.
func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}

	item := elem.Value.(*Item)

	// Check expiration
	if time.Now().After(item.Expiration) {
		c.removeElement(elem)
		return nil, false
	}

	// Mark as recently used
	c.evictList.MoveToFront(elem)
	return item.Value, true
}

// Set stores a key-value pair in the cache.
// If the key already exists, it updates the value and expiration.
// Evicts the least recently used item if capacity is exceeded.
func (c *LRUCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiration := time.Now().Add(c.ttl)

	// Update existing item
	if elem, ok := c.items[key]; ok {
		item := elem.Value.(*Item)
		item.Value = value
		item.Expiration = expiration
		c.evictList.MoveToFront(elem)
		return
	}

	// Add new item
	item := &Item{
		Key:        key,
		Value:      value,
		Expiration: expiration,
	}

	elem := c.evictList.PushFront(item)
	c.items[key] = elem

	// Evict if over capacity
	if c.evictList.Len() > c.capacity {
		c.removeOldest()
	}
}

// Delete removes an item from the cache.
func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.removeElement(elem)
	}
}

// Clear removes all items from the cache.
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.evictList.Init()
}

// removeOldest removes the least recently used item from the cache.
// Must be called with lock held.
func (c *LRUCache) removeOldest() {
	elem := c.evictList.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

// removeElement removes a specific element from the cache.
// Must be called with lock held.
func (c *LRUCache) removeElement(elem *list.Element) {
	c.evictList.Remove(elem)
	item := elem.Value.(*Item)
	delete(c.items, item.Key)
}

// CleanExpired removes all expired items from the cache.
// This is called periodically by the cleanup goroutine.
func (c *LRUCache) CleanExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var toRemove []*list.Element

	// Iterate from oldest to newest
	for elem := c.evictList.Back(); elem != nil; elem = elem.Prev() {
		item := elem.Value.(*Item)
		if now.After(item.Expiration) {
			toRemove = append(toRemove, elem)
		}
	}

	// Remove expired items
	for _, elem := range toRemove {
		c.removeElement(elem)
	}
}

// StartCleanup starts a background goroutine that periodically removes expired items.
// The cleanup runs every hour and stops when the context is cancelled.
func (c *LRUCache) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(defaultCleanupInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.CleanExpired()
			case <-ctx.Done():
				return
			}
		}
	}()
}
