package tools

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// CacheEntry 缓存条目
type CacheEntry struct {
	Result    string
	Error     string
	IsError   bool
	CachedAt  time.Time
}

// ToolCache 工具结果缓存
type ToolCache struct {
	mu       sync.RWMutex
	entries  map[string]*CacheEntry
	ttl      time.Duration
	maxSize  int
}

// NewToolCache 创建工具缓存
func NewToolCache(ttl time.Duration, maxSize int) *ToolCache {
	return &ToolCache{
		entries: make(map[string]*CacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// CacheKey 生成缓存键
func CacheKey(toolName string, args map[string]interface{}) string {
	// 排序keys确保确定性
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	key := toolName + ":"
	for _, k := range keys {
		key += k + "=" + formatCacheValue(args[k]) + ";"
	}
	return key
}

// Get 获取缓存
func (c *ToolCache) Get(key string) (*CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	// 检查TTL
	if time.Since(entry.CachedAt) > c.ttl {
		return nil, false
	}

	return entry, true
}

// Set 设置缓存
func (c *ToolCache) Set(key string, result string, isError bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 如果满了，清理过期条目
	if len(c.entries) >= c.maxSize {
		c.evict()
	}

	c.entries[key] = &CacheEntry{
		Result:   result,
		IsError:  isError,
		CachedAt: time.Now(),
	}
}

// Invalidate 使缓存失效
func (c *ToolCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// InvalidatePrefix 使匹配前缀的缓存失效
func (c *ToolCache) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.entries, key)
		}
	}
}

// Clear 清空所有缓存
func (c *ToolCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*CacheEntry)
}

// Size 返回缓存大小
func (c *ToolCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// evict 清理过期或最旧的条目（在锁内调用）
func (c *ToolCache) evict() {
	now := time.Now()
	// 先清过期的
	for key, entry := range c.entries {
		if now.Sub(entry.CachedAt) > c.ttl {
			delete(c.entries, key)
		}
	}

	// 如果还是满的，删最旧的
	if len(c.entries) >= c.maxSize {
		var oldestKey string
		var oldestTime time.Time
		first := true
		for key, entry := range c.entries {
			if first || entry.CachedAt.Before(oldestTime) {
				oldestKey = key
				oldestTime = entry.CachedAt
				first = false
			}
		}
		if oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}
}

// CachedTool 包装一个Tool，添加缓存层
type CachedTool struct {
	Tool  Tool
	Cache *ToolCache
	// 哪些工具启用缓存
	cacheable bool
}

// ShouldCache 判断工具是否应该缓存
func ShouldCache(toolName string) bool {
	// 只读操作缓存，写操作不缓存
	switch toolName {
	case "read_file", "list_dir", "glob_find", "grep_search":
		return true
	default:
		return false
	}
}

// ExecuteWithCache 带缓存的工具执行
func ExecuteWithCache(ctx context.Context, tool Tool, args map[string]interface{}, cache *ToolCache) (string, error) {
	if cache == nil || !ShouldCache(tool.Name()) {
		return tool.Execute(ctx, args)
	}

	key := CacheKey(tool.Name(), args)

	// 尝试读缓存
	if entry, ok := cache.Get(key); ok {
		if entry.IsError {
			return entry.Result, fmt.Errorf("%s", entry.Error)
		}
		return entry.Result, nil
	}

	// 执行并缓存结果
	result, err := tool.Execute(ctx, args)
	if err != nil {
		cache.Set(key, result, true)
		return result, err
	}

	cache.Set(key, result, false)
	return result, nil
}

// InvalidateFileCache 文件修改后使相关缓存失效
func InvalidateFileCache(cache *ToolCache, path string) {
	if cache == nil {
		return
	}
	// 清除所有文件相关缓存（简单策略：全部清除）
	cache.Clear()
}

func formatCacheValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%v", val)
	case bool:
		return fmt.Sprintf("%v", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
