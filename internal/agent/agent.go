package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"taiji-code/internal/llm"
	"taiji-code/internal/memory"
	"taiji-code/internal/permission"
	"taiji-code/internal/tools"
	"taiji-code/types"
	"time"
)

// CostTracker tracks token usage and estimated costs
type CostTracker struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	Requests     int
	ToolCalls    int
}

// EstimateCost returns estimated cost in CNY (DeepSeek pricing)
// deepseek-chat: Input ¥1/M tokens, Output ¥2/M tokens
func (ct CostTracker) EstimateCost() float64 {
	inputCost := float64(ct.InputTokens) * 1.0 / 1_000_000
	outputCost := float64(ct.OutputTokens) * 2.0 / 1_000_000
	return inputCost + outputCost
}

// FormatCost returns a formatted cost string
func (ct CostTracker) FormatCost() string {
	cost := ct.EstimateCost()
	if cost < 0.01 {
		return fmt.Sprintf("¥%.4f", cost)
	}
	return fmt.Sprintf("¥%.2f", cost)
}

// FormatUsage returns a formatted usage summary
func (ct CostTracker) FormatUsage() string {
	return fmt.Sprintf("Token: %d 输入 / %d 输出 / %d 总计 | 请求: %d | 工具调用: %d | 费用: %s",
		ct.InputTokens, ct.OutputTokens, ct.TotalTokens,
		ct.Requests, ct.ToolCalls, ct.FormatCost())
}

// AgentMode 代理运行模式
type AgentMode string

const (
	ModeNormal  AgentMode = "normal"   // 正常模式
	ModePlan    AgentMode = "plan"     // 规划模式(只读)
	ModeAutoEdit AgentMode = "auto_edit" // 自动编辑模式
)

// Agent is the core conversation loop
type Agent struct {
	client     *llm.Client
	registry   *tools.Registry
	perm       *permission.Handler
	mem        *memory.Memory
	sessions   *SessionManager
	messages   []types.Message
	workDir    string
	maxTokens  int
	maxSteps   int
	cost       CostTracker
	retryCfg   llm.RetryConfig

	// 项目信息
	project    *ProjectInfo
	autoCompact int // token阈值，超过自动压缩

	// 工具缓存
	toolCache  *tools.ToolCache

	// Current session
	sessionID string

	// 运行模式
	mode        AgentMode
	outputStyle string // "concise", "normal", "detailed"

	// Hook和Skill管理
	hookMgr  *HookManager
	skillMgr *SkillManager

	// 任务管理
	todoStore *tools.TodoStore

	// 消息历史快照(用于rewind)
	messageSnapshots []messageSnapshot

	// Callbacks for UI
	OnToolCall    func(name string, args map[string]interface{})
	OnToolResult  func(name string, result string, isError bool)
	OnContent     func(text string)
	OnPermission  func(toolName, description string) bool
	OnUsage       func(cost CostTracker)
	OnCompact     func() // 自动压缩通知
}

// messageSnapshot 消息快照（用于rewind）
type messageSnapshot struct {
	Timestamp time.Time
	MsgCount  int
	Label     string
}

// New creates a new Agent
func New(client *llm.Client, registry *tools.Registry, perm *permission.Handler, mem *memory.Memory, workDir string, maxTokens, maxSteps int) *Agent {
	todoStore := tools.NewTodoStore()
	ag := &Agent{
		client:      client,
		registry:    registry,
		perm:        perm,
		mem:         mem,
		sessions:    NewSessionManager(workDir),
		workDir:     workDir,
		maxTokens:   maxTokens,
		maxSteps:    maxSteps,
		retryCfg:    llm.DefaultRetryConfig(),
		sessionID:   GenerateID(),
		autoCompact: 100000,
		toolCache:   tools.NewToolCache(30*time.Second, 100),
		mode:        ModeNormal,
		outputStyle: "normal",
		hookMgr:     NewHookManager(workDir),
		skillMgr:    NewSkillManager(workDir),
		todoStore:   todoStore,
	}

	// 自动检测项目类型
	ag.project = DetectProject(workDir)

	// 加载Hooks配置
	hooksPath := filepath.Join(workDir, ".taiji", "hooks.json")
	_ = ag.hookMgr.LoadHooks(hooksPath)

	// 加载技能
	_ = ag.skillMgr.LoadSkills()

	return ag
}

// SystemPrompt returns the system prompt
func (a *Agent) SystemPrompt() string {
	prompt := `你是太极Code，一个强大的AI编程助手，运行在用户的终端中。你帮助用户完成编程、文件操作、代码分析等各种任务。

## 核心行为
- **直接行动**：不要说"我来帮你..."，直接做
- **先读后改**：修改文件前必须先读取理解上下文
- **精确编辑**：优先用 edit_file 做精确替换，而非 write_file 重写整个文件
- **验证结果**：执行命令后检查输出，确认成功
- **简洁回复**：回答简短准确，不啰嗦

## 工具使用策略
- read_file: 读取文件。大文件用 offset/limit 分段读
- write_file: 创建新文件或完全重写（谨慎使用）
- edit_file: 精确替换文本。old_string 必须唯一匹配，包含前后 2-3 行上下文
- bash: 执行命令。支持所有 shell 命令
- list_dir: 列出目录
- glob_find: 按模式查找文件（支持 ** 递归）
- grep_search: 正则搜索文件内容
- git: Git 操作（status/diff/log/add/commit 等）

## 工作流
1. 理解任务 → 2. 读取相关文件 → 3. 规划修改 → 4. 精确编辑 → 5. 验证结果
- 创建项目时先规划目录结构，再逐个创建文件
- 修复 bug 时先复现，再定位，再修复
- 重构时确保不破坏现有功能

## 输出规范
- 代码块用三个反引号加语言名标记
- 文件路径用反引号包裹
- 重要信息用 **加粗**`

	// 添加输出风格指令
	switch a.outputStyle {
	case "concise":
		prompt += "\n\n## 输出风格\n- 极度简洁，不解释显而易见的部分\n- 代码优先，文字最少\n- 跳过礼貌用语"
	case "detailed":
		prompt += "\n\n## 输出风格\n- 详细解释每一步操作的原因\n- 提供替代方案和注意事项\n- 适合教学场景"
	}

	// 规划模式限制
	if a.mode == ModePlan {
		prompt += "\n\n## 规划模式(只读)\n- 你处于规划模式，只能读取和分析，不能修改文件\n- 提供详细的实施方案，但不要执行任何写操作\n- 不要调用 write_file, edit_file, bash(写操作) 等修改性工具"
	}

	// Add memory context
	if a.mem != nil {
		memExt := a.mem.BuildSystemPromptExtension()
		if memExt != "" {
			prompt += "\n\n" + memExt
		}
	}

	// Add project context
	if a.project != nil {
		projExt := a.project.BuildContextExtension()
		if projExt != "" {
			prompt += "\n\n" + projExt
		}
	}

	// Add skills context
	if a.skillMgr != nil {
		skillsExt := a.skillMgr.AllSkillContent()
		if skillsExt != "" {
			prompt += "\n\n" + skillsExt
		}
	}

	return prompt
}

// Chat processes a user message and returns the assistant's response
func (a *Agent) Chat(ctx context.Context, userMessage string) (string, error) {
	// Initialize messages with system prompt if empty
	if len(a.messages) == 0 {
		a.messages = append(a.messages, types.Message{
			Role:    types.RoleSystem,
			Content: a.SystemPrompt(),
		})
	}

	// Add user message
	a.messages = append(a.messages, types.Message{
		Role:    types.RoleUser,
		Content: userMessage,
	})

	// Run the agent loop
	return a.runLoop(ctx)
}

// runLoop is the core ReAct loop
func (a *Agent) runLoop(ctx context.Context) (string, error) {
	// 自动上下文压缩：检测token是否超过阈值
	if a.autoCompact > 0 {
		tokens := a.EstimateContextTokens()
		if tokens > a.autoCompact {
			a.Compact()
			if a.OnCompact != nil {
				a.OnCompact()
			}
		}
	}

	toolDefs := a.registry.Definitions()

	for step := 0; step < a.maxSteps; step++ {
		var assistantMsg *types.Message
		var err error

		if a.OnContent != nil {
			// Streaming mode with retry
			assistantMsg, err = a.client.AccumulateStreamWithRetry(
				ctx, a.messages, toolDefs, a.maxTokens, a.OnContent, a.retryCfg,
			)
			// 流式模式不返回usage，根据内容估算token
			if err == nil && assistantMsg != nil {
				inputEst := estimateTokens(systemPromptLen(a.messages))
				outputEst := estimateTokens(len(assistantMsg.Content))
				a.cost.InputTokens += inputEst
				a.cost.OutputTokens += outputEst
				a.cost.TotalTokens += inputEst + outputEst
			}
		} else {
			// Non-streaming with retry
			resp, e := a.client.ChatWithRetry(ctx, a.messages, toolDefs, a.maxTokens, a.retryCfg)
			if e != nil {
				return "", fmt.Errorf("LLM请求失败: %w", e)
			}
			if len(resp.Choices) > 0 {
				assistantMsg = &resp.Choices[0].Message
				a.cost.InputTokens += resp.Usage.PromptTokens
				a.cost.OutputTokens += resp.Usage.CompletionTokens
				a.cost.TotalTokens += resp.Usage.TotalTokens
			}
		}

		if err != nil {
			return "", fmt.Errorf("LLM请求失败: %w", err)
		}

		if assistantMsg == nil {
			return "", fmt.Errorf("LLM返回空响应（无choices）")
		}

		a.cost.Requests++

		// Add assistant message to history
		a.messages = append(a.messages, *assistantMsg)

		// If no tool calls, we're done
		if len(assistantMsg.ToolCalls) == 0 {
			// Auto-save session
			a.autoSave()
			if a.OnUsage != nil {
				a.OnUsage(a.cost)
			}
			return assistantMsg.Content, nil
		}

		// Execute tool calls
		for _, tc := range assistantMsg.ToolCalls {
			// 检查context是否已取消
			if ctx.Err() != nil {
				return "", fmt.Errorf("操作已取消")
			}

			a.cost.ToolCalls++
			result := a.executeToolCall(ctx, tc)

			a.messages = append(a.messages, types.Message{
				Role:       types.RoleTool,
				Content:    result.Content,
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
			})
		}
	}

	return "", fmt.Errorf("达到最大工具调用步数限制 (%d)", a.maxSteps)
}

// executeToolCall runs a single tool call
func (a *Agent) executeToolCall(ctx context.Context, tc types.ToolCall) types.ToolResult {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		result := types.ToolResult{
			ToolCallID: tc.ID,
			Name:       tc.Function.Name,
			Content:    fmt.Sprintf("参数解析失败: %v\n原始参数: %s", err, tc.Function.Arguments),
			IsError:    true,
		}
		if a.OnToolResult != nil {
			a.OnToolResult(tc.Function.Name, result.Content, true)
		}
		return result
	}

	if a.OnToolCall != nil {
		a.OnToolCall(tc.Function.Name, args)
	}

	// 规划模式：阻止写操作
	if a.mode == ModePlan {
		writeTools := map[string]bool{
			"write_file": true, "edit_file": true, "notebook_edit": true,
		}
		isWrite := writeTools[tc.Function.Name]
		// bash写操作检测
		if tc.Function.Name == "bash" {
			if cmd, ok := args["command"].(string); ok {
				for _, kw := range []string{"rm ", "mv ", "cp ", ">", "tee ", "chmod ", "chown "} {
					if strings.Contains(cmd, kw) {
						isWrite = true
						break
					}
				}
			}
		}
		if isWrite {
			result := types.ToolResult{
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    "当前处于规划模式(只读)，不允许执行写操作。请提供方案后由用户确认切换模式。",
				IsError:    true,
			}
			if a.OnToolResult != nil {
				a.OnToolResult(tc.Function.Name, result.Content, true)
			}
			return result
		}
	}

	tool, ok := a.registry.Get(tc.Function.Name)
	if !ok {
		result := types.ToolResult{
			ToolCallID: tc.ID,
			Name:       tc.Function.Name,
			Content:    fmt.Sprintf("未知工具: %s", tc.Function.Name),
			IsError:    true,
		}
		if a.OnToolResult != nil {
			a.OnToolResult(tc.Function.Name, result.Content, true)
		}
		return result
	}

	// Permission check
	desc := permission.DescribeOperation(tc.Function.Name, args)
	permLevel := tool.PermissionLevel()

	// 自动编辑模式：自动批准文件操作
	if a.mode == ModeAutoEdit {
		editTools := map[string]bool{
			"write_file": true, "edit_file": true, "notebook_edit": true,
		}
		if editTools[tc.Function.Name] {
			permLevel = types.PermAutoApprove
		}
	}

	// Extra safety for bash
	if tc.Function.Name == "bash" {
		if cmd, ok := args["command"].(string); ok && permission.IsDangerousCommand(cmd) {
			permLevel = types.PermAskUser
		}
	}

	if !a.perm.Check(tc.Function.Name, permLevel, desc) {
		result := types.ToolResult{
			ToolCallID: tc.ID,
			Name:       tc.Function.Name,
			Content:    "用户拒绝了此操作",
			IsError:    true,
		}
		if a.OnToolResult != nil {
			a.OnToolResult(tc.Function.Name, result.Content, true)
		}
		return result
	}

	// PreToolUse Hook
	if a.hookMgr != nil && a.hookMgr.HasHooks(HookPreToolUse) {
		results := a.hookMgr.Fire(ctx, HookPreToolUse, map[string]interface{}{
			"tool_name": tc.Function.Name,
			"arguments": string(tc.Function.Arguments),
		})
		for _, r := range results {
			if r.Blocked {
				result := types.ToolResult{
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
					Content:    "Hook阻止了此操作: " + r.Message,
					IsError:    true,
				}
				if a.OnToolResult != nil {
					a.OnToolResult(tc.Function.Name, result.Content, true)
				}
				return result
			}
		}
	}

	// Execute with cache for read operations
	var content string
	var execErr error
	cacheKey := tools.CacheKey(tc.Function.Name, args)
	if tools.ShouldCache(tc.Function.Name) {
		if entry, ok := a.toolCache.Get(cacheKey); ok {
			content = entry.Result
			execErr = nil
		} else {
			content, execErr = tool.Execute(ctx, args)
			if execErr == nil {
				a.toolCache.Set(cacheKey, content, false)
			}
		}
	} else {
		content, execErr = tool.Execute(ctx, args)
	}

	if execErr != nil {
		content = fmt.Sprintf("%s\n[错误: %v]", content, execErr)
	}

	result := types.ToolResult{
		ToolCallID: tc.ID,
		Name:       tc.Function.Name,
		Content:    content,
		IsError:    execErr != nil,
	}

	if a.OnToolResult != nil {
		a.OnToolResult(tc.Function.Name, content, execErr != nil)
	}

	// PostToolUse Hook
	if a.hookMgr != nil && a.hookMgr.HasHooks(HookPostToolUse) {
		a.hookMgr.Fire(ctx, HookPostToolUse, map[string]interface{}{
			"tool_name": tc.Function.Name,
			"result":    content,
			"is_error":  execErr != nil,
		})
	}

	return result
}

// ClearHistory resets the conversation
func (a *Agent) ClearHistory() {
	a.messages = nil
	a.sessionID = GenerateID()
}

// Compact summarizes old messages to reduce context size
func (a *Agent) Compact() string {
	if len(a.messages) <= 4 {
		return "对话已经很短，无需压缩"
	}

	system := a.messages[0]

	// 找到安全的分割点：不在tool call和tool result之间切割
	// 从倒数第6条开始找安全点
	cutoff := len(a.messages) - 6
	if cutoff < 1 {
		cutoff = 1
	}
	// 向前扫描，确保不在tool call和tool result之间切割
	for cutoff < len(a.messages)-1 {
		msg := a.messages[cutoff]
		// 如果当前是tool结果，检查前一条是否是assistant的tool call
		if msg.Role == types.RoleTool && cutoff > 0 {
			prev := a.messages[cutoff-1]
			if prev.Role == types.RoleAssistant && len(prev.ToolCalls) > 0 {
				cutoff++ // 跳过这一对
				continue
			}
		}
		break
	}

	recent := a.messages[cutoff:]

	var oldContent strings.Builder
	for _, msg := range a.messages[1:cutoff] {
		switch msg.Role {
		case types.RoleUser:
			oldContent.WriteString("用户: " + truncate(msg.Content, 200) + "\n")
		case types.RoleAssistant:
			if msg.Content != "" {
				oldContent.WriteString("助手: " + truncate(msg.Content, 200) + "\n")
			}
			for _, tc := range msg.ToolCalls {
				oldContent.WriteString(fmt.Sprintf("  [调用工具: %s]\n", tc.Function.Name))
			}
		case types.RoleTool:
			oldContent.WriteString(fmt.Sprintf("  [工具结果: %s]\n", truncate(msg.Content, 100)))
		}
	}

	summary := fmt.Sprintf("[之前的对话摘要]\n%s[摘要结束]", oldContent.String())

	a.messages = []types.Message{
		system,
		{Role: types.RoleUser, Content: summary},
		{Role: types.RoleAssistant, Content: "好的，我已了解之前的对话内容。请继续。"},
	}
	a.messages = append(a.messages, recent...)

	// 持久化压缩后的状态
	a.autoSave()

	return fmt.Sprintf("已压缩对话上下文")
}

// SaveSession saves the current session
func (a *Agent) SaveSession() error {
	session := &Session{
		ID:       a.sessionID,
		WorkDir:  a.workDir,
		Model:    a.client.GetModel(),
		Messages: a.messages,
		Usage: types.Usage{
			PromptTokens:     a.cost.InputTokens,
			CompletionTokens: a.cost.OutputTokens,
			TotalTokens:      a.cost.TotalTokens,
		},
	}

	// Generate summary from first user message
	for _, msg := range a.messages {
		if msg.Role == types.RoleUser {
			session.Summary = msg.Content
			runes := []rune(session.Summary)
			if len(runes) > 80 {
				session.Summary = string(runes[:80]) + "..."
			}
			break
		}
	}

	return a.sessions.Save(session)
}

// LoadSession loads a session by ID
func (a *Agent) LoadSession(id string) error {
	session, err := a.sessions.Load(id)
	if err != nil {
		return err
	}

	a.messages = session.Messages
	a.sessionID = session.ID
	a.cost = CostTracker{
		InputTokens:  session.Usage.PromptTokens,
		OutputTokens: session.Usage.CompletionTokens,
		TotalTokens:  session.Usage.TotalTokens,
	}

	// 刷新系统提示以匹配当前模式/风格/技能
	a.refreshSystemPrompt()

	return nil
}

// ListSessions returns saved sessions
func (a *Agent) ListSessions() ([]Session, error) {
	return a.sessions.List()
}

// autoSave saves session silently (errors ignored)
func (a *Agent) autoSave() {
	if a.sessions == nil {
		return
	}
	_ = a.SaveSession()
}

// GetMessages returns the current message history
func (a *Agent) GetMessages() []types.Message {
	return a.messages
}

// GetCost returns the cost tracker
func (a *Agent) GetCost() CostTracker {
	return a.cost
}

// EstimateContextTokens estimates current context size
func (a *Agent) EstimateContextTokens() int {
	total := 0
	for _, msg := range a.messages {
		total += llm.CountTokens(msg.Content)
		for _, tc := range msg.ToolCalls {
			total += llm.CountTokens(tc.Function.Arguments)
			total += 50
		}
	}
	return total
}

// GetSessionID returns current session ID
func (a *Agent) GetSessionID() string {
	return a.sessionID
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return s
}

// estimateTokens 根据字节长度估算token数（粗略：约3字节/token）
func estimateTokens(byteLen int) int {
	if byteLen <= 0 {
		return 0
	}
	// 粗略估算：英文约4字符/token，中文约3字节/字符≈1.5字符/token
	// 取平均约3字节/token
	tokens := byteLen / 3
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

// systemPromptLen 计算消息列表中系统提示的字节长度
func systemPromptLen(msgs []types.Message) int {
	total := 0
	for _, m := range msgs {
		if m.Role == types.RoleSystem {
			total += len(m.Content)
		}
	}
	// 加上用户消息和对话历史的粗略长度
	for _, m := range msgs {
		if m.Role == types.RoleUser || m.Role == types.RoleAssistant {
			total += len(m.Content)
		}
	}
	return total
}

// GetProject 返回检测到的项目信息
func (a *Agent) GetProject() *ProjectInfo {
	return a.project
}

// GetToolCache 返回工具缓存
func (a *Agent) GetToolCache() *tools.ToolCache {
	return a.toolCache
}

// SetAutoCompact 设置自动压缩阈值
func (a *Agent) SetAutoCompact(tokens int) {
	a.autoCompact = tokens
}

// CreateCheckpoint 创建检查点（git stash或文件备份）
func (a *Agent) CreateCheckpoint(label string) (string, error) {
	if a.project == nil || !a.project.HasGit {
		return "", fmt.Errorf("项目没有Git仓库，无法创建检查点")
	}

	// 使用 git stash push 创建带标签的检查点
	cmd := fmt.Sprintf("git stash push -m \"taiji-checkpoint:%s:%s\"", a.sessionID, label)
	result, err := tools.NewBash(a.workDir).Execute(context.Background(), map[string]interface{}{
		"command": cmd,
	})
	if err != nil {
		return "", fmt.Errorf("创建检查点失败: %w", err)
	}

	return result, nil
}

// RollbackCheckpoint 回滚到最近的检查点
func (a *Agent) RollbackCheckpoint() (string, error) {
	if a.project == nil || !a.project.HasGit {
		return "", fmt.Errorf("项目没有Git仓库，无法回滚")
	}

	result, err := tools.NewBash(a.workDir).Execute(context.Background(), map[string]interface{}{
		"command": "git stash pop",
	})
	if err != nil {
		return "", fmt.Errorf("回滚检查点失败: %w", err)
	}

	return result, nil
}

// AutoResumeSession 尝试自动恢复上次会话
func (a *Agent) AutoResumeSession() (bool, string) {
	sessions, err := a.sessions.List()
	if err != nil || len(sessions) == 0 {
		return false, ""
	}

	// 找到最近的会话（24小时内）
	latest := sessions[0]
	if time.Since(latest.UpdatedAt) > 24*time.Hour {
		return false, ""
	}

	// 恢复会话
	a.messages = latest.Messages
	a.sessionID = latest.ID
	a.cost = CostTracker{
		InputTokens:  latest.Usage.PromptTokens,
		OutputTokens: latest.Usage.CompletionTokens,
		TotalTokens:  latest.Usage.TotalTokens,
	}

	// 刷新系统提示以匹配当前模式/风格/技能
	a.refreshSystemPrompt()

	summary := latest.Summary
	if summary == "" {
		summary = fmt.Sprintf("%d条消息", len(latest.Messages))
	}

	return true, fmt.Sprintf("已恢复上次会话: %s (%s)", latest.ID, summary)
}

// ─── 新增方法 ────────────────────────────────────────────────────────────

// GetMode 返回当前运行模式
func (a *Agent) GetMode() AgentMode { return a.mode }

// SetMode 设置运行模式
func (a *Agent) SetMode(mode AgentMode) {
	a.mode = mode
	// 规划模式下强制只读权限
	if mode == ModePlan {
		a.perm.AutoApproveAll = false
	}
	a.refreshSystemPrompt()
}

// GetOutputStyle 返回输出风格
func (a *Agent) GetOutputStyle() string { return a.outputStyle }

// SetOutputStyle 设置输出风格
func (a *Agent) SetOutputStyle(style string) {
	switch style {
	case "concise", "normal", "detailed":
		a.outputStyle = style
		a.refreshSystemPrompt()
	}
}

// refreshSystemPrompt 更新消息历史中的系统提示
func (a *Agent) refreshSystemPrompt() {
	if len(a.messages) == 0 {
		return
	}
	// 找到并替换第一条系统消息
	newPrompt := a.SystemPrompt()
	for i, msg := range a.messages {
		if msg.Role == types.RoleSystem {
			a.messages[i].Content = newPrompt
			return
		}
	}
}

// GetHookManager 返回Hook管理器
func (a *Agent) GetHookManager() *HookManager { return a.hookMgr }

// GetSkillManager 返回技能管理器
func (a *Agent) GetSkillManager() *SkillManager { return a.skillMgr }

// GetTodoStore 返回任务存储
func (a *Agent) GetTodoStore() *tools.TodoStore { return a.todoStore }

// SetTodoStore 设置外部任务存储（共享实例）
func (a *Agent) SetTodoStore(store *tools.TodoStore) { a.todoStore = store }

// SaveSnapshot 保存消息快照（用于rewind）
func (a *Agent) SaveSnapshot(label string) {
	a.messageSnapshots = append(a.messageSnapshots, messageSnapshot{
		Timestamp: time.Now(),
		MsgCount:  len(a.messages),
		Label:     label,
	})
	// 最多保留20个快照
	if len(a.messageSnapshots) > 20 {
		a.messageSnapshots = a.messageSnapshots[len(a.messageSnapshots)-20:]
	}
}

// Rewind 回退到指定快照
func (a *Agent) Rewind(index int) (string, error) {
	if index < 0 || index >= len(a.messageSnapshots) {
		return "", fmt.Errorf("无效的快照索引: %d (共 %d 个快照)", index, len(a.messageSnapshots))
	}

	snapshot := a.messageSnapshots[index]
	if snapshot.MsgCount > len(a.messages) {
		return "", fmt.Errorf("快照消息数超过当前消息数")
	}

	// 截断消息到快照点
	a.messages = a.messages[:snapshot.MsgCount]
	// 删除此快照及之后的快照
	a.messageSnapshots = a.messageSnapshots[:index]

	return fmt.Sprintf("已回退到快照: %s (%d条消息)", snapshot.Label, snapshot.MsgCount), nil
}

// ListSnapshots 列出所有快照
func (a *Agent) ListSnapshots() string {
	if len(a.messageSnapshots) == 0 {
		return "没有可用的快照"
	}

	var sb strings.Builder
	for i, s := range a.messageSnapshots {
		sb.WriteString(fmt.Sprintf("  [%d] %s — %s (%d条消息)\n",
			i, s.Timestamp.Format("15:04:05"), s.Label, s.MsgCount))
	}
	return sb.String()
}

// ExportConversation 导出对话为Markdown
func (a *Agent) ExportConversation() string {
	var sb strings.Builder
	sb.WriteString("# 太极 Code 对话记录\n\n")
	sb.WriteString(fmt.Sprintf("导出时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("模型: %s\n", a.client.GetModel()))
	sb.WriteString(fmt.Sprintf("会话: %s\n\n", a.sessionID))
	sb.WriteString("---\n\n")

	for _, msg := range a.messages {
		switch msg.Role {
		case types.RoleUser:
			sb.WriteString("## 用户\n\n")
			sb.WriteString(msg.Content + "\n\n")
		case types.RoleAssistant:
			sb.WriteString("## 助手\n\n")
			if msg.Content != "" {
				sb.WriteString(msg.Content + "\n\n")
			}
			for _, tc := range msg.ToolCalls {
				sb.WriteString(fmt.Sprintf("**工具调用**: `%s`\n```json\n%s\n```\n\n",
					tc.Function.Name, tc.Function.Arguments))
			}
		case types.RoleTool:
			sb.WriteString(fmt.Sprintf("**工具结果** (`%s`):\n", msg.Name))
			content := msg.Content
			runes := []rune(content)
			if len(runes) > 500 {
				content = string(runes[:500]) + "\n...(已截断)"
			}
			sb.WriteString("```\n" + content + "\n```\n\n")
		}
		sb.WriteString("---\n\n")
	}

	return sb.String()
}

// GetStatus 获取详细状态信息
func (a *Agent) GetStatus() string {
	tokens := a.EstimateContextTokens()
	msgCount := len(a.messages)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("会话: %s\n", a.sessionID))
	sb.WriteString(fmt.Sprintf("模型: %s\n", a.client.GetModel()))
	sb.WriteString(fmt.Sprintf("模式: %s\n", a.mode))
	sb.WriteString(fmt.Sprintf("输出风格: %s\n", a.outputStyle))
	sb.WriteString(fmt.Sprintf("上下文: ~%d tokens (%d 条消息)\n", tokens, msgCount))
	sb.WriteString(fmt.Sprintf("费用: %s\n", a.cost.FormatCost()))
	sb.WriteString(fmt.Sprintf("工具调用: %d 次\n", a.cost.ToolCalls))
	sb.WriteString(fmt.Sprintf("工作目录: %s\n", a.workDir))

	if a.project != nil && a.project.Type != "unknown" {
		sb.WriteString(fmt.Sprintf("项目: %s (%s)\n", a.project.Name, a.project.Type))
	}

	// Hook信息
	if a.hookMgr != nil {
		sb.WriteString(fmt.Sprintf("Hooks: 已注册\n"))
	}

	// 技能信息
	if a.skillMgr != nil {
		sb.WriteString(fmt.Sprintf("技能: 已加载\n"))
	}

	return sb.String()
}

// Doctor 诊断检查
func (a *Agent) Doctor() string {
	var sb strings.Builder
	sb.WriteString("太极 Code 诊断报告\n")
	sb.WriteString("════════════════════\n\n")

	// 1. API连通性
	sb.WriteString("1. API连通性: ")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := a.client.Chat(ctx, []types.Message{
		{Role: types.RoleUser, Content: "hi"},
	}, nil, 10)
	if err != nil {
		sb.WriteString(fmt.Sprintf("✗ 失败 — %v\n", err))
	} else {
		sb.WriteString("✓ 正常\n")
	}

	// 2. 配置检查
	sb.WriteString(fmt.Sprintf("2. 模型: %s ✓\n", a.client.GetModel()))
	sb.WriteString(fmt.Sprintf("3. 工作目录: %s ✓\n", a.workDir))

	// 3. 项目检测
	if a.project != nil {
		sb.WriteString(fmt.Sprintf("4. 项目类型: %s", a.project.Type))
		if a.project.Type != "unknown" {
			sb.WriteString(" ✓")
		}
		sb.WriteString("\n")
	}

	// 4. 记忆文件
	memPath := filepath.Join(a.workDir, "TAIJI.md")
	if _, err := os.Stat(memPath); err == nil {
		sb.WriteString("5. 记忆文件(TAIJI.md): ✓ 存在\n")
	} else {
		sb.WriteString("5. 记忆文件(TAIJI.md): ✗ 不存在\n")
	}

	// 5. Git
	if a.project != nil && a.project.HasGit {
		sb.WriteString("6. Git仓库: ✓\n")
	} else {
		sb.WriteString("6. Git仓库: ✗ 未检测到\n")
	}

	// 6. 工具数量
	sb.WriteString(fmt.Sprintf("7. 已注册工具: %d 个\n", len(a.registry.Names())))

	// 7. 会话
	sessions, _ := a.ListSessions()
	sb.WriteString(fmt.Sprintf("8. 已保存会话: %d 个\n", len(sessions)))

	return sb.String()
}

// ReviewCode 代码审查（简化版）
func (a *Agent) ReviewCode(scope string) string {
	ctx := context.Background()

	prompt := "请对以下代码进行审查，指出潜在的问题、bug、安全隐患和改进建议:\n\n"
	if scope != "" {
		prompt += scope
	} else {
		// 默认审查当前目录的变更
		bash := tools.NewBash(a.workDir)
		diff, err := bash.Execute(ctx, map[string]interface{}{
			"command": "git diff --stat && echo '---' && git diff",
		})
		if err != nil {
			return fmt.Sprintf("获取代码变更失败: %v", err)
		}
		if diff == "" || diff == "[命令执行成功,无输出]" {
			return "没有检测到代码变更。请先修改代码或使用 git add 暂存变更。"
		}
		prompt += "当前Git变更:\n```\n" + diff + "\n```"
	}

	resp, err := a.Chat(ctx, prompt)
	if err != nil {
		return fmt.Sprintf("审查失败: %v", err)
	}
	return resp
}
