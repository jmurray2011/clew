package lru

import (
	"fmt"
	"testing"
)

func TestNew(t *testing.T) {
	cache := New(100)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	if cache.capacity != 100 {
		t.Errorf("capacity = %d, want 100", cache.capacity)
	}
	if cache.Len() != 0 {
		t.Errorf("initial Len() = %d, want 0", cache.Len())
	}
}

func TestCache_Add(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
		adds     []string
		wantLen  int
	}{
		{
			name:     "add within capacity",
			capacity: 5,
			adds:     []string{"a", "b", "c"},
			wantLen:  3,
		},
		{
			name:     "add up to capacity",
			capacity: 3,
			adds:     []string{"a", "b", "c"},
			wantLen:  3,
		},
		{
			name:     "add exceeds capacity",
			capacity: 3,
			adds:     []string{"a", "b", "c", "d", "e"},
			wantLen:  3,
		},
		{
			name:     "zero capacity",
			capacity: 0,
			adds:     []string{"a", "b", "c"},
			wantLen:  0,
		},
		{
			name:     "negative capacity",
			capacity: -1,
			adds:     []string{"a", "b"},
			wantLen:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := New(tt.capacity)
			for _, key := range tt.adds {
				cache.Add(key)
			}
			if cache.Len() != tt.wantLen {
				t.Errorf("Len() = %d, want %d", cache.Len(), tt.wantLen)
			}
		})
	}
}

func TestCache_AddReturnValue(t *testing.T) {
	cache := New(10)

	// First add should return true
	if !cache.Add("key1") {
		t.Error("first Add('key1') should return true")
	}

	// Duplicate add should return false
	if cache.Add("key1") {
		t.Error("duplicate Add('key1') should return false")
	}

	// New key should return true
	if !cache.Add("key2") {
		t.Error("Add('key2') should return true")
	}
}

func TestCache_Contains(t *testing.T) {
	cache := New(10)

	// Key not in cache
	if cache.Contains("key1") {
		t.Error("Contains('key1') should be false for empty cache")
	}

	// Add key
	cache.Add("key1")
	if !cache.Contains("key1") {
		t.Error("Contains('key1') should be true after adding")
	}

	// Different key still not present
	if cache.Contains("key2") {
		t.Error("Contains('key2') should be false")
	}
}

func TestCache_EvictionOrder(t *testing.T) {
	cache := New(3)

	// Add three items
	cache.Add("a")
	cache.Add("b")
	cache.Add("c")

	// All should be present
	if !cache.Contains("a") || !cache.Contains("b") || !cache.Contains("c") {
		t.Error("all three items should be present")
	}

	// Add fourth item - should evict "a" (oldest)
	cache.Add("d")

	if cache.Contains("a") {
		t.Error("'a' should have been evicted")
	}
	if !cache.Contains("b") || !cache.Contains("c") || !cache.Contains("d") {
		t.Error("'b', 'c', 'd' should still be present")
	}

	// Add fifth item - should evict "b"
	cache.Add("e")

	if cache.Contains("b") {
		t.Error("'b' should have been evicted")
	}
	if !cache.Contains("c") || !cache.Contains("d") || !cache.Contains("e") {
		t.Error("'c', 'd', 'e' should still be present")
	}
}

func TestCache_DuplicateDoesNotEvict(t *testing.T) {
	cache := New(3)

	cache.Add("a")
	cache.Add("b")
	cache.Add("c")

	// Re-adding "a" should not evict anything
	cache.Add("a")

	if !cache.Contains("a") || !cache.Contains("b") || !cache.Contains("c") {
		t.Error("all three items should still be present after duplicate add")
	}
	if cache.Len() != 3 {
		t.Errorf("Len() = %d, want 3", cache.Len())
	}
}

func TestCache_SingleCapacity(t *testing.T) {
	cache := New(1)

	cache.Add("a")
	if !cache.Contains("a") {
		t.Error("'a' should be present")
	}

	cache.Add("b")
	if cache.Contains("a") {
		t.Error("'a' should have been evicted")
	}
	if !cache.Contains("b") {
		t.Error("'b' should be present")
	}
}

func BenchmarkCache_Add(b *testing.B) {
	cache := New(10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Add(fmt.Sprintf("key%d", i))
	}
}

func BenchmarkCache_Contains(b *testing.B) {
	cache := New(10000)
	// Pre-populate
	for i := 0; i < 10000; i++ {
		cache.Add(fmt.Sprintf("key%d", i))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Contains(fmt.Sprintf("key%d", i%10000))
	}
}

func BenchmarkCache_AddWithEviction(b *testing.B) {
	cache := New(1000) // Smaller cache to force evictions
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Add(fmt.Sprintf("key%d", i))
	}
}
