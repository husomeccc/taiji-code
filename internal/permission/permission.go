package permission

import (
	"fmt"
	"strings"
	"taiji-code/types"
)

// Handler manages permission checks for tool operations
type Handler struct {
	// AutoApproveAll bypasses all permission checks (for testing)
	AutoApproveAll bool

	// AskFunc is called when user confirmation is needed
	// Returns true if user approves
	AskFunc func(toolName, description string) bool
}

// NewHandler creates a permission handler with default settings
func NewHandler() *Handler {
	return &Handler{
		AutoApproveAll: false,
	}
}

// Check determines if an operation is allowed
func (h *Handler) Check(toolName string, level types.PermissionLevel, description string) bool {
	if h.AutoApproveAll {
		return true
	}

	switch level {
	case types.PermAutoApprove:
		return true
	case types.PermDeny:
		return false
	case types.PermAskUser:
		if h.AskFunc != nil {
			return h.AskFunc(toolName, description)
		}
		return false
	default:
		return false
	}
}

// DangerousPatterns are bash command patterns that need confirmation
var DangerousPatterns = []string{
	"rm -rf",
	"rm -r",
	"del /s",
	"format",
	"mkfs",
	"dd if=",
	"> /dev/",
	"chmod -R 777",
	"curl.*| sh",
	"curl.*| bash",
	"wget.*| sh",
	"pip install",
	"npm install -g",
	"git push --force",
	"git reset --hard",
	"git clean -fd",
}

// IsDangerousCommand checks if a bash command matches dangerous patterns
func IsDangerousCommand(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, pattern := range DangerousPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// DescribeOperation returns a human-readable description of what a tool will do
func DescribeOperation(toolName string, args map[string]interface{}) string {
	switch toolName {
	case "read_file":
		path, _ := args["path"].(string)
		return fmt.Sprintf("读取文件: %s", path)
	case "write_file":
		path, _ := args["path"].(string)
		return fmt.Sprintf("写入文件: %s", path)
	case "edit_file":
		path, _ := args["path"].(string)
		return fmt.Sprintf("编辑文件: %s", path)
	case "bash":
		cmd, _ := args["command"].(string)
		if len(cmd) > 80 {
			cmd = cmd[:77] + "..."
		}
		return fmt.Sprintf("执行命令: %s", cmd)
	case "list_dir":
		path, _ := args["path"].(string)
		return fmt.Sprintf("列出目录: %s", path)
	case "grep_search":
		pattern, _ := args["pattern"].(string)
		return fmt.Sprintf("搜索内容: %s", pattern)
	case "glob_find":
		pattern, _ := args["pattern"].(string)
		return fmt.Sprintf("查找文件: %s", pattern)
	case "git":
		subcmd, _ := args["subcommand"].(string)
		return fmt.Sprintf("Git %s", subcmd)
	default:
		return fmt.Sprintf("执行工具: %s", toolName)
	}
}
