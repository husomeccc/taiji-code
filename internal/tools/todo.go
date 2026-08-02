package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"taiji-code/types"
)

// ─── 任务管理 ────────────────────────────────────────────────────────────

// TodoItem 单个任务
type TodoItem struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"` // "pending", "in_progress", "completed", "cancelled"
}

// TodoStore 任务存储（全局单例）
type TodoStore struct {
	mu    sync.Mutex
	items []TodoItem
}

// NewTodoStore 创建任务存储
func NewTodoStore() *TodoStore {
	return &TodoStore{}
}

// SetTasks 设置任务列表（整体替换）
func (s *TodoStore) SetTasks(items []TodoItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = items
}

// GetTasks 获取所有任务
func (s *TodoStore) GetTasks() []TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]TodoItem, len(s.items))
	copy(result, s.items)
	return result
}

// FormatTasks 格式化任务列表
func (s *TodoStore) FormatTasks() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.items) == 0 {
		return "当前没有任务"
	}

	var sb strings.Builder
	for _, item := range s.items {
		icon := "○"
		switch item.Status {
		case "in_progress":
			icon = "◉"
		case "completed":
			icon = "✓"
		case "cancelled":
			icon = "✗"
		}
		sb.WriteString(fmt.Sprintf("  %s [%s] %s\n", icon, item.Status, item.Description))
	}
	return sb.String()
}

// ─── TodoWrite 工具 ──────────────────────────────────────────────────────

// TodoWrite 任务管理工具
type TodoWrite struct {
	Store *TodoStore
}

func (t *TodoWrite) Name() string       { return "todo_write" }
func (t *TodoWrite) Description() string {
	return "管理任务清单。用于跟踪复杂多步骤任务的进度。每次调用会替换整个任务列表。"
}
func (t *TodoWrite) PermissionLevel() types.PermissionLevel {
	return types.PermAutoApprove
}

func (t *TodoWrite) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"todos": map[string]interface{}{
				"type":        "array",
				"description": "任务列表，每个任务包含description和status",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"description": map[string]interface{}{
							"type":        "string",
							"description": "任务描述",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "任务状态: pending/in_progress/completed/cancelled",
							"enum":        []interface{}{"pending", "in_progress", "completed", "cancelled"},
						},
					},
					"required": []interface{}{"description", "status"},
				},
			},
		},
		"required": []interface{}{"todos"},
	}
}

func (t *TodoWrite) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	todosRaw, ok := args["todos"].([]interface{})
	if !ok {
		return "", fmt.Errorf("todos 必须是数组")
	}

	var items []TodoItem
	for i, raw := range todosRaw {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		desc, _ := m["description"].(string)
		status, _ := m["status"].(string)
		if desc == "" {
			continue
		}
		if status == "" {
			status = "pending"
		}
		items = append(items, TodoItem{
			ID:          fmt.Sprintf("%d", i+1),
			Description: desc,
			Status:      status,
		})
	}

	t.Store.SetTasks(items)
	return fmt.Sprintf("任务列表已更新 (%d 项任务)\n%s", len(items), t.Store.FormatTasks()), nil
}
