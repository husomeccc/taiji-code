package ui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// renderMarkdown 使用 glamour 渲染 Markdown
func renderMarkdown(text string, width int) string {
	if width <= 0 {
		width = 80
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return text
	}

	rendered, err := r.Render(text)
	if err != nil {
		return text
	}

	return strings.TrimRight(rendered, "\n")
}
