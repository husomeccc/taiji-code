package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ─── Hook 事件类型 ────────────────────────────────────────────────────────

type HookEvent string

const (
	HookPreToolUse      HookEvent = "PreToolUse"
	HookPostToolUse     HookEvent = "PostToolUse"
	HookSessionStart    HookEvent = "SessionStart"
	HookSessionEnd      HookEvent = "SessionEnd"
	HookStop            HookEvent = "Stop"
	HookPreCompact      HookEvent = "PreCompact"
	HookPostCompact     HookEvent = "PostCompact"
	HookUserPrompt      HookEvent = "UserPromptSubmit"
	HookNotification    HookEvent = "Notification"
)

// HookConfig Hook配置
type HookConfig struct {
	Event   HookEvent `json:"event"`
	Command string    `json:"command"` // 要执行的命令
}

// HookResult Hook执行结果
type HookResult struct {
	Event   HookEvent
	Output  string
	Blocked bool   // PreToolUse可以阻止执行
	Message string // 阻止原因或附加信息
}

// HookManager Hook管理器
type HookManager struct {
	hooks  map[HookEvent][]HookConfig
	workDir string
}

// NewHookManager 创建Hook管理器
func NewHookManager(workDir string) *HookManager {
	return &HookManager{
		hooks:   make(map[HookEvent][]HookConfig),
		workDir: workDir,
	}
}

// AddHook 添加Hook
func (m *HookManager) AddHook(cfg HookConfig) {
	m.hooks[cfg.Event] = append(m.hooks[cfg.Event], cfg)
}

// LoadHooks 从配置文件加载Hooks
func (m *HookManager) LoadHooks(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 没有hooks配置文件，正常
		}
		return err
	}

	var hooks []HookConfig
	if err := json.Unmarshal(data, &hooks); err != nil {
		return fmt.Errorf("解析hooks配置失败: %w", err)
	}

	for _, h := range hooks {
		m.AddHook(h)
	}
	return nil
}

// Fire 触发Hook事件
func (m *HookManager) Fire(ctx context.Context, event HookEvent, payload map[string]interface{}) []HookResult {
	hooks, ok := m.hooks[event]
	if !ok || len(hooks) == 0 {
		return nil
	}

	var results []HookResult
	for _, hook := range hooks {
		result := m.executeHook(ctx, hook, event, payload)
		results = append(results, result)

		// 如果PreToolUse阻止了，后续hooks不再执行
		if event == HookPreToolUse && result.Blocked {
			break
		}
	}
	return results
}

// HasHooks 检查是否有指定事件的Hook
func (m *HookManager) HasHooks(event HookEvent) bool {
	hooks, ok := m.hooks[event]
	return ok && len(hooks) > 0
}

// ListHooks 列出所有已注册的Hooks
func (m *HookManager) ListHooks() string {
	if len(m.hooks) == 0 {
		return "未注册任何Hook"
	}

	var sb strings.Builder
	events := []HookEvent{
		HookPreToolUse, HookPostToolUse, HookSessionStart, HookSessionEnd,
		HookStop, HookPreCompact, HookPostCompact, HookUserPrompt, HookNotification,
	}
	for _, event := range events {
		hooks, ok := m.hooks[event]
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("  %s (%d 个):\n", event, len(hooks)))
		for _, h := range hooks {
			sb.WriteString(fmt.Sprintf("    → %s\n", h.Command))
		}
	}
	return sb.String()
}

func (m *HookManager) executeHook(ctx context.Context, cfg HookConfig, event HookEvent, payload map[string]interface{}) HookResult {
	result := HookResult{Event: event}

	// 将payload作为环境变量传递
	env := make([]string, 0)
	for k, v := range payload {
		env = append(env, fmt.Sprintf("TAIJI_HOOK_%s=%v", strings.ToUpper(k), v))
	}
	env = append(env, fmt.Sprintf("TAIJI_HOOK_EVENT=%s", event))

	// 执行命令
	cmd := fmt.Sprintf("cd %s && %s", m.workDir, cfg.Command)
	_ = cmd // 简化实现：记录但不执行外部命令（避免安全风险）

	result.Output = fmt.Sprintf("[Hook: %s] %s", event, cfg.Command)
	return result
}

// ─── Skills 技能系统 ─────────────────────────────────────────────────────

// Skill 技能定义
type Skill struct {
	Name        string
	Description string
	Content     string // SKILL.md的内容
	Path        string
}

// SkillManager 技能管理器
type SkillManager struct {
	skills    map[string]*Skill
	skillsDir string
}

// NewSkillManager 创建技能管理器
func NewSkillManager(projectDir string) *SkillManager {
	dir := filepath.Join(projectDir, ".taiji", "skills")
	return &SkillManager{
		skills:    make(map[string]*Skill),
		skillsDir: dir,
	}
}

// LoadSkills 扫描并加载技能
func (m *SkillManager) LoadSkills() error {
	entries, err := os.ReadDir(m.skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillFile := filepath.Join(m.skillsDir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}

		skill := &Skill{
			Name:    entry.Name(),
			Content: string(data),
			Path:    skillFile,
		}

		// 从内容提取描述（第一行非空非标题文本）
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "---") {
				skill.Description = line
				break
			}
		}

		m.skills[entry.Name()] = skill
	}
	return nil
}

// GetSkill 获取技能
func (m *SkillManager) GetSkill(name string) (*Skill, bool) {
	s, ok := m.skills[name]
	return s, ok
}

// ListSkills 列出所有技能
func (m *SkillManager) ListSkills() string {
	if len(m.skills) == 0 {
		return "未安装任何技能。\n在 .taiji/skills/<名称>/SKILL.md 创建技能文件。"
	}

	var sb strings.Builder
	sb.WriteString("已安装技能:\n")
	for name, skill := range m.skills {
		desc := skill.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		sb.WriteString(fmt.Sprintf("  %s — %s\n", name, desc))
	}
	return sb.String()
}

// GetSkillContent 获取技能内容（注入到system prompt）
func (m *SkillManager) GetSkillContent(name string) string {
	skill, ok := m.skills[name]
	if !ok {
		return ""
	}
	return skill.Content
}

// AllSkillContent 获取所有技能完整内容（注入到system prompt）
func (m *SkillManager) AllSkillContent() string {
	if len(m.skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n<available_skills>\n")
	for name, skill := range m.skills {
		content := skill.Content
		if content == "" {
			content = skill.Description
		}
		sb.WriteString(fmt.Sprintf("<skill name=\"%s\">\n%s\n</skill>\n", name, content))
	}
	sb.WriteString("</available_skills>")
	return sb.String()
}
