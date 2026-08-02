package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"taiji-code/types"
)

// GrepSearch searches file contents using regex
type GrepSearch struct{}

func (t *GrepSearch) Name() string        { return "grep_search" }
func (t *GrepSearch) Description() string  { return "使用正则表达式搜索文件内容。支持递归搜索、文件类型过滤、上下文显示。类似 ripgrep。" }
func (t *GrepSearch) PermissionLevel() types.PermissionLevel { return types.PermAutoApprove }

func (t *GrepSearch) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "正则表达式搜索模式",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "搜索目录(默认当前目录)",
			},
			"include": map[string]interface{}{
				"type":        "string",
				"description": "文件名过滤glob(如 *.go, *.ts)",
			},
			"context": map[string]interface{}{
				"type":        "integer",
				"description": "显示匹配行前后上下文行数(默认0)",
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "最大结果数(默认50)",
			},
		},
		"required": []interface{}{"pattern"},
	}
}

func (t *GrepSearch) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	root := "."
	if p, ok := args["path"].(string); ok && p != "" {
		root = expandPath(p)
	}

	include := ""
	if v, ok := args["include"].(string); ok {
		include = v
	}

	contextLines := 0
	if v, ok := args["context"].(float64); ok && v > 0 {
		contextLines = int(v)
	}

	maxResults := 50
	if v, ok := args["max_results"].(float64); ok && v > 0 {
		maxResults = int(v)
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("无效的正则表达式: %w", err)
	}

	type Match struct {
		File    string
		Line    int
		Content string
	}

	var matches []Match

	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip hidden and common non-source dirs
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			switch name {
			case "node_modules", "vendor", "__pycache__", ".git", "dist", "build", "target":
				return filepath.SkipDir
			}
		}

		if info.IsDir() || info.Size() > 1<<20 { // Skip dirs and files > 1MB
			return nil
		}

		// Apply include filter
		if include != "" {
			matched, _ := filepath.Match(include, info.Name())
			if !matched {
				return nil
			}
		}

		// Skip binary files
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if isBinary(data) {
			return nil
		}

		// Search line by line
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				matches = append(matches, Match{
					File:    path,
					Line:    i + 1,
					Content: line,
				})
				if len(matches) >= maxResults {
					return filepath.SkipAll
				}
			}
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return "", fmt.Errorf("搜索失败: %w", err)
	}

	if len(matches) == 0 {
		return fmt.Sprintf("未找到匹配 '%s' 的内容", pattern), nil
	}

	// Group by file
	fileMatches := make(map[string][]Match)
	for _, m := range matches {
		fileMatches[m.File] = append(fileMatches[m.File], m)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("搜索 '%s' 的结果 (%d 处匹配):\n\n", pattern, len(matches)))

	// Sort files
	var files []string
	for f := range fileMatches {
		files = append(files, f)
	}
	sort.Strings(files)

	for _, file := range files {
		ms := fileMatches[file]
		sb.WriteString(fmt.Sprintf("  %s:\n", file))
		for _, m := range ms {
			// Add context if requested
			if contextLines > 0 {
				data, _ := os.ReadFile(file)
				allLines := strings.Split(string(data), "\n")
				start := m.Line - 1 - contextLines
				if start < 0 {
					start = 0
				}
				end := m.Line + contextLines
				if end > len(allLines) {
					end = len(allLines)
				}
				for i := start; i < end; i++ {
					prefix := "  "
					if i == m.Line-1 {
						prefix = "> "
					}
					sb.WriteString(fmt.Sprintf("    %s%4d | %s\n", prefix, i+1, allLines[i]))
				}
			} else {
				content := strings.TrimSpace(m.Content)
				if len(content) > 200 {
					content = content[:200] + "..."
				}
				sb.WriteString(fmt.Sprintf("    %4d | %s\n", m.Line, content))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// GrepFile is a helper to search within a single file (used internally)
func GrepFile(path, pattern string) ([]string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var matches []string
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			matches = append(matches, fmt.Sprintf("%d: %s", lineNum, line))
		}
	}

	return matches, scanner.Err()
}
