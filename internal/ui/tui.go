package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─── 颜色方案 (Qoder 风格 - 蓝紫强调 + 灰度基底) ──────────────────────────
var (
	// 主文字色
	primaryText = lipgloss.Color("15")
	// 次要文字 - 状态栏、工具结果
	secondaryText = lipgloss.Color("245")
	// 边框/分隔线
	borderGray = lipgloss.Color("238")
	// 错误色
	errRed = lipgloss.Color("203")
	// 成功色
	successGreen = lipgloss.Color("119")
	// 警告/权限色
	warnYellow = lipgloss.Color("220")
	// 主强调色 - 蓝
	accentBlue = lipgloss.Color("75")
	// 次强调色 - 紫
	accentPurple = lipgloss.Color("141")
	// 用户标签色
	userYellow = lipgloss.Color("221")
	// 助手标签色
	assistantBlue = lipgloss.Color("117")
	// 工具标签色
	toolPurple = lipgloss.Color("141")
	// 卡片边框色
	cardBorder = lipgloss.Color("240")
)

// ─── 样式定义 (Qoder 风格) ──────────────────────────────────────────────
var (
	// 头部样式 - 蓝强调
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentBlue)

	// 头部版本
	headerVersionStyle = lipgloss.NewStyle().
				Foreground(secondaryText)

	// 用户标签
	userLabelStyle = lipgloss.NewStyle().
			Foreground(userYellow).
			Bold(true)

	// 助手标签
	assistantLabelStyle = lipgloss.NewStyle().
				Foreground(assistantBlue).
				Bold(true)

	// 工具卡片边框
	cardBorderStyle = lipgloss.NewStyle().
			Foreground(cardBorder)

	// 工具名称
	toolNameStyle = lipgloss.NewStyle().
			Foreground(toolPurple).
			Bold(true)

	// 工具参数
	toolArgsStyle = lipgloss.NewStyle().
			Foreground(secondaryText)

	// 工具结果
	toolResultStyle = lipgloss.NewStyle().
			Foreground(secondaryText)

	// 错误
	errorStyle = lipgloss.NewStyle().
			Foreground(errRed)

	// 权限提示
	permStyle = lipgloss.NewStyle().
			Foreground(warnYellow).
			Bold(true)

	// 状态栏
	statusBarStyle = lipgloss.NewStyle().
			Foreground(secondaryText)

	// 状态栏活跃态(working)
	statusActiveStyle = lipgloss.NewStyle().
				Foreground(accentBlue).
				Italic(true)

	// 输入提示符
	inputPromptStyle = lipgloss.NewStyle().
				Foreground(accentBlue).
				Bold(true)

	// 分隔线
	dividerStyle = lipgloss.NewStyle().
			Foreground(borderGray)

	// 信息块
	infoStyle = lipgloss.NewStyle().
			Foreground(secondaryText).
			Italic(true)

	// 用量块
	usageStyle = lipgloss.NewStyle().
			Foreground(secondaryText)

	// 模式标签
	modePlanStyle = lipgloss.NewStyle().
			Foreground(warnYellow).
			Bold(true)

	modeAutoStyle = lipgloss.NewStyle().
			Foreground(successGreen)
)

// ─── 输出块类型 ──────────────────────────────────────────────────────────
type BlockType string

const (
	BlockUser       BlockType = "user"
	BlockAssistant  BlockType = "assistant"
	BlockToolCall   BlockType = "tool_call"
	BlockToolResult BlockType = "tool_result"
	BlockError      BlockType = "error"
	BlockInfo       BlockType = "info"
	BlockPermission BlockType = "permission"
	BlockUsage      BlockType = "usage"
	BlockDivider    BlockType = "divider"
)

// OutputBlock 表示一个输出块
type OutputBlock struct {
	Type    BlockType
	Content string
	// 工具调用专用
	ToolName string
	ToolArgs string
	IsError  bool
}

// ─── 消息类型 ────────────────────────────────────────────────────────────

// AgentResponse 代理响应完成
type AgentResponse struct {
	Content string
	Err     error
}

// ToolCallNotify 工具调用通知
type ToolCallNotify struct {
	Name string
	Args string
}

// ToolResultNotify 工具结果通知
type ToolResultNotify struct {
	Name   string
	Result string
	IsErr  bool
}

// UsageNotify 用量通知
type UsageNotify struct {
	Text string
}

// PermissionRequest 权限请求
type PermissionRequest struct {
	ToolName    string
	Description string
	ResponseCh  chan<- bool
}

// BlockMsg 线程安全的块添加消息（通过p.Send从agent goroutine发送）
type BlockMsg struct {
	BlockType string
	Content   string
	ToolName  string
	ToolArgs  string
}

// StreamToken 流式token
type StreamToken struct {
	Text string
	Done bool   // 是否结束
	Err  error  // 错误
}

// PermissionMode 权限模式
type PermissionMode string

const (
	PermDefault  PermissionMode = "default"
	PermAutoEdit PermissionMode = "auto_edit"
	PermPlanOnly PermissionMode = "plan_only"
	PermBypass   PermissionMode = "bypass"
)

// ─── 主模型 ──────────────────────────────────────────────────────────────

type Model struct {
	// 输入状态
	input     string
	cursor    int
	history   []string
	histIdx   int
	multiLine bool

	// 输出
	blocks []OutputBlock

	// 窗口
	width  int
	height int

	// 状态
	working bool
	model   string // 当前模型名
	version string // 版本号

	// 运行模式
	agentMode  string         // "normal", "plan", "auto_edit"
	permMode   PermissionMode // 权限模式
	vimMode    bool           // vim键绑定

	// 流式显示
	streamingBlock int    // 当前流式块索引(-1表示无)
	streamBuffer   string // 流式内容缓冲

	// 权限队列
	pendingPerms []PermissionRequest

	// 回调
	AgentRunner    func(string) <-chan AgentResponse
	CommandHandler func(string) string
	CancelFunc     func()
	TabCompleter   func(string) []string

	// Tab补全状态
	tabCandidates []string
	tabIdx        int

	// 滚动
	scrollOffset int
}

// NewModel 创建新模型
func NewModel() *Model {
	return &Model{
		history:        []string{},
		histIdx:        -1,
		model:          "deepseek-chat",
		version:        "0.3.0",
		agentMode:      "normal",
		permMode:       PermDefault,
		streamingBlock: -1,
	}
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case AgentResponse:
		m.working = false
		if msg.Err != nil {
			// Ctrl+C 已显示 info 块，不再重复显示 context canceled 错误
			if !errors.Is(msg.Err, context.Canceled) {
				m.blocks = append(m.blocks, OutputBlock{
					Type:    BlockError,
					Content: msg.Err.Error(),
				})
			}
		} else if msg.Content != "" {
			m.blocks = append(m.blocks, OutputBlock{
				Type:    BlockAssistant,
				Content: msg.Content,
			})
		}
		m.scrollToBottom()
		return m, nil

	case ToolCallNotify:
		m.blocks = append(m.blocks, OutputBlock{
			Type:     BlockToolCall,
			ToolName: msg.Name,
			ToolArgs: msg.Args,
			Content:  formatToolCall(msg.Name, msg.Args),
		})
		m.scrollToBottom()
		return m, nil

	case ToolResultNotify:
		blockType := BlockToolResult
		if msg.IsErr {
			blockType = BlockError
		}
		m.blocks = append(m.blocks, OutputBlock{
			Type:     blockType,
			ToolName: msg.Name,
			Content:  msg.Result,
			IsError:  msg.IsErr,
		})
		m.scrollToBottom()
		return m, nil

	case UsageNotify:
		m.blocks = append(m.blocks, OutputBlock{
			Type:    BlockUsage,
			Content: msg.Text,
		})
		return m, nil

	case PermissionRequest:
		m.pendingPerms = append(m.pendingPerms, msg)
		m.working = false
		return m, nil

	case BlockMsg:
		bt := BlockType(msg.BlockType)
		m.blocks = append(m.blocks, OutputBlock{
			Type:     bt,
			Content:  msg.Content,
			ToolName: msg.ToolName,
			ToolArgs: msg.ToolArgs,
		})
		m.scrollToBottom()
		return m, nil

	case StreamToken:
		if msg.Err != nil {
			m.blocks = append(m.blocks, OutputBlock{
				Type:    BlockError,
				Content: msg.Err.Error(),
			})
			m.streamingBlock = -1
			m.streamBuffer = ""
			return m, nil
		}
		m.streamBuffer += msg.Text
		if msg.Done {
			if m.streamBuffer != "" {
				if m.streamingBlock >= 0 && m.streamingBlock < len(m.blocks) {
					m.blocks[m.streamingBlock].Content = m.streamBuffer
				} else {
					m.blocks = append(m.blocks, OutputBlock{
						Type:    BlockAssistant,
						Content: m.streamBuffer,
					})
				}
			}
			m.streamingBlock = -1
			m.streamBuffer = ""
		} else if m.streamBuffer != "" {
			if m.streamingBlock >= 0 && m.streamingBlock < len(m.blocks) {
				m.blocks[m.streamingBlock].Content = m.streamBuffer
			} else {
				m.blocks = append(m.blocks, OutputBlock{
					Type:    BlockAssistant,
					Content: m.streamBuffer,
				})
				m.streamingBlock = len(m.blocks) - 1
			}
		}
		m.scrollToBottom()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// 权限确认模式
	if len(m.pendingPerms) > 0 {
		return m.handlePermKey(msg)
	}

	switch msg.String() {
	case "shift+tab":
		switch m.agentMode {
		case "normal":
			m.agentMode = "plan"
			m.blocks = append(m.blocks, OutputBlock{
				Type:    BlockInfo,
				Content: "已切换到规划模式(只读)",
			})
		case "plan":
			m.agentMode = "auto_edit"
			m.blocks = append(m.blocks, OutputBlock{
				Type:    BlockInfo,
				Content: "已切换到自动编辑模式",
			})
		case "auto_edit":
			m.agentMode = "normal"
			m.blocks = append(m.blocks, OutputBlock{
				Type:    BlockInfo,
				Content: "已切换到标准模式",
			})
		}
		return m, nil

	case "ctrl+c":
		if m.working && m.CancelFunc != nil {
			m.CancelFunc()
			m.working = false
			m.blocks = append(m.blocks, OutputBlock{
				Type:    BlockInfo,
				Content: "已中断当前操作",
			})
			return m, nil
		}
		return m, tea.Quit

	case "ctrl+g":
		m.multiLine = !m.multiLine
		return m, nil

	case "ctrl+l":
		m.blocks = nil
		m.scrollOffset = 0
		return m, nil

	case "enter":
		if m.working {
			return m, nil
		}
		if m.multiLine {
			m.input = m.input[:m.cursor] + "\n" + m.input[m.cursor:]
			m.cursor++
			return m, nil
		}
		return m.submit()

	case "ctrl+enter":
		if m.multiLine && !m.working {
			return m.submit()
		}
		return m, nil

	case "backspace":
		if m.cursor > 0 {
			m.input = m.input[:m.cursor-1] + m.input[m.cursor:]
			m.cursor--
		}

	case "delete":
		if m.cursor < len(m.input) {
			m.input = m.input[:m.cursor] + m.input[m.cursor+1:]
		}

	case "left":
		if m.cursor > 0 {
			m.cursor--
		}

	case "right":
		if m.cursor < len(m.input) {
			m.cursor++
		}

	case "up":
		if m.histIdx > 0 {
			m.histIdx--
			m.input = m.history[m.histIdx]
			m.cursor = len(m.input)
		}

	case "down":
		if m.histIdx < len(m.history)-1 {
			m.histIdx++
			m.input = m.history[m.histIdx]
			m.cursor = len(m.input)
		} else {
			m.histIdx = len(m.history)
			m.input = ""
			m.cursor = 0
		}

	case "home", "ctrl+a":
		m.cursor = 0

	case "end", "ctrl+e":
		m.cursor = len(m.input)

	case "pgup":
		m.scrollOffset += 10

	case "pgdown":
		m.scrollOffset -= 10
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}

	case "tab":
		if m.TabCompleter != nil && !m.working {
			if len(m.tabCandidates) == 0 {
				candidates := m.TabCompleter(m.input)
				if len(candidates) == 1 {
					m.input = candidates[0]
					m.cursor = len(m.input)
					m.tabCandidates = nil
				} else if len(candidates) > 1 {
					m.tabCandidates = candidates
					m.tabIdx = 0
					m.input = candidates[0]
					m.cursor = len(m.input)
					m.blocks = append(m.blocks, OutputBlock{
						Type:    BlockInfo,
						Content: "补全候选: " + strings.Join(candidates, "  "),
					})
				}
			} else {
				m.tabIdx = (m.tabIdx + 1) % len(m.tabCandidates)
				m.input = m.tabCandidates[m.tabIdx]
				m.cursor = len(m.input)
			}
		}

	default:
		m.tabCandidates = nil
		m.tabIdx = 0

		// Vim模式: j/k/h/l 导航
		if m.vimMode && !m.working && m.input == "" {
			switch msg.String() {
			case "j":
				m.scrollOffset -= 3
				if m.scrollOffset < 0 {
					m.scrollOffset = 0
				}
				return m, nil
			case "k":
				m.scrollOffset += 3
				return m, nil
			}
		}

		if msg.Type == tea.KeyRunes {
			ch := string(msg.Runes)
			m.input = m.input[:m.cursor] + ch + m.input[m.cursor:]
			m.cursor += len([]rune(ch))
		}
	}

	return m, nil
}

func (m *Model) handlePermKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	perm := m.pendingPerms[0]

	switch msg.String() {
	case "y", "Y":
		perm.ResponseCh <- true
		m.pendingPerms = m.pendingPerms[1:]
		m.blocks = append(m.blocks, OutputBlock{
			Type:    BlockInfo,
			Content: "✓ 已允许: " + perm.Description,
		})
		m.working = true
	case "n", "N":
		perm.ResponseCh <- false
		m.pendingPerms = m.pendingPerms[1:]
		m.blocks = append(m.blocks, OutputBlock{
			Type:    BlockInfo,
			Content: "✗ 已拒绝: " + perm.Description,
		})
		m.working = true
	case "ctrl+c":
		perm.ResponseCh <- false
		m.pendingPerms = nil
		m.working = false
	}

	return m, nil
}

func (m *Model) submit() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.input)
	if input == "" {
		return m, nil
	}

	m.history = append(m.history, input)
	m.histIdx = len(m.history)

	m.blocks = append(m.blocks, OutputBlock{Type: BlockUser, Content: input})
	m.input = ""
	m.cursor = 0
	m.tabCandidates = nil
	m.tabIdx = -1

	if strings.HasPrefix(input, "/") {
		if m.CommandHandler != nil {
			result := m.CommandHandler(input)
			if result != "" {
				m.blocks = append(m.blocks, OutputBlock{Type: BlockInfo, Content: result})
			}
		}
		return m, nil
	}

	if m.AgentRunner != nil {
		m.working = true
		ch := m.AgentRunner(input)
		return m, waitForAgent(ch)
	}
	return m, nil
}

// ─── 渲染 ────────────────────────────────────────────────────────────────

func (m *Model) View() string {
	var sb strings.Builder

	// ── 头部 (Qoder 风格) ──
	sb.WriteString(m.renderHeader())
	sb.WriteString("\n")

	// ── 内容区域 ──
	contentHeight := m.height - 7 // 头部2 + 分隔1 + 状态1 + 输入2 + 底边1
	if contentHeight < 5 {
		contentHeight = 10
	}

	rendered := m.renderAllBlocks()
	lines := strings.Split(rendered, "\n")

	// 应用滚动偏移
	totalLines := len(lines)
	startLine := 0
	if m.scrollOffset > 0 {
		startLine = totalLines - contentHeight - m.scrollOffset
		if startLine < 0 {
			startLine = 0
		}
	} else {
		if totalLines > contentHeight {
			startLine = totalLines - contentHeight
		}
	}

	endLine := startLine + contentHeight
	if endLine > totalLines {
		endLine = totalLines
	}

	for i := startLine; i < endLine; i++ {
		sb.WriteString(lines[i])
		sb.WriteString("\n")
	}

	// 填充空行
	for i := endLine - startLine; i < contentHeight; i++ {
		sb.WriteString("\n")
	}

	// ── 状态栏 ──
	sb.WriteString(m.renderStatusBar())
	sb.WriteString("\n")

	// ── 分隔线 ──
	sb.WriteString(dividerStyle.Render(strings.Repeat("─", m.width)))
	sb.WriteString("\n")

	// ── 权限提示 或 输入区 ──
	if len(m.pendingPerms) > 0 {
		sb.WriteString(m.renderPermPrompt())
	} else {
		sb.WriteString(m.renderInput())
	}

	return sb.String()
}

// renderHeader 渲染头部 (Qoder 风格: ⚡ 太极 Code)
func (m *Model) renderHeader() string {
	icon := headerStyle.Render("⚡ ")
	title := headerStyle.Render("太极 Code")
	ver := headerVersionStyle.Render(" v" + m.version)
	return icon + title + ver
}

// renderStatusBar 渲染状态栏 (Qoder 风格: ⚡ 模型 | 轮次 | 模式)
func (m *Model) renderStatusBar() string {
	if m.working {
		text := statusActiveStyle.Render("⚡ thinking...")
		hint := statusBarStyle.Render("  (Ctrl+C to interrupt)")
		return text + hint
	}

	// 统计轮次
	msgCount := 0
	for _, b := range m.blocks {
		if b.Type == BlockUser {
			msgCount++
		}
	}

	// 构建状态栏内容
	parts := []string{}

	// 模型名
	if m.model != "" {
		parts = append(parts, m.model)
	}

	// 轮次
	parts = append(parts, fmt.Sprintf("%d turns", msgCount))

	// 模式
	switch m.agentMode {
	case "plan":
		parts = append(parts, modePlanStyle.Render("[plan]"))
	case "auto_edit":
		parts = append(parts, modeAutoStyle.Render("[auto]"))
	}

	// 用 | 连接 (Qoder 风格)
	separator := statusBarStyle.Render(" │ ")
	statusText := statusBarStyle.Render("⚡ ")
	for i, part := range parts {
		if i > 0 {
			statusText += separator
		}
		statusText += part
	}

	return statusText
}

// renderPermPrompt 渲染权限确认提示
func (m *Model) renderPermPrompt() string {
	perm := m.pendingPerms[0]

	icon := permStyle.Render("⚠ ")
	desc := permStyle.Render("权限请求: " + perm.Description)
	prompt := permStyle.Render("\n  是否允许? [y/n] ")

	return icon + desc + prompt
}

// renderInput 渲染输入区 (Qoder 风格: ⚡ > )
func (m *Model) renderInput() string {
	prompt := inputPromptStyle.Render("⚡ > ")

	if m.working {
		return prompt + statusActiveStyle.Render("thinking...")
	}

	inputRunes := []rune(m.input)
	if len(inputRunes) == 0 {
		return prompt + lipgloss.NewStyle().Reverse(true).Render(" ")
	}

	var display string
	if m.cursor >= len(inputRunes) {
		display = string(inputRunes) + lipgloss.NewStyle().Reverse(true).Render(" ")
	} else {
		before := string(inputRunes[:m.cursor])
		cursorCh := string(inputRunes[m.cursor : m.cursor+1])
		after := ""
		if m.cursor+1 < len(inputRunes) {
			after = string(inputRunes[m.cursor+1:])
		}
		display = before + lipgloss.NewStyle().Reverse(true).Render(cursorCh) + after
	}

	return prompt + display
}

// renderAllBlocks 渲染所有输出块
func (m *Model) renderAllBlocks() string {
	var sb strings.Builder
	for i, block := range m.blocks {
		sb.WriteString(m.renderBlock(block))
		if i < len(m.blocks)-1 {
			if block.Type == BlockAssistant || block.Type == BlockUser {
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}

// renderBlock 渲染单个输出块
func (m *Model) renderBlock(block OutputBlock) string {
	switch block.Type {
	case BlockUser:
		return m.renderUserBlock(block.Content)
	case BlockAssistant:
		return m.renderAssistantBlock(block.Content)
	case BlockToolCall:
		return m.renderToolCallBlock(block)
	case BlockToolResult:
		return m.renderToolResultBlock(block)
	case BlockError:
		return m.renderErrorBlock(block)
	case BlockInfo:
		return m.renderInfoBlock(block.Content)
	case BlockUsage:
		return m.renderUsageBlock(block.Content)
	default:
		return block.Content
	}
}

// renderUserBlock 渲染用户消息 (Qoder 风格: 👤 用户 标签)
func (m *Model) renderUserBlock(content string) string {
	label := userLabelStyle.Render("👤 用户")
	lines := strings.Split(content, "\n")
	var sb strings.Builder

	sb.WriteString(label + "\n")
	for _, line := range lines {
		sb.WriteString("  " + line + "\n")
	}

	return sb.String()
}

// renderAssistantBlock 渲染助手消息 (Qoder 风格: 🤖 助手 标签 + markdown)
func (m *Model) renderAssistantBlock(content string) string {
	rendered := renderMarkdown(content, m.width-6)
	if rendered == "" {
		return ""
	}

	label := assistantLabelStyle.Render("🤖 助手")
	lines := strings.Split(rendered, "\n")
	var sb strings.Builder

	sb.WriteString(label + "\n")
	for _, line := range lines {
		sb.WriteString("  " + line + "\n")
	}

	return sb.String()
}

// renderToolCallBlock 渲染工具调用 (Qoder 风格: 卡片式)
//
//	┌─ 📦 工具名 ──────────────┐
//	│  (参数)                   │
//	└──────────────────────────┘
func (m *Model) renderToolCallBlock(block OutputBlock) string {
	cnName := translateToolName(block.ToolName)
	nameDisplay := "🔧 " + cnName

	// 计算卡片宽度
	contentWidth := m.width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}
	if contentWidth > 60 {
		contentWidth = 60
	}

	border := cardBorderStyle.Render("─")
	cornerTL := cardBorderStyle.Render("┌─")
	cornerTR := cardBorderStyle.Render("─┐")
	cornerBL := cardBorderStyle.Render("└─")
	cornerBR := cardBorderStyle.Render("─┘")
	pipe := cardBorderStyle.Render("│")

	// 顶行: ┌─ 🔧 工具名 ─────┐
	topLine := cornerTL + " " + toolNameStyle.Render(nameDisplay)
	topFill := contentWidth - lipgloss.Width(nameDisplay) - 2
	if topFill < 1 {
		topFill = 1
	}
	topLine += " " + strings.Repeat(border, topFill) + cornerTR

	var sb strings.Builder
	sb.WriteString(topLine + "\n")

	// 内容行: │ (参数) │
	if block.ToolArgs != "" {
		argsStr := block.ToolArgs
		runes := []rune(argsStr)
		maxArgsWidth := contentWidth - 4
		if len(runes) > maxArgsWidth {
			argsStr = string(runes[:maxArgsWidth-3]) + "..."
		}
		argsDisplay := toolArgsStyle.Render(argsStr)
		argsPad := contentWidth - lipgloss.Width(argsStr) - 2
		if argsPad < 0 {
			argsPad = 0
		}
		sb.WriteString(pipe + " " + argsDisplay + strings.Repeat(" ", argsPad) + " " + pipe + "\n")
	}

	// 底行
	bottomLine := cornerBL + strings.Repeat(border, contentWidth-1) + cornerBR
	sb.WriteString(bottomLine)

	return sb.String()
}

// renderToolResultBlock 渲染工具结果 (Qoder 风格: 缩进灰色)
func (m *Model) renderToolResultBlock(block OutputBlock) string {
	content := block.Content
	runes := []rune(content)
	if len(runes) > 500 {
		content = string(runes[:500]) + "\n  ... (truncated)"
	}

	lines := strings.Split(content, "\n")
	var sb strings.Builder

	for i, line := range lines {
		sb.WriteString(toolResultStyle.Render("  ⎿  " + line))
		if i < len(lines)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// renderErrorBlock 渲染错误
func (m *Model) renderErrorBlock(block OutputBlock) string {
	icon := errorStyle.Render("  ✗")
	msg := block.Content
	if block.ToolName != "" {
		msg = translateToolName(block.ToolName) + ": " + msg
	}
	return icon + " " + errorStyle.Render(msg)
}

// renderInfoBlock 渲染信息
func (m *Model) renderInfoBlock(content string) string {
	return infoStyle.Render("  " + content)
}

// renderUsageBlock 渲染用量
func (m *Model) renderUsageBlock(content string) string {
	return usageStyle.Render("  " + content)
}

// ─── 辅助函数 ────────────────────────────────────────────────────────────

func (m *Model) scrollToBottom() {
	m.scrollOffset = 0
}

// AddBlock 添加输出块
func (m *Model) AddBlock(blockType string, content string) {
	bt := BlockType(blockType)
	m.blocks = append(m.blocks, OutputBlock{Type: bt, Content: content})
}

// SetWorking 设置工作状态
func (m *Model) SetWorking(w bool) { m.working = w }

// ClearBlocks 清空输出
func (m *Model) ClearBlocks() {
	m.blocks = nil
	m.scrollOffset = 0
}

// SetModel 设置模型名
func (m *Model) SetModel(name string) { m.model = name }

// SetVersion 设置版本号
func (m *Model) SetVersion(v string) { m.version = v }

// waitForAgent 等待代理响应
func waitForAgent(ch <-chan AgentResponse) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

// formatToolCall 格式化工具调用显示
func formatToolCall(name, args string) string {
	if args == "" {
		return translateToolName(name)
	}
	return translateToolName(name) + " " + args
}

// translateToolName 翻译工具名为中文
func translateToolName(name string) string {
	translations := map[string]string{
		"read_file":     "读取文件",
		"write_file":    "写入文件",
		"edit_file":     "编辑文件",
		"bash":          "执行命令",
		"bash_bg":       "后台执行",
		"bash_output":   "查看输出",
		"bash_kill":     "终止进程",
		"list_dir":      "列出目录",
		"glob_find":     "查找文件",
		"grep_search":   "搜索内容",
		"git":           "Git操作",
		"web_fetch":     "抓取网页",
		"web_search":    "搜索网页",
		"todo_write":    "任务管理",
		"sub_agent":     "子代理",
		"notebook_edit": "编辑Notebook",
	}
	if cn, ok := translations[name]; ok {
		return cn
	}
	return name
}

// SetAgentMode 设置代理模式
func (m *Model) SetAgentMode(mode string) { m.agentMode = mode }

// GetAgentMode 获取代理模式
func (m *Model) GetAgentMode() string { return m.agentMode }

// SetVimMode 切换vim模式
func (m *Model) SetVimMode(on bool) { m.vimMode = on }

// IsVimMode 检查vim模式
func (m *Model) IsVimMode() bool { return m.vimMode }

// SetPermMode 设置权限模式
func (m *Model) SetPermMode(mode PermissionMode) { m.permMode = mode }

// GetPermMode 获取权限模式
func (m *Model) GetPermMode() PermissionMode { return m.permMode }

// ─── 启动横幅 ────────────────────────────────────────────────────────────

func PrintBanner(version string) {
	icon := headerStyle.Render("⚡")
	title := headerStyle.Render(" 太极 Code")
	ver := headerVersionStyle.Render(" v" + version)
	dim := lipgloss.NewStyle().Foreground(secondaryText)

	fmt.Println()
	fmt.Println(icon + title + ver)
	fmt.Println(dim.Render(" ─────────────────────────────────"))
	fmt.Println(dim.Render(" AI 编程助手 | 输入 /help 查看帮助"))
	fmt.Println()
}
