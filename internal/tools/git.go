package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"taiji-code/types"
)

// GitTool provides git operations
type GitTool struct {
	WorkDir string
}

func NewGitTool(workDir string) *GitTool {
	return &GitTool{WorkDir: workDir}
}

func (t *GitTool) Name() string        { return "git" }
func (t *GitTool) Description() string  { return "执行Git操作。支持的子命令: status, diff, log, show, add, commit, branch, checkout, stash。" }
func (t *GitTool) PermissionLevel() types.PermissionLevel { return types.PermAskUser }

func (t *GitTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"subcommand": map[string]interface{}{
				"type":        "string",
				"description": "Git子命令(status/diff/log/show/add/commit/branch/checkout/stash)",
			},
			"args": map[string]interface{}{
				"type":        "string",
				"description": "Git命令参数(可选)",
			},
		},
		"required": []interface{}{"subcommand"},
	}
}

func (t *GitTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	subcmd, _ := args["subcommand"].(string)
	if subcmd == "" {
		return "", fmt.Errorf("subcommand is required")
	}

	extraArgs, _ := args["args"].(string)

	// Build command
	cmdArgs := []string{subcmd}
	if extraArgs != "" {
		// Split args by space (simple split, doesn't handle quoted args)
		for _, arg := range strings.Fields(extraArgs) {
			cmdArgs = append(cmdArgs, arg)
		}
	}

	// Add helpful flags for certain subcommands
	switch subcmd {
	case "log":
		if extraArgs == "" {
			cmdArgs = append(cmdArgs, "--oneline", "-20", "--no-pager")
		}
	case "diff":
		if extraArgs == "" {
			cmdArgs = append(cmdArgs, "--no-pager")
		}
	case "status":
		cmdArgs = append(cmdArgs, "--short")
	}

	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Dir = t.WorkDir

	// Disable pager
	cmd.Env = append(cmd.Environ(), "GIT_PAGER=cat", "PAGER=cat")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if err != nil {
		if output == "" {
			return "", fmt.Errorf("git %s 失败: %w", subcmd, err)
		}
		return output, nil // Return output even with error (git sometimes writes to stderr)
	}

	if output == "" {
		return fmt.Sprintf("git %s 执行成功(无输出)", subcmd), nil
	}

	return output, nil
}

// NotebookEdit is a placeholder for notebook editing (Jupyter)
type NotebookEdit struct{}

func (t *NotebookEdit) Name() string        { return "notebook_edit" }
func (t *NotebookEdit) Description() string  { return "编辑Jupyter Notebook(.ipynb)中的单元格。可以替换、插入或删除单元格。" }
func (t *NotebookEdit) PermissionLevel() types.PermissionLevel { return types.PermAskUser }

func (t *NotebookEdit) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"notebook_path": map[string]interface{}{
				"type":        "string",
				"description": "Notebook文件路径",
			},
			"cell_id": map[string]interface{}{
				"type":        "string",
				"description": "单元格ID",
			},
			"new_source": map[string]interface{}{
				"type":        "string",
				"description": "新的单元格内容",
			},
			"edit_mode": map[string]interface{}{
				"type":        "string",
				"description": "编辑模式: replace/insert/delete",
				"enum":        []interface{}{"replace", "insert", "delete"},
			},
		},
		"required": []interface{}{"notebook_path", "new_source"},
	}
}

func (t *NotebookEdit) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	nbPath, _ := args["notebook_path"].(string)
	cellID, _ := args["cell_id"].(string)
	newSource, _ := args["new_source"].(string)
	editMode, _ := args["edit_mode"].(string)

	if nbPath == "" {
		return "", fmt.Errorf("notebook_path is required")
	}
	if editMode == "" {
		editMode = "replace"
	}

	// 读取notebook
	data, err := os.ReadFile(nbPath)
	if err != nil {
		return "", fmt.Errorf("读取notebook失败: %w", err)
	}

	var nb map[string]interface{}
	if err := json.Unmarshal(data, &nb); err != nil {
		return "", fmt.Errorf("解析notebook失败: %w", err)
	}

	cells, ok := nb["cells"].([]interface{})
	if !ok {
		return "", fmt.Errorf("notebook中没有cells")
	}

	switch editMode {
	case "replace":
		if cellID == "" {
			return "", fmt.Errorf("replace模式需要cell_id")
		}
		found := false
		for i, c := range cells {
			cell, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			id, _ := cell["id"].(string)
			if id == cellID {
				cell["source"] = []interface{}{newSource}
				cells[i] = cell
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("未找到cell: %s", cellID)
		}

	case "insert":
		newCell := map[string]interface{}{
			"cell_type":    "code",
			"source":       []interface{}{newSource},
			"metadata":     map[string]interface{}{},
			"execution_count": nil,
			"outputs":      []interface{}{},
		}
		if cellID != "" {
			// 在指定cell后插入
			for i, c := range cells {
				cell, ok := c.(map[string]interface{})
				if !ok {
					continue
				}
				id, _ := cell["id"].(string)
				if id == cellID {
					idx := i + 1
					if idx > len(cells) {
						idx = len(cells)
					}
					cells = append(cells[:idx], append([]interface{}{newCell}, cells[idx:]...)...)
					break
				}
			}
		} else {
			cells = append(cells, newCell)
		}

	case "delete":
		if cellID == "" {
			return "", fmt.Errorf("delete模式需要cell_id")
		}
		for i, c := range cells {
			cell, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			id, _ := cell["id"].(string)
			if id == cellID {
				cells = append(cells[:i], cells[i+1:]...)
				break
			}
		}
	}

	nb["cells"] = cells

	// 写回
	out, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		return "", fmt.Errorf("序列化notebook失败: %w", err)
	}
	if err := os.WriteFile(nbPath, out, 0644); err != nil {
		return "", fmt.Errorf("写入notebook失败: %w", err)
	}

	return fmt.Sprintf("Notebook已更新: %s (模式: %s, 共 %d 个cell)", nbPath, editMode, len(cells)), nil
}
