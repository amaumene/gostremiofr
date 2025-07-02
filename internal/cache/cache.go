package cache

import (
	"container/list"
	"context"
	"sync"
	"time"
)

type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{})
	Delete(key string)
	Clear()
}

type Item struct {
	Key        string
	Value      interface{}
	Expiration time.Time
}

type LRUCache struct {
	capacity  int
	items     map[string]*list.Element
	evictList *list.List
	mu        sync.RWMutex
	ttl       time.Duration
}

func New(capacity int, ttl time.Duration) *LRUCache {
	return &LRUCache{
		capacity:  capacity,
		items:     make(map[string]*list.Element),
		evictList: list.New(),
		ttl:       ttl,
	}
}

func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		item := elem.Value.(*Item)
		
		if time.Now().After(item.Expiration) {
			c.removeElement(elem)
			return nil, false
		}
		
		c.evictList.MoveToFront(elem)
		return item.Value, true
	}
	
	return nil, false
}

func (c *LRUCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiration := time.Now().Add(c.ttl)
	
	if elem, ok := c.items[key]; ok {
		item := elem.Value.(*Item)
		item.Value = value
		item.Expiration = expiration
		c.evictList.MoveToFront(elem)
		return
	}

	item := &Item{
		Key:        key,
		Value:      value,
		Expiration: expiration,
	}
	
	elem := c.evictList.PushFront(item)
	c.items[key] = elem

	if c.evictList.Len() > c.capacity {
		c.removeOldest()
	}
}

func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.removeElement(elem)
	}
}

func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.evictList.Init()
}

func (c *LRUCache) removeOldest() {
	elem := c.evictList.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

func (c *LRUCache) removeElement(elem *list.Element) {
	c.evictList.Remove(elem)
	item := elem.Value.(*Item)
	delete(c.items, item.Key)
}

func (c *LRUCache) CleanExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var toRemove []*list.Element
	
	for elem := c.evictList.Back(); elem != nil; elem = elem.Prev() {
		item := elem.Value.(*Item)
		if now.After(item.Expiration) {
			toRemove = append(toRemove, elem)
		}
	}
	
	for _, elem := range toRemove {
		c.removeElement(elem)
	}
}

func (c *LRUCache) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
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