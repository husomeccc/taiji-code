package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"taiji-code/types"
	"time"
	"unicode/utf8"
)

// WebFetch 抓取网页内容并提取文本
type WebFetch struct{}

func (t *WebFetch) Name() string       { return "web_fetch" }
func (t *WebFetch) Description() string { return "抓取指定URL的网页内容，提取纯文本。用于查阅文档、API参考等。" }
func (t *WebFetch) PermissionLevel() types.PermissionLevel {
	return types.PermAutoApprove
}

func (t *WebFetch) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "要抓取的URL地址",
			},
		},
		"required": []interface{}{"url"},
	}
}

func (t *WebFetch) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	url, _ := args["url"].(string)
	if url == "" {
		return "", fmt.Errorf("url is required")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "TaijiCode/0.2.0 (CLI AI Assistant)")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// 限制读取大小
	body, err := io.ReadAll(io.LimitReader(resp.Body, 200*1024))
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	content := string(body)

	// 简单HTML清理
	content = stripHTML(content)

	// 截断
	const maxLen = 50000
	if len(content) > maxLen {
		content = content[:maxLen] + "\n\n[内容已截断]"
	}

	if strings.TrimSpace(content) == "" {
		return "[页面内容为空或无法提取文本]", nil
	}

	return content, nil
}

// stripHTML 简单清理HTML标签
func stripHTML(s string) string {
	var sb strings.Builder
	inTag := false
	inScript := false
	inStyle := false
	prevWasSpace := false

	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]

		if r == '<' {
			inTag = true
			// 检测script/style
			lower := strings.ToLower(s[:minInt(10, len(s))])
			if strings.HasPrefix(lower, "script") {
				inScript = true
			}
			if strings.HasPrefix(lower, "/script") {
				inScript = false
			}
			if strings.HasPrefix(lower, "style") {
				inStyle = true
			}
			if strings.HasPrefix(lower, "/style") {
				inStyle = false
			}
			continue
		}
		if r == '>' {
			inTag = false
			// 标签后加换行
			if !prevWasSpace {
				sb.WriteRune('\n')
				prevWasSpace = true
			}
			continue
		}
		if inTag || inScript || inStyle {
			continue
		}

		// 合并空白
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevWasSpace {
				sb.WriteRune(' ')
				prevWasSpace = true
			}
			continue
		}

		prevWasSpace = false
		sb.WriteRune(r)
	}

	return strings.TrimSpace(sb.String())
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// WebSearch 使用DuckDuckGo搜索网页（无需API Key）
type WebSearch struct{}

func (t *WebSearch) Name() string       { return "web_search" }
func (t *WebSearch) Description() string { return "搜索互联网获取信息。使用DuckDuckGo搜索引擎，返回相关网页标题、链接和摘要。适合查找文档、解决方案、最新资讯等。" }
func (t *WebSearch) PermissionLevel() types.PermissionLevel {
	return types.PermAutoApprove
}

func (t *WebSearch) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "搜索关键词",
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "最大结果数(默认8,最多15)",
			},
		},
		"required": []interface{}{"query"},
	}
}

func (t *WebSearch) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	maxResults := 8
	if v, ok := args["max_results"].(float64); ok && int(v) > 0 {
		maxResults = int(v)
		if maxResults > 15 {
			maxResults = 15
		}
	}

	// 使用DuckDuckGo HTML版本搜索
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", urlEncode(query))

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建搜索请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("搜索请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("搜索HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
	if err != nil {
		return "", fmt.Errorf("读取搜索结果失败: %w", err)
	}

	html := string(body)
	results := parseDDGResults(html, maxResults)

	if len(results) == 0 {
		return fmt.Sprintf("未找到与 \"%s\" 相关的搜索结果", query), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("搜索: %s\n找到 %d 条结果:\n\n", query, len(results)))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   链接: %s\n", i+1, r.Title, r.URL))
		if r.Snippet != "" {
			sb.WriteString(fmt.Sprintf("   摘要: %s\n", r.Snippet))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

// parseDDGResults 解析DuckDuckGo HTML搜索结果
func parseDDGResults(html string, maxResults int) []searchResult {
	var results []searchResult

	// DuckDuckGo HTML版结果在 <a class="result__a" href="...">标题</a> 中
	// 摘要在 <a class="result__snippet">...</a> 中
	// 链接在 result__url 中

	// 简单解析：查找 result__a 链接
	remaining := html
	for len(results) < maxResults {
		// 找结果标题链接
		titleIdx := strings.Index(remaining, `class="result__a"`)
		if titleIdx == -1 {
			// 也尝试 result__url 作为备选
			titleIdx = strings.Index(remaining, `class="result__url"`)
			if titleIdx == -1 {
				break
			}
		}

		// 提取href
		hrefStart := strings.LastIndex(remaining[:titleIdx], `href="`)
		if hrefStart == -1 {
			remaining = remaining[titleIdx+20:]
			continue
		}
		hrefStart += 6
		hrefEnd := strings.Index(remaining[hrefStart:], `"`)
		if hrefEnd == -1 {
			break
		}
		href := remaining[hrefStart : hrefStart+hrefEnd]

		// 提取标题文本
		tagEnd := strings.Index(remaining[titleIdx:], ">")
		if tagEnd == -1 {
			break
		}
		tagEnd += titleIdx + 1
		closeTag := strings.Index(remaining[tagEnd:], "</a>")
		if closeTag == -1 {
			break
		}
		title := stripHTML(remaining[tagEnd : tagEnd+closeTag])
		title = strings.TrimSpace(title)

		// DuckDuckGo的href是重定向URL，提取真实URL
		realURL := href
		if strings.Contains(href, "uddg=") {
			// 从 uddg= 参数提取真实URL
			idx := strings.Index(href, "uddg=")
			if idx != -1 {
				encoded := href[idx+5:]
				if end := strings.Index(encoded, "&"); end != -1 {
					encoded = encoded[:end]
				}
				if decoded := urlDecode(encoded); decoded != "" {
					realURL = decoded
				}
			}
		}

		// 查找摘要
		snippet := ""
		snippetArea := remaining[tagEnd:]
		snipIdx := strings.Index(snippetArea, `class="result__snippet"`)
		if snipIdx != -1 && snipIdx < 500 {
			snipStart := snipIdx + 22
			snipTagEnd := strings.Index(snippetArea[snipStart:], ">")
			if snipTagEnd != -1 {
				snipTagEnd += snipStart + 1
				snipClose := strings.Index(snippetArea[snipTagEnd:], "</a>")
				if snipClose != -1 {
					snippet = stripHTML(snippetArea[snipTagEnd : snipTagEnd+snipClose])
					snippet = strings.TrimSpace(snippet)
				}
			}
		}

		if title != "" && realURL != "" {
			results = append(results, searchResult{
				Title:   title,
				URL:     realURL,
				Snippet: snippet,
			})
		}

		// 继续搜索
		nextStart := tagEnd + 10
		if nextStart >= len(remaining) {
			break
		}
		remaining = remaining[nextStart:]
	}

	return results
}

// urlEncode 简单URL编码（正确处理UTF-8多字节字符）
func urlEncode(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '-' || r == '_' || r == '.' || r == '~':
			sb.WriteRune(r)
		case r == ' ':
			sb.WriteString("%20")
		default:
			// 将rune编码为UTF-8字节，然后逐字节percent-encode
			buf := make([]byte, 4)
			n := 0
			if r < 0x80 {
				buf[0] = byte(r)
				n = 1
			} else if r < 0x800 {
				buf[0] = byte(0xC0 | (r >> 6))
				buf[1] = byte(0x80 | (r & 0x3F))
				n = 2
			} else if r < 0x10000 {
				buf[0] = byte(0xE0 | (r >> 12))
				buf[1] = byte(0x80 | ((r >> 6) & 0x3F))
				buf[2] = byte(0x80 | (r & 0x3F))
				n = 3
			} else {
				buf[0] = byte(0xF0 | (r >> 18))
				buf[1] = byte(0x80 | ((r >> 12) & 0x3F))
				buf[2] = byte(0x80 | ((r >> 6) & 0x3F))
				buf[3] = byte(0x80 | (r & 0x3F))
				n = 4
			}
			for i := 0; i < n; i++ {
				sb.WriteString(fmt.Sprintf("%%%02X", buf[i]))
			}
		}
	}
	return sb.String()
}

// urlDecode 简单URL解码
func urlDecode(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			var b byte
			fmt.Sscanf(s[i+1:i+3], "%02x", &b)
			sb.WriteByte(b)
			i += 2
		} else if s[i] == '+' {
			sb.WriteByte(' ')
		} else {
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}
