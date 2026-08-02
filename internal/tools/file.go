package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"taiji-code/types"
)

// ReadFile reads a file from disk
type ReadFile struct{}

func (t *ReadFile) Name() string        { return "read_file" }
func (t *ReadFile) Description() string  { return "读取文件内容。支持文本文件和二进制文件(返回base64)。可指定行范围读取大文件。" }
func (t *ReadFile) PermissionLevel() types.PermissionLevel { return types.PermAutoApprove }

func (t *ReadFile) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "文件路径(绝对或相对)",
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "起始行号(从1开始,可选)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "读取行数(可选,默认2000行)",
			},
		},
		"required": []interface{}{"path"},
	}
}

func (t *ReadFile) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	path, _ := args["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	// Expand ~ to home directory
	path = expandPath(path)

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("无法访问文件: %w", err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("%s 是一个目录,请使用 list_dir 工具", path)
	}

	// 大文件保护：超过1MB时提示使用offset/limit
	const maxReadSize = 1 * 1024 * 1024
	if info.Size() > int64(maxReadSize) {
		// 检查是否指定了offset/limit
		_, hasOffset := args["offset"]
		_, hasLimit := args["limit"]
		if !hasOffset && !hasLimit {
			return fmt.Sprintf("文件较大 (%d KB)，建议指定 offset 和 limit 参数分段读取。\n或直接指定 offset=1 limit=500 读取前500行。", info.Size()/1024), nil
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取失败: %w", err)
	}

	// Check if binary
	if isBinary(data) {
		encoded := base64.StdEncoding.EncodeToString(data)
		return fmt.Sprintf("[二进制文件] %s (大小: %d bytes)\nBase64:\n%s", path, len(data), encoded), nil
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	// Apply offset/limit
	offset := 0
	if v, ok := args["offset"].(float64); ok && v > 0 {
		offset = int(v) - 1 // Convert to 0-indexed
	}

	limit := 2000
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	if offset >= len(lines) {
		return fmt.Sprintf("[文件 %s 共 %d 行, 偏移量超出范围]", path, len(lines)), nil
	}

	end := offset + limit
	if end > len(lines) {
		end = len(lines)
	}

	// Format with line numbers
	var sb strings.Builder
	for i := offset; i < end; i++ {
		sb.WriteString(fmt.Sprintf("%6d\t%s\n", i+1, lines[i]))
	}

	if end < len(lines) {
		sb.WriteString(fmt.Sprintf("\n[显示第 %d-%d 行, 共 %d 行]", offset+1, end, len(lines)))
	}

	return sb.String(), nil
}

// WriteFile writes content to a file
type WriteFile struct{}

func (t *WriteFile) Name() string        { return "write_file" }
func (t *WriteFile) Description() string  { return "创建新文件或完全覆盖现有文件。如果父目录不存在会自动创建。" }
func (t *WriteFile) PermissionLevel() types.PermissionLevel { return types.PermAskUser }

func (t *WriteFile) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "文件路径(绝对或相对)",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "要写入的文件内容",
			},
		},
		"required": []interface{}{"path", "content"},
	}
}

func (t *WriteFile) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)

	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	path = expandPath(path)

	// Create parent directory
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("写入失败: %w", err)
	}

	return fmt.Sprintf("已成功写入文件: %s (%d bytes)", path, len(content)), nil
}

// EditFile makes precise edits to a file by replacing exact text
type EditFile struct{}

func (t *EditFile) Name() string        { return "edit_file" }
func (t *EditFile) Description() string  { return "通过精确替换文本编辑文件。old_string必须在文件中唯一匹配,建议包含上下文(前后各2-3行)。适合修改函数、修复bug、更新配置等。" }
func (t *EditFile) PermissionLevel() types.PermissionLevel { return types.PermAskUser }

func (t *EditFile) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "文件路径",
			},
			"old_string": map[string]interface{}{
				"type":        "string",
				"description": "要被替换的原始文本(必须在文件中精确匹配,建议包含上下文)",
			},
			"new_string": map[string]interface{}{
				"type":        "string",
				"description": "替换后的新文本",
			},
		},
		"required": []interface{}{"path", "old_string", "new_string"},
	}
}

func (t *EditFile) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	path, _ := args["path"].(string)
	oldStr, _ := args["old_string"].(string)
	newStr, _ := args["new_string"].(string)

	if path == "" || oldStr == "" {
		return "", fmt.Errorf("path and old_string are required")
	}

	path = expandPath(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	content := string(data)

	// Count occurrences
	count := strings.Count(content, oldStr)
	if count == 0 {
		return "", fmt.Errorf("未找到匹配的文本。请确认 old_string 与文件内容完全一致(包括空格和缩进)")
	}
	if count > 1 {
		return "", fmt.Errorf("找到 %d 处匹配,需要唯一匹配。请添加更多上下文(前后各2-3行)使匹配唯一", count)
	}

	// 生成unified diff预览
	diff := generateDiff(path, content, oldStr, newStr)

	// Perform replacement
	result := strings.Replace(content, oldStr, newStr, 1)

	if err := os.WriteFile(path, []byte(result), 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	// Count changed lines
	oldLines := strings.Count(oldStr, "\n") + 1
	newLines := strings.Count(newStr, "\n") + 1

	return fmt.Sprintf("已成功编辑文件: %s (替换 %d 行 → %d 行)\n%s", path, oldLines, newLines, diff), nil
}

// generateDiff 生成unified diff预览
func generateDiff(path, content, oldStr, newStr string) string {
	lines := strings.Split(content, "\n")
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	// 找到oldStr在文件中的起始行号
	startLine := 0
	for i := 0; i <= len(lines)-len(oldLines); i++ {
		match := true
		for j := 0; j < len(oldLines); j++ {
			if lines[i+j] != oldLines[j] {
				match = false
				break
			}
		}
		if match {
			startLine = i
			break
		}
	}

	// 上下文行数
	contextLines := 2
	start := startLine - contextLines
	if start < 0 {
		start = 0
	}
	end := startLine + len(oldLines) + contextLines
	if end > len(lines) {
		end = len(lines)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- a/%s\n+++ b/%s\n", path, path))
	sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n",
		start+1, end-start, start+1, end-start+len(newLines)-len(oldLines)))

	// 上下文 + 删除 + 新增
	for i := start; i < startLine; i++ {
		sb.WriteString(fmt.Sprintf(" %s\n", lines[i]))
	}
	for _, line := range oldLines {
		sb.WriteString(fmt.Sprintf("-%s\n", line))
	}
	for _, line := range newLines {
		sb.WriteString(fmt.Sprintf("+%s\n", line))
	}
	afterEnd := startLine + len(oldLines)
	for i := afterEnd; i < end; i++ {
		sb.WriteString(fmt.Sprintf(" %s\n", lines[i]))
	}

	return sb.String()
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}
	return path
}

// isBinary checks if data appears to be binary
func isBinary(data []byte) bool {
	// Check first 8KB for null bytes
	checkLen := len(data)
	if checkLen > 8192 {
		checkLen = 8192
	}
	for i := 0; i < checkLen; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}
