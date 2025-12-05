package cloudwatch

import (
	"fmt"
	"testing"
)

func TestNewLRUCache(t *testing.T) {
	cache := newLRUCache(100)

	if cache.capacity != 100 {
		t.Errorf("expected capacity 100, got %d", cache.capacity)
	}
	if cache.order.Len() != 0 {
		t.Errorf("expected empty list, got length %d", cache.order.Len())
	}
	if len(cache.items) != 0 {
		t.Errorf("expected empty map, got length %d", len(cache.items))
	}
}

func TestLRUCache_Add(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
		keys     []string
		wantLen  int
	}{
		{
			name:     "add single item",
			capacity: 10,
			keys:     []string{"a"},
			wantLen:  1,
		},
		{
			name:     "add multiple items under capacity",
			capacity: 10,
			keys:     []string{"a", "b", "c"},
			wantLen:  3,
		},
		{
			name:     "add items at capacity",
			capacity: 3,
			keys:     []string{"a", "b", "c"},
			wantLen:  3,
		},
		{
			name:     "add items over capacity evicts oldest",
			capacity: 3,
			keys:     []string{"a", "b", "c", "d"},
			wantLen:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newLRUCache(tt.capacity)
			for _, key := range tt.keys {
				cache.add(key)
			}

			if cache.order.Len() != tt.wantLen {
				t.Errorf("expected length %d, got %d", tt.wantLen, cache.order.Len())
			}
			if len(cache.items) != tt.wantLen {
				t.Errorf("expected items map length %d, got %d", tt.wantLen, len(cache.items))
			}
		})
	}
}

func TestLRUCache_AddReturnValue(t *testing.T) {
	cache := newLRUCache(10)

	// First add should return true
	if !cache.add("a") {
		t.Error("expected true for new key")
	}

	// Duplicate add should return false
	if cache.add("a") {
		t.Error("expected false for duplicate key")
	}

	// Different key should return true
	if !cache.add("b") {
		t.Error("expected true for new key")
	}
}

func TestLRUCache_Contains(t *testing.T) {
	cache := newLRUCache(10)

	// Empty cache should not contain anything
	if cache.contains("a") {
		t.Error("expected empty cache to not contain 'a'")
	}

	// After adding, should contain
	cache.add("a")
	if !cache.contains("a") {
		t.Error("expected cache to contain 'a' after add")
	}

	// Should not contain other keys
	if cache.contains("b") {
		t.Error("expected cache to not contain 'b'")
	}
}

func TestLRUCache_EvictionOrder(t *testing.T) {
	cache := newLRUCache(3)

	// Add items a, b, c (a is oldest)
	cache.add("a")
	cache.add("b")
	cache.add("c")

	// All should be present
	if !cache.contains("a") || !cache.contains("b") || !cache.contains("c") {
		t.Error("expected all items to be present")
	}

	// Adding d should evict a (oldest)
	cache.add("d")

	if cache.contains("a") {
		t.Error("expected 'a' to be evicted")
	}
	if !cache.contains("b") {
		t.Error("expected 'b' to still be present")
	}
	if !cache.contains("c") {
		t.Error("expected 'c' to still be present")
	}
	if !cache.contains("d") {
		t.Error("expected 'd' to be present")
	}

	// Adding e should evict b (now oldest)
	cache.add("e")

	if cache.contains("b") {
		t.Error("expected 'b' to be evicted")
	}
	if !cache.contains("c") {
		t.Error("expected 'c' to still be present")
	}
	if !cache.contains("d") {
		t.Error("expected 'd' to still be present")
	}
	if !cache.contains("e") {
		t.Error("expected 'e' to be present")
	}
}

func TestLRUCache_DuplicateDoesNotEvict(t *testing.T) {
	cache := newLRUCache(3)

	cache.add("a")
	cache.add("b")
	cache.add("c")

	// Adding duplicate should not evict anything
	cache.add("b")

	if !cache.contains("a") || !cache.contains("b") || !cache.contains("c") {
		t.Error("adding duplicate should not evict any items")
	}

	if cache.order.Len() != 3 {
		t.Errorf("expected length 3, got %d", cache.order.Len())
	}
}

func TestLRUCache_ZeroCapacity(t *testing.T) {
	cache := newLRUCache(0)

	// Adding to zero capacity cache should still evict immediately
	cache.add("a")

	// With capacity 0, nothing should be stored
	if cache.order.Len() != 0 {
		t.Errorf("expected length 0 for zero-capacity cache, got %d", cache.order.Len())
	}
}

func TestLRUCache_SingleCapacity(t *testing.T) {
	cache := newLRUCache(1)

	cache.add("a")
	if !cache.contains("a") {
		t.Error("expected 'a' to be present")
	}

	cache.add("b")
	if cache.contains("a") {
		t.Error("expected 'a' to be evicted")
	}
	if !cache.contains("b") {
		t.Error("expected 'b' to be present")
	}
}

func TestLRUCache_LargeCapacity(t *testing.T) {
	capacity := 10000
	cache := newLRUCache(capacity)

	// Fill to capacity
	for i := 0; i < capacity; i++ {
		cache.add(fmt.Sprintf("key%d", i))
	}

	if cache.order.Len() != capacity {
		t.Errorf("expected length %d, got %d", capacity, cache.order.Len())
	}

	// Add one more, should evict oldest (key0)
	cache.add("new_key")

	if cache.contains("key0") {
		t.Error("expected 'key0' to be evicted")
	}
	if !cache.contains("new_key") {
		t.Error("expected 'new_key' to be present")
	}
	if !cache.contains(fmt.Sprintf("key%d", capacity-1)) {
		t.Error("expected last key to still be present")
	}
}

// Benchmark for typical usage pattern
func BenchmarkLRUCache_Add(b *testing.B) {
	cache := newLRUCache(DefaultLRUCacheCapacity)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.add(fmt.Sprintf("key%d", i))
	}
}

func BenchmarkLRUCache_Contains(b *testing.B) {
	cache := newLRUCache(DefaultLRUCacheCapacity)

	// Pre-fill cache
	for i := 0; i < DefaultLRUCacheCapacity; i++ {
		cache.add(fmt.Sprintf("key%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.contains(fmt.Sprintf("key%d", i%DefaultLRUCacheCapacity))
	}
}

func BenchmarkLRUCache_AddWithEviction(b *testing.B) {
	cache := newLRUCache(1000) // Smaller cache to force evictions

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.add(fmt.Sprintf("key%d", i))
	}
}
