package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"taiji-code/types"
)

// ListDir lists directory contents
type ListDir struct{}

func (t *ListDir) Name() string        { return "list_dir" }
func (t *ListDir) Description() string  { return "列出目录中的文件和子目录。显示文件类型、大小和修改时间。" }
func (t *ListDir) PermissionLevel() types.PermissionLevel { return types.PermAutoApprove }

func (t *ListDir) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "目录路径(默认当前目录)",
			},
		},
	}
}

func (t *ListDir) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = expandPath(p)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("读取目录失败: %w", err)
	}

	var dirs, files []string
	for _, entry := range entries {
		name := entry.Name()
		// Skip hidden files by default
		if strings.HasPrefix(name, ".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if entry.IsDir() {
			dirs = append(dirs, name+"/")
		} else {
			size := formatSize(info.Size())
			files = append(files, fmt.Sprintf("%s (%s)", name, size))
		}
	}

	sort.Strings(dirs)
	sort.Strings(files)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("目录: %s\n\n", path))

	if len(dirs) > 0 {
		sb.WriteString("目录:\n")
		for _, d := range dirs {
			sb.WriteString("  " + d + "\n")
		}
	}

	if len(files) > 0 {
		sb.WriteString("\n文件:\n")
		for _, f := range files {
			sb.WriteString("  " + f + "\n")
		}
	}

	sb.WriteString(fmt.Sprintf("\n共 %d 个目录, %d 个文件", len(dirs), len(files)))
	return sb.String(), nil
}

// GlobFind finds files matching a pattern
type GlobFind struct{}

func (t *GlobFind) Name() string        { return "glob_find" }
func (t *GlobFind) Description() string  { return "使用glob模式查找文件。支持 ** 递归匹配。例如: **/*.go 查找所有Go文件, src/**/test_* 查找test_开头的文件。" }
func (t *GlobFind) PermissionLevel() types.PermissionLevel { return types.PermAutoApprove }

func (t *GlobFind) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Glob模式(如 **/*.go, src/**/*.ts)",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "搜索根目录(默认当前目录)",
			},
		},
		"required": []interface{}{"pattern"},
	}
}

func (t *GlobFind) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	root := "."
	if p, ok := args["path"].(string); ok && p != "" {
		root = expandPath(p)
	}

	var matches []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip hidden directories
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
			return filepath.SkipDir
		}

		// Skip common non-source directories
		if info.IsDir() {
			switch info.Name() {
			case "node_modules", "vendor", "__pycache__", ".git", "dist", "build":
				return filepath.SkipDir
			}
		}

		matched, err := matchGlob(pattern, path)
		if err != nil {
			return nil
		}
		if matched {
			matches = append(matches, path)
		}
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("搜索失败: %w", err)
	}

	if len(matches) == 0 {
		return fmt.Sprintf("未找到匹配 '%s' 的文件", pattern), nil
	}

	sort.Strings(matches)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("匹配 '%s' 的文件 (%d 个):\n\n", pattern, len(matches)))
	for _, m := range matches {
		sb.WriteString("  " + m + "\n")
	}

	return sb.String(), nil
}

// matchGlob handles ** recursive glob patterns
func matchGlob(pattern, path string) (bool, error) {
	// Normalize separators
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)

	// Handle ** pattern
	if strings.Contains(pattern, "**") {
		parts := strings.SplitN(pattern, "**", 2)
		prefix := parts[0]
		suffix := parts[1]

		// Check prefix
		if prefix != "" && !strings.HasPrefix(path, prefix) {
			return false, nil
		}

		// Check suffix with simple glob
		if suffix == "" || suffix == "/" {
			return true, nil
		}

		suffix = strings.TrimPrefix(suffix, "/")
		matched, _ := filepath.Match(suffix, filepath.Base(path))
		return matched, nil
	}

	// Simple glob
	matched, err := filepath.Match(pattern, path)
	return matched, err
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
