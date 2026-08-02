package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PermissionMode 权限模式
type PermissionMode string

const (
	PermModeDefault   PermissionMode = "default"    // 逐条确认
	PermModeAutoEdit  PermissionMode = "auto_edit"  // 自动批准编辑
	PermModePlanOnly  PermissionMode = "plan_only"  // 仅规划(只读)
	PermModeBypass    PermissionMode = "bypass"     // 跳过所有权限
)

// Config holds all configuration for taiji-code
type Config struct {
	// DeepSeek API settings
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`

	// Behavior settings
	MaxTokens       int     `json:"max_tokens"`
	Temperature     float64 `json:"temperature"`
	AutoCompact     int     `json:"auto_compact"` // Token threshold for auto-compact
	MaxToolSteps    int     `json:"max_tool_steps"` // Max consecutive tool calls

	// Display settings
	Theme        string `json:"theme"`
	OutputFormat string `json:"output_format"`  // "text", "json", "stream-json"
	ResponseStyle string `json:"response_style"` // "concise", "normal", "detailed"

	// Permission
	PermissionMode PermissionMode `json:"permission_mode"`

	// Memory file name
	MemoryFile string `json:"memory_file"`

	// MCP servers
	McpServers []McpServerConfig `json:"mcp_servers,omitempty"`
}

// McpServerConfig MCP服务器配置
type McpServerConfig struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Env     []string `json:"env,omitempty"`
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		APIKey:         "",
		BaseURL:        "https://api.deepseek.com/v1",
		Model:          "deepseek-chat",
		MaxTokens:      8192,
		Temperature:    0.7,
		AutoCompact:    100000,
		MaxToolSteps:   50,
		Theme:          "dark",
		OutputFormat:   "text",
		ResponseStyle:  "normal",
		PermissionMode: PermModeDefault,
		MemoryFile:     "TAIJI.md",
	}
}

// ConfigDir returns the config directory path
func ConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".taiji-code")
}

// Load reads config from file, merging with defaults
func Load() *Config {
	cfg := DefaultConfig()

	// Check environment variable first
	if key := os.Getenv("DEEPSEEK_API_KEY"); key != "" {
		cfg.APIKey = key
	}

	configPath := filepath.Join(ConfigDir(), "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 配置文件格式错误，使用默认配置: %v\n", err)
	}

	// Environment variable always wins
	if key := os.Getenv("DEEPSEEK_API_KEY"); key != "" {
		cfg.APIKey = key
	}

	return cfg
}

// Save writes config to file
func (c *Config) Save() error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0600)
}

// HasAPIKey returns whether an API key is configured
func (c *Config) HasAPIKey() bool {
	return c.APIKey != ""
}
