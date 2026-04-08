// Package lru provides a simple LRU cache implementation for deduplication.
package lru

import "container/list"

// Cache is a simple LRU cache for string keys.
// It maintains a fixed capacity and evicts the oldest entries when full.
type Cache struct {
	capacity int
	items    map[string]*list.Element
	order    *list.List // front = newest, back = oldest
}

// New creates a new LRU cache with the given capacity.
func New(capacity int) *Cache {
	return &Cache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

// Contains checks if a key exists in the cache.
func (c *Cache) Contains(key string) bool {
	_, exists := c.items[key]
	return exists
}

// Add adds a key to the cache. If the cache is at capacity,
// the oldest entry is evicted. Returns true if the key was newly added.
func (c *Cache) Add(key string) bool {
	if c.capacity <= 0 {
		return false // Zero or negative capacity means no caching
	}

	if _, exists := c.items[key]; exists {
		return false
	}

	// Evict oldest if at capacity
	if c.order.Len() >= c.capacity {
		oldest := c.order.Back()
		if oldest != nil {
			delete(c.items, oldest.Value.(string))
			c.order.Remove(oldest)
		}
	}

	// Add new entry at front
	elem := c.order.PushFront(key)
	c.items[key] = elem
	return true
}

// Len returns the current number of items in the cache.
func (c *Cache) Len() int {
	return len(c.items)
}
