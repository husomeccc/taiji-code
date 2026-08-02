package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"taiji-code/types"
	"time"
)

// Bash executes shell commands
type Bash struct {
	WorkDir string
	Timeout time.Duration
}

func NewBash(workDir string) *Bash {
	return &Bash{
		WorkDir: workDir,
		Timeout: 120 * time.Second,
	}
}

func (t *Bash) Name() string        { return "bash" }
func (t *Bash) Description() string  { return "在本地Shell中执行命令。支持所有bash/cmd命令。命令在工作目录中执行。超时限制120秒。" }
func (t *Bash) PermissionLevel() types.PermissionLevel { return types.PermAskUser }

func (t *Bash) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "要执行的Shell命令",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "超时秒数(可选,默认120)",
			},
		},
		"required": []interface{}{"command"},
	}
}

func (t *Bash) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	cmdStr, _ := args["command"].(string)
	if cmdStr == "" {
		return "", fmt.Errorf("command is required")
	}

	timeout := t.Timeout
	if v, ok := args["timeout"].(float64); ok && v > 0 {
		timeout = time.Duration(v) * time.Second
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-c", cmdStr)
	}

	if t.WorkDir != "" {
		cmd.Dir = t.WorkDir
	}

	// Set environment
	cmd.Env = append(cmd.Environ(), "TERM=dumb")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var result strings.Builder

	if stdout.Len() > 0 {
		result.WriteString(stdout.String())
	}

	if stderr.Len() > 0 {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString("[STDERR]\n")
		result.WriteString(stderr.String())
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return result.String() + "\n[命令超时]", fmt.Errorf("command timed out after %v", timeout)
		}

		if result.Len() == 0 {
			return "", fmt.Errorf("命令执行失败: %w", err)
		}

		// Include stderr output but note the error
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			result.WriteString(fmt.Sprintf("\n[退出码: %d]", exitErr.ExitCode()))
		}
	}

	output := result.String()
	if output == "" {
		output = "[命令执行成功,无输出]"
	}

	// Truncate very long output
	const maxOutput = 50000
	runes := []rune(output)
	if len(runes) > maxOutput {
		output = string(runes[:maxOutput]) + fmt.Sprintf("\n\n[输出被截断,共 %d 字符]", len(runes))
	}

	return output, nil
}
