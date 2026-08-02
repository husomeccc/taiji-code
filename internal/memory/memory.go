package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Memory manages persistent context files (equivalent to CLAUDE.md)
type Memory struct {
	// ProjectDir is the working directory
	ProjectDir string

	// MemoryFileName is the name of the memory file (e.g., TAIJI.md)
	MemoryFileName string
}

// New creates a memory manager
func New(projectDir, fileName string) *Memory {
	return &Memory{
		ProjectDir:     projectDir,
		MemoryFileName: fileName,
	}
}

// MemoryPath returns the full path to the memory file
func (m *Memory) MemoryPath() string {
	return filepath.Join(m.ProjectDir, m.MemoryFileName)
}

// Load reads the memory file content
func (m *Memory) Load() (string, error) {
	data, err := os.ReadFile(m.MemoryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// Save writes content to the memory file
func (m *Memory) Save(content string) error {
	dir := filepath.Dir(m.MemoryPath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建记忆目录失败: %w", err)
	}
	return os.WriteFile(m.MemoryPath(), []byte(content), 0644)
}

// Append adds content to the memory file
func (m *Memory) Append(content string) error {
	existing, err := m.Load()
	if err != nil {
		return err
	}

	if existing != "" {
		content = existing + "\n\n" + content
	}

	return m.Save(content)
}

// Exists checks if the memory file exists
func (m *Memory) Exists() bool {
	_, err := os.Stat(m.MemoryPath())
	return err == nil
}

// CreateDefault creates a default memory file if it doesn't exist
func (m *Memory) CreateDefault() error {
	if m.Exists() {
		return nil
	}

	defaultContent := fmt.Sprintf(`# %s

This file provides persistent context for the AI assistant across sessions.
It is automatically loaded at the start of each session.

## Project Info
- Created: %s
- Working Directory: %s

## Conventions
<!-- Add project-specific conventions, coding standards, etc. -->

## Architecture
<!-- Add high-level architecture notes here -->

## Notes
<!-- Add any other persistent context here -->
`, m.MemoryFileName, time.Now().Format("2006-01-02"), m.ProjectDir)

	return m.Save(defaultContent)
}

// BuildSystemPromptExtension reads the memory file and returns it as a system prompt addition
func (m *Memory) BuildSystemPromptExtension() string {
	content, err := m.Load()
	if err != nil || content == "" {
		return ""
	}

	return fmt.Sprintf(`
<project_context>
The following is loaded from %s in the project directory.
Treat this as authoritative project context.

%s
</project_context>`, m.MemoryFileName, content)
}

// SessionLog appends a session summary to the daily log
func (m *Memory) SessionLog(summary string) error {
	logDir := filepath.Join(m.ProjectDir, ".taiji", "sessions")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	date := time.Now().Format("2006-01-02")
	logPath := filepath.Join(logDir, date+".md")

	entry := fmt.Sprintf("\n## %s\n%s\n",
		time.Now().Format("15:04"),
		summary,
	)

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(entry)
	return err
}

// ListSessions returns recent session log entries
func (m *Memory) ListSessions(limit int) ([]string, error) {
	logDir := filepath.Join(m.ProjectDir, ".taiji", "sessions")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []string
	// Read in reverse order (most recent first)
	for i := len(entries) - 1; i >= 0 && len(sessions) < limit; i-- {
		if !entries[i].IsDir() && strings.HasSuffix(entries[i].Name(), ".md") {
			sessions = append(sessions, entries[i].Name())
		}
	}

	return sessions, nil
}
