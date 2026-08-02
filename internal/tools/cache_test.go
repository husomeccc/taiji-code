package tools

import (
	"testing"
	"time"
)

func TestCacheKeyDeterminism(t *testing.T) {
	args := map[string]interface{}{
		"path":   "/tmp/test.go",
		"line":   float64(42),
		"hidden": true,
	}

	first := CacheKey("read_file", args)
	for i := 0; i < 100; i++ {
		got := CacheKey("read_file", args)
		if got != first {
			t.Fatalf("iteration %d: CacheKey returned %q, expected %q", i, got, first)
		}
	}
}

func TestCacheKeyDifferentArgs(t *testing.T) {
	args1 := map[string]interface{}{"path": "/tmp/a.go"}
	args2 := map[string]interface{}{"path": "/tmp/b.go"}

	key1 := CacheKey("read_file", args1)
	key2 := CacheKey("read_file", args2)
	if key1 == key2 {
		t.Fatalf("expected different keys for different args, both were %q", key1)
	}
}

func TestCacheKeyDifferentToolNames(t *testing.T) {
	args := map[string]interface{}{"query": "test"}

	key1 := CacheKey("grep_search", args)
	key2 := CacheKey("read_file", args)
	if key1 == key2 {
		t.Fatalf("expected different keys for different tool names, both were %q", key1)
	}
}

func TestToolCacheGetSet(t *testing.T) {
	cache := NewToolCache(5*time.Minute, 100)

	cache.Set("key1", "result1", false)

	entry, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected to find key1 in cache")
	}
	if entry.Result != "result1" {
		t.Fatalf("expected Result %q, got %q", "result1", entry.Result)
	}
	if entry.IsError {
		t.Fatal("expected IsError to be false")
	}
}

func TestToolCacheGetMiss(t *testing.T) {
	cache := NewToolCache(5*time.Minute, 100)

	_, ok := cache.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss for nonexistent key")
	}
}

func TestToolCacheSetError(t *testing.T) {
	cache := NewToolCache(5*time.Minute, 100)

	cache.Set("errkey", "some error", true)

	entry, ok := cache.Get("errkey")
	if !ok {
		t.Fatal("expected to find errkey in cache")
	}
	if !entry.IsError {
		t.Fatal("expected IsError to be true")
	}
	if entry.Result != "some error" {
		t.Fatalf("expected Result %q, got %q", "some error", entry.Result)
	}
}

func TestToolCacheTTLExpiry(t *testing.T) {
	cache := NewToolCache(50*time.Millisecond, 100)

	cache.Set("ttlkey", "value", false)

	// Should be available immediately
	if _, ok := cache.Get("ttlkey"); !ok {
		t.Fatal("expected key to be present immediately after Set")
	}

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	if _, ok := cache.Get("ttlkey"); ok {
		t.Fatal("expected key to be expired after TTL")
	}
}

func TestToolCacheClear(t *testing.T) {
	cache := NewToolCache(5*time.Minute, 100)

	cache.Set("a", "1", false)
	cache.Set("b", "2", false)
	cache.Set("c", "3", false)

	if cache.Size() != 3 {
		t.Fatalf("expected size 3, got %d", cache.Size())
	}

	cache.Clear()

	if cache.Size() != 0 {
		t.Fatalf("expected size 0 after Clear, got %d", cache.Size())
	}

	if _, ok := cache.Get("a"); ok {
		t.Fatal("expected cache miss after Clear")
	}
}

func TestToolCacheSize(t *testing.T) {
	cache := NewToolCache(5*time.Minute, 100)

	if cache.Size() != 0 {
		t.Fatalf("expected initial size 0, got %d", cache.Size())
	}

	cache.Set("k1", "v1", false)
	if cache.Size() != 1 {
		t.Fatalf("expected size 1, got %d", cache.Size())
	}

	cache.Set("k2", "v2", false)
	if cache.Size() != 2 {
		t.Fatalf("expected size 2, got %d", cache.Size())
	}

	// Overwriting an existing key should not increase size
	cache.Set("k1", "v1-updated", false)
	if cache.Size() != 2 {
		t.Fatalf("expected size 2 after overwrite, got %d", cache.Size())
	}
}

func TestToolCacheInvalidate(t *testing.T) {
	cache := NewToolCache(5*time.Minute, 100)

	cache.Set("inv-key", "value", false)
	cache.Invalidate("inv-key")

	if _, ok := cache.Get("inv-key"); ok {
		t.Fatal("expected cache miss after Invalidate")
	}
	if cache.Size() != 0 {
		t.Fatalf("expected size 0 after Invalidate, got %d", cache.Size())
	}
}

func TestToolCacheInvalidatePrefix(t *testing.T) {
	cache := NewToolCache(5*time.Minute, 100)

	cache.Set("read_file:path=a", "1", false)
	cache.Set("read_file:path=b", "2", false)
	cache.Set("grep_search:query=c", "3", false)

	cache.InvalidatePrefix("read_file:")

	if cache.Size() != 1 {
		t.Fatalf("expected size 1 after InvalidatePrefix, got %d", cache.Size())
	}
	if _, ok := cache.Get("grep_search:query=c"); !ok {
		t.Fatal("expected grep_search entry to survive InvalidatePrefix")
	}
}

func TestShouldCache(t *testing.T) {
	readOnlyTools := []string{"read_file", "list_dir", "glob_find", "grep_search"}
	for _, name := range readOnlyTools {
		if !ShouldCache(name) {
			t.Errorf("expected ShouldCache(%q) to be true", name)
		}
	}

	writeTools := []string{"write_file", "edit_file", "run_command", "web_fetch", "web_search"}
	for _, name := range writeTools {
		if ShouldCache(name) {
			t.Errorf("expected ShouldCache(%q) to be false", name)
		}
	}
}

func TestCacheKeyEmptyArgs(t *testing.T) {
	key := CacheKey("list_dir", map[string]interface{}{})
	expected := "list_dir:"
	if key != expected {
		t.Fatalf("expected %q, got %q", expected, key)
	}
}

func TestCacheKeyFormat(t *testing.T) {
	args := map[string]interface{}{
		"path": "/tmp/test.go",
	}
	key := CacheKey("read_file", args)
	expected := "read_file:path=/tmp/test.go;"
	if key != expected {
		t.Fatalf("expected %q, got %q", expected, key)
	}
}

func TestCacheKeySortedOrder(t *testing.T) {
	args := map[string]interface{}{
		"z_param": "last",
		"a_param": "first",
		"m_param": "middle",
	}
	key := CacheKey("tool", args)
	// Keys should be sorted alphabetically
	expected := "tool:a_param=first;m_param=middle;z_param=last;"
	if key != expected {
		t.Fatalf("expected %q, got %q", expected, key)
	}
}
