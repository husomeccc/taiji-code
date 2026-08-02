package agent

import (
	"context"
	"fmt"
	"sync"
	"taiji-code/internal/llm"
	"taiji-code/internal/permission"
	"taiji-code/internal/tools"
	"taiji-code/types"
	"time"
)

// SubAgentTask 子代理任务
type SubAgentTask struct {
	ID          string
	Description string
	Prompt      string
	Result      string
	Error       error
	Status      string // "pending", "running", "done", "failed"
}

// SubAgentPool 子代理池
type SubAgentPool struct {
	client   *llm.Client
	registry *tools.Registry
	perm     *permission.Handler
	workDir  string
	maxTokens int
	mu       sync.Mutex
	tasks    map[string]*SubAgentTask
}

// NewSubAgentPool 创建子代理池
func NewSubAgentPool(client *llm.Client, registry *tools.Registry, perm *permission.Handler, workDir string, maxTokens int) *SubAgentPool {
	return &SubAgentPool{
		client:    client,
		registry:  registry,
		perm:      perm,
		workDir:   workDir,
		maxTokens: maxTokens,
		tasks:     make(map[string]*SubAgentTask),
	}
}

// SpawnTask 派生子代理任务
func (p *SubAgentPool) SpawnTask(id, description, prompt string) *SubAgentTask {
	p.mu.Lock()
	defer p.mu.Unlock()

	task := &SubAgentTask{
		ID:          id,
		Description: description,
		Prompt:      prompt,
		Status:      "pending",
	}
	p.tasks[id] = task
	return task
}

// RunTask 运行单个子代理任务
func (p *SubAgentPool) RunTask(ctx context.Context, task *SubAgentTask) {
	p.mu.Lock()
	task.Status = "running"
	p.mu.Unlock()

	// 创建子代理（独立的对话历史）
	subAgent := &Agent{
		client:      p.client,
		registry:    p.registry,
		perm:        p.perm,
		workDir:     p.workDir,
		maxTokens:   p.maxTokens,
		maxSteps:    20, // 子代理限制步数
		retryCfg:    llm.DefaultRetryConfig(),
		sessionID:   GenerateID(),
		sessions:    NewSessionManager(p.workDir),
		toolCache:   tools.NewToolCache(30*time.Second, 100),
		mode:        ModeNormal,
		outputStyle: "normal",
		hookMgr:     NewHookManager(p.workDir),
		skillMgr:    NewSkillManager(p.workDir),
	}

	// 设置system prompt
	subAgent.messages = append(subAgent.messages, types.Message{
		Role: types.RoleSystem,
		Content: fmt.Sprintf(`你是太极Code的子代理，负责完成一项具体子任务。
任务描述: %s

规则:
- 专注于当前任务，不要偏离
- 完成后给出简洁的结果摘要
- 如果遇到问题，说明原因并建议解决方案`, task.Description),
	})

	// 添加用户提示
	subAgent.messages = append(subAgent.messages, types.Message{
		Role:    types.RoleUser,
		Content: task.Prompt,
	})

	// 运行
	result, err := subAgent.runLoop(ctx)

	p.mu.Lock()
	task.Result = result
	task.Error = err
	if err != nil {
		task.Status = "failed"
	} else {
		task.Status = "done"
	}
	p.mu.Unlock()
}

// RunParallel 并行运行多个子任务
func (p *SubAgentPool) RunParallel(ctx context.Context, tasks []*SubAgentTask) {
	var wg sync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		go func(t *SubAgentTask) {
			defer wg.Done()
			p.RunTask(ctx, t)
		}(task)
	}
	wg.Wait()
}

// GetTask 获取任务状态
func (p *SubAgentPool) GetTask(id string) (*SubAgentTask, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	task, ok := p.tasks[id]
	return task, ok
}

// GetAllTasks 获取所有任务
func (p *SubAgentPool) GetAllTasks() []*SubAgentTask {
	p.mu.Lock()
	defer p.mu.Unlock()
	var tasks []*SubAgentTask
	for _, t := range p.tasks {
		tasks = append(tasks, t)
	}
	return tasks
}

// FormatTaskResults 格式化任务结果
func FormatTaskResults(tasks []*SubAgentTask) string {
	var result string
	for _, t := range tasks {
		status := "✓"
		if t.Status == "failed" {
			status = "✗"
		}
		result += fmt.Sprintf("%s [%s] %s\n", status, t.ID, t.Description)
		if t.Result != "" {
			summary := t.Result
			if len(summary) > 200 {
				summary = summary[:200] + "..."
			}
			result += fmt.Sprintf("  结果: %s\n", summary)
		}
		if t.Error != nil {
			result += fmt.Sprintf("  错误: %v\n", t.Error)
		}
	}
	return result
}

// SubAgentTool 子代理工具（让LLM可以派生子任务）
type SubAgentTool struct {
	Pool *SubAgentPool
}

func (t *SubAgentTool) Name() string        { return "sub_agent" }
func (t *SubAgentTool) Description() string  { return "派生子代理执行独立子任务。适合并行处理多个独立的代码分析、文件搜索等任务。子代理有自己的工具集和对话上下文。" }
func (t *SubAgentTool) PermissionLevel() types.PermissionLevel { return types.PermAskUser }

func (t *SubAgentTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task_id": map[string]interface{}{
				"type":        "string",
				"description": "任务唯一标识",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "任务简述",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "详细的任务指令",
			},
		},
		"required": []interface{}{"task_id", "description", "prompt"},
	}
}

func (t *SubAgentTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	taskID, _ := args["task_id"].(string)
	description, _ := args["description"].(string)
	prompt, _ := args["prompt"].(string)

	if taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	if description == "" {
		return "", fmt.Errorf("description is required")
	}
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}

	task := t.Pool.SpawnTask(taskID, description, prompt)
	t.Pool.RunTask(ctx, task)

	if task.Error != nil {
		return fmt.Sprintf("子任务失败: %v\n部分结果: %s", task.Error, task.Result), task.Error
	}

	return task.Result, nil
}
