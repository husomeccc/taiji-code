package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// translateToolName
// ---------------------------------------------------------------------------

func TestTranslateToolName_KnownTools(t *testing.T) {
	cases := map[string]string{
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
	for input, want := range cases {
		got := translateToolName(input)
		if got != want {
			t.Errorf("translateToolName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTranslateToolName_UnknownTool(t *testing.T) {
	unknown := "some_custom_tool"
	got := translateToolName(unknown)
	if got != unknown {
		t.Errorf("translateToolName(%q) = %q, want %q (unchanged)", unknown, got, unknown)
	}
}

func TestTranslateToolName_EmptyString(t *testing.T) {
	got := translateToolName("")
	if got != "" {
		t.Errorf("translateToolName(\"\") = %q, want \"\"", got)
	}
}

// ---------------------------------------------------------------------------
// NewModel
// ---------------------------------------------------------------------------

func TestNewModel_DefaultValues(t *testing.T) {
	m := NewModel()
	if m == nil {
		t.Fatal("NewModel() returned nil")
	}
	if m.model != "deepseek-chat" {
		t.Errorf("default model = %q, want %q", m.model, "deepseek-chat")
	}
	if m.version != "0.3.0" {
		t.Errorf("default version = %q, want %q", m.version, "0.3.0")
	}
	if m.agentMode != "normal" {
		t.Errorf("default agentMode = %q, want %q", m.agentMode, "normal")
	}
	if m.permMode != PermDefault {
		t.Errorf("default permMode = %q, want %q", m.permMode, PermDefault)
	}
	if m.streamingBlock != -1 {
		t.Errorf("default streamingBlock = %d, want -1", m.streamingBlock)
	}
	if m.histIdx != -1 {
		t.Errorf("default histIdx = %d, want -1", m.histIdx)
	}
	if m.history == nil {
		t.Error("history should be initialized (non-nil)")
	}
	if len(m.history) != 0 {
		t.Errorf("history should be empty, got %d items", len(m.history))
	}
	if m.working {
		t.Error("default working should be false")
	}
}

// ---------------------------------------------------------------------------
// AddBlock
// ---------------------------------------------------------------------------

func TestAddBlock(t *testing.T) {
	m := NewModel()

	m.AddBlock("user", "hello")
	if len(m.blocks) != 1 {
		t.Fatalf("after AddBlock, len(blocks) = %d, want 1", len(m.blocks))
	}
	if m.blocks[0].Type != BlockUser {
		t.Errorf("block type = %q, want %q", m.blocks[0].Type, BlockUser)
	}
	if m.blocks[0].Content != "hello" {
		t.Errorf("block content = %q, want %q", m.blocks[0].Content, "hello")
	}

	m.AddBlock("assistant", "hi there")
	if len(m.blocks) != 2 {
		t.Fatalf("after second AddBlock, len(blocks) = %d, want 2", len(m.blocks))
	}
	if m.blocks[1].Type != BlockAssistant {
		t.Errorf("second block type = %q, want %q", m.blocks[1].Type, BlockAssistant)
	}
	if m.blocks[1].Content != "hi there" {
		t.Errorf("second block content = %q, want %q", m.blocks[1].Content, "hi there")
	}
}

func TestAddBlock_MultipleTypes(t *testing.T) {
	m := NewModel()
	types := []string{"user", "assistant", "tool_call", "tool_result", "error", "info"}
	for _, bt := range types {
		m.AddBlock(bt, "content-"+bt)
	}
	if len(m.blocks) != len(types) {
		t.Fatalf("len(blocks) = %d, want %d", len(m.blocks), len(types))
	}
	for i, bt := range types {
		if string(m.blocks[i].Type) != bt {
			t.Errorf("block[%d].Type = %q, want %q", i, m.blocks[i].Type, bt)
		}
		if m.blocks[i].Content != "content-"+bt {
			t.Errorf("block[%d].Content = %q, want %q", i, m.blocks[i].Content, "content-"+bt)
		}
	}
}

// ---------------------------------------------------------------------------
// BlockMsg struct fields
// ---------------------------------------------------------------------------

func TestBlockMsg_Fields(t *testing.T) {
	msg := BlockMsg{
		BlockType: "tool_call",
		Content:   "some content",
		ToolName:  "bash",
		ToolArgs:  "ls -la",
	}
	if msg.BlockType != "tool_call" {
		t.Errorf("BlockType = %q, want %q", msg.BlockType, "tool_call")
	}
	if msg.Content != "some content" {
		t.Errorf("Content = %q, want %q", msg.Content, "some content")
	}
	if msg.ToolName != "bash" {
		t.Errorf("ToolName = %q, want %q", msg.ToolName, "bash")
	}
	if msg.ToolArgs != "ls -la" {
		t.Errorf("ToolArgs = %q, want %q", msg.ToolArgs, "ls -la")
	}
}

func TestBlockMsg_ViaUpdate(t *testing.T) {
	m := NewModel()
	msg := BlockMsg{
		BlockType: "info",
		Content:   "test info",
		ToolName:  "",
		ToolArgs:  "",
	}
	m.Update(msg)
	if len(m.blocks) != 1 {
		t.Fatalf("after BlockMsg update, len(blocks) = %d, want 1", len(m.blocks))
	}
	if m.blocks[0].Type != BlockInfo {
		t.Errorf("block type = %q, want %q", m.blocks[0].Type, BlockInfo)
	}
	if m.blocks[0].Content != "test info" {
		t.Errorf("block content = %q, want %q", m.blocks[0].Content, "test info")
	}
	// scrollToBottom should have been called
	if m.scrollOffset != 0 {
		t.Errorf("scrollOffset = %d, want 0 after BlockMsg", m.scrollOffset)
	}
}

// ---------------------------------------------------------------------------
// StreamToken struct fields
// ---------------------------------------------------------------------------

func TestStreamToken_Fields(t *testing.T) {
	tok := StreamToken{
		Text: "hello",
		Done: false,
		Err:  nil,
	}
	if tok.Text != "hello" {
		t.Errorf("Text = %q, want %q", tok.Text, "hello")
	}
	if tok.Done {
		t.Error("Done should be false")
	}
	if tok.Err != nil {
		t.Error("Err should be nil")
	}
}

func TestStreamToken_Done(t *testing.T) {
	tok := StreamToken{
		Text: "",
		Done: true,
		Err:  nil,
	}
	if !tok.Done {
		t.Error("Done should be true")
	}
}

func TestStreamToken_ViaUpdate_Accumulates(t *testing.T) {
	m := NewModel()

	// Send first token (not done)
	m.Update(StreamToken{Text: "Hello", Done: false})
	if m.streamBuffer != "Hello" {
		t.Errorf("streamBuffer = %q, want %q", m.streamBuffer, "Hello")
	}

	// Send second token (not done)
	m.Update(StreamToken{Text: " World", Done: false})
	if m.streamBuffer != "Hello World" {
		t.Errorf("streamBuffer = %q, want %q", m.streamBuffer, "Hello World")
	}

	// Send done token
	m.Update(StreamToken{Text: "", Done: true})
	if m.streamBuffer != "" {
		t.Errorf("streamBuffer should be empty after Done, got %q", m.streamBuffer)
	}
	if m.streamingBlock != -1 {
		t.Errorf("streamingBlock should be -1 after Done, got %d", m.streamingBlock)
	}
	// A block should have been created
	if len(m.blocks) == 0 {
		t.Fatal("expected at least one block after stream completion")
	}
	if m.blocks[len(m.blocks)-1].Content != "Hello World" {
		t.Errorf("final block content = %q, want %q", m.blocks[len(m.blocks)-1].Content, "Hello World")
	}
}

// ---------------------------------------------------------------------------
// scrollToBottom
// ---------------------------------------------------------------------------

func TestScrollToBottom(t *testing.T) {
	m := NewModel()
	m.scrollOffset = 42
	m.scrollToBottom()
	if m.scrollOffset != 0 {
		t.Errorf("scrollOffset = %d, want 0 after scrollToBottom", m.scrollOffset)
	}
}

func TestScrollToBottom_AlreadyZero(t *testing.T) {
	m := NewModel()
	m.scrollOffset = 0
	m.scrollToBottom()
	if m.scrollOffset != 0 {
		t.Errorf("scrollOffset = %d, want 0", m.scrollOffset)
	}
}

// ---------------------------------------------------------------------------
// ClearBlocks
// ---------------------------------------------------------------------------

func TestClearBlocks(t *testing.T) {
	m := NewModel()
	m.AddBlock("user", "a")
	m.AddBlock("assistant", "b")
	m.scrollOffset = 15

	m.ClearBlocks()

	if len(m.blocks) != 0 {
		t.Errorf("len(blocks) = %d, want 0 after ClearBlocks", len(m.blocks))
	}
	if m.scrollOffset != 0 {
		t.Errorf("scrollOffset = %d, want 0 after ClearBlocks", m.scrollOffset)
	}
}

func TestClearBlocks_EmptyModel(t *testing.T) {
	m := NewModel()
	m.ClearBlocks() // should not panic
	if len(m.blocks) != 0 {
		t.Errorf("len(blocks) = %d, want 0", len(m.blocks))
	}
}

// ---------------------------------------------------------------------------
// SetVersion / SetModel / SetWorking
// ---------------------------------------------------------------------------

func TestSetVersion(t *testing.T) {
	m := NewModel()
	m.SetVersion("1.2.3")
	if m.version != "1.2.3" {
		t.Errorf("version = %q, want %q", m.version, "1.2.3")
	}
}

func TestSetModel(t *testing.T) {
	m := NewModel()
	m.SetModel("gpt-4")
	if m.model != "gpt-4" {
		t.Errorf("model = %q, want %q", m.model, "gpt-4")
	}
}

func TestSetWorking(t *testing.T) {
	m := NewModel()
	if m.working {
		t.Fatal("working should default to false")
	}
	m.SetWorking(true)
	if !m.working {
		t.Error("working should be true after SetWorking(true)")
	}
	m.SetWorking(false)
	if m.working {
		t.Error("working should be false after SetWorking(false)")
	}
}

// ---------------------------------------------------------------------------
// History navigation (up / down key presses via Update)
// ---------------------------------------------------------------------------

func TestHistoryNavigation(t *testing.T) {
	m := NewModel()

	// Populate history by simulating submissions.
	// We need to set up the model so submit() doesn't try to call AgentRunner.
	// Set input directly and call submit via enter key.
	type keyInput struct {
		keys []string // runes to type
	}

	// Helper: type a string then press enter
	submitInput := func(s string) {
		m.input = s
		m.cursor = len(s)
		// Simulate pressing enter
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	}

	submitInput("first")
	submitInput("second")
	submitInput("third")

	// After submissions, histIdx should be at len(history) = 3
	if m.histIdx != 3 {
		t.Fatalf("histIdx = %d, want 3", m.histIdx)
	}
	if len(m.history) != 3 {
		t.Fatalf("len(history) = %d, want 3", len(m.history))
	}

	// Press up: histIdx should go to 2, input = "third"
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.histIdx != 2 {
		t.Errorf("after up: histIdx = %d, want 2", m.histIdx)
	}
	if m.input != "third" {
		t.Errorf("after up: input = %q, want %q", m.input, "third")
	}

	// Press up again: histIdx = 1, input = "second"
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.histIdx != 1 {
		t.Errorf("after 2nd up: histIdx = %d, want 1", m.histIdx)
	}
	if m.input != "second" {
		t.Errorf("after 2nd up: input = %q, want %q", m.input, "second")
	}

	// Press up again: histIdx = 0, input = "first"
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.histIdx != 0 {
		t.Errorf("after 3rd up: histIdx = %d, want 0", m.histIdx)
	}
	if m.input != "first" {
		t.Errorf("after 3rd up: input = %q, want %q", m.input, "first")
	}

	// Press up at top: should stay at 0
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.histIdx != 0 {
		t.Errorf("after up at top: histIdx = %d, want 0", m.histIdx)
	}

	// Press down: histIdx = 1, input = "second"
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.histIdx != 1 {
		t.Errorf("after down: histIdx = %d, want 1", m.histIdx)
	}
	if m.input != "second" {
		t.Errorf("after down: input = %q, want %q", m.input, "second")
	}

	// Press down: histIdx = 2, input = "third"
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.histIdx != 2 {
		t.Errorf("after 2nd down: histIdx = %d, want 2", m.histIdx)
	}

	// Press down: histIdx = 3 (len(history)), input = ""
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.histIdx != 3 {
		t.Errorf("after 3rd down: histIdx = %d, want 3", m.histIdx)
	}
	if m.input != "" {
		t.Errorf("after 3rd down: input = %q, want empty", m.input)
	}

	// Press down past end: should stay at len(history) with empty input
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.histIdx != 3 {
		t.Errorf("after down past end: histIdx = %d, want 3", m.histIdx)
	}
}

// ---------------------------------------------------------------------------
// Tab completion state reset after submit
// ---------------------------------------------------------------------------

func TestTabCompletionResetAfterSubmit(t *testing.T) {
	m := NewModel()
	m.TabCompleter = func(input string) []string {
		return []string{"/help", "/history"}
	}

	// Simulate typing "/" then pressing tab to trigger completion
	m.input = "/"
	m.cursor = 1
	m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// tabCandidates should now be populated
	if len(m.tabCandidates) == 0 {
		t.Fatal("tabCandidates should be non-empty after tab press")
	}

	// Now submit the input
	m.input = m.tabCandidates[0]
	m.cursor = len(m.input)
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// After submit, tabCandidates should be nil
	if m.tabCandidates != nil {
		t.Errorf("tabCandidates should be nil after submit, got %v", m.tabCandidates)
	}
}

func TestTabCompletion_SingleCandidate(t *testing.T) {
	m := NewModel()
	m.TabCompleter = func(input string) []string {
		return []string{"/help"}
	}

	m.input = "/h"
	m.cursor = 2
	m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Single candidate should be applied directly
	if m.input != "/help" {
		t.Errorf("input = %q, want %q", m.input, "/help")
	}
	// tabCandidates should be nil for single candidate
	if m.tabCandidates != nil {
		t.Errorf("tabCandidates should be nil for single candidate, got %v", m.tabCandidates)
	}
}

// ---------------------------------------------------------------------------
// formatToolCall
// ---------------------------------------------------------------------------

func TestFormatToolCall_WithArgs(t *testing.T) {
	got := formatToolCall("bash", "ls -la")
	want := "执行命令 ls -la"
	if got != want {
		t.Errorf("formatToolCall(\"bash\", \"ls -la\") = %q, want %q", got, want)
	}
}

func TestFormatToolCall_NoArgs(t *testing.T) {
	got := formatToolCall("read_file", "")
	want := "读取文件"
	if got != want {
		t.Errorf("formatToolCall(\"read_file\", \"\") = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// AgentResponse via Update
// ---------------------------------------------------------------------------

func TestAgentResponse_Success(t *testing.T) {
	m := NewModel()
	m.working = true
	m.scrollOffset = 10

	m.Update(AgentResponse{Content: "I can help with that."})

	if m.working {
		t.Error("working should be false after AgentResponse")
	}
	if m.scrollOffset != 0 {
		t.Errorf("scrollOffset = %d, want 0 (auto-scroll)", m.scrollOffset)
	}
	found := false
	for _, b := range m.blocks {
		if b.Type == BlockAssistant && b.Content == "I can help with that." {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected an assistant block with the response content")
	}
}

func TestAgentResponse_Error(t *testing.T) {
	m := NewModel()
	m.working = true
	m.Update(AgentResponse{Err: fmt.Errorf("something went wrong")})

	if m.working {
		t.Error("working should be false after error response")
	}
	found := false
	for _, b := range m.blocks {
		if b.Type == BlockError && b.Content == "something went wrong" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected an error block")
	}
}

// ---------------------------------------------------------------------------
// WindowSizeMsg via Update
// ---------------------------------------------------------------------------

func TestWindowSizeMsg(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 120 {
		t.Errorf("width = %d, want 120", m.width)
	}
	if m.height != 40 {
		t.Errorf("height = %d, want 40", m.height)
	}
}

// ---------------------------------------------------------------------------
// Mode cycling via shift+tab
// ---------------------------------------------------------------------------

func TestModeCycling(t *testing.T) {
	m := NewModel()
	if m.GetAgentMode() != "normal" {
		t.Fatalf("initial mode = %q, want %q", m.GetAgentMode(), "normal")
	}

	// normal -> plan
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.GetAgentMode() != "plan" {
		t.Errorf("after shift+tab: mode = %q, want %q", m.GetAgentMode(), "plan")
	}

	// plan -> auto_edit
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.GetAgentMode() != "auto_edit" {
		t.Errorf("after 2nd shift+tab: mode = %q, want %q", m.GetAgentMode(), "auto_edit")
	}

	// auto_edit -> normal
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.GetAgentMode() != "normal" {
		t.Errorf("after 3rd shift+tab: mode = %q, want %q", m.GetAgentMode(), "normal")
	}
}

// ---------------------------------------------------------------------------
// PermissionMode constants
// ---------------------------------------------------------------------------

func TestPermissionModeConstants(t *testing.T) {
	if PermDefault != "default" {
		t.Errorf("PermDefault = %q, want %q", PermDefault, "default")
	}
	if PermAutoEdit != "auto_edit" {
		t.Errorf("PermAutoEdit = %q, want %q", PermAutoEdit, "auto_edit")
	}
	if PermPlanOnly != "plan_only" {
		t.Errorf("PermPlanOnly = %q, want %q", PermPlanOnly, "plan_only")
	}
	if PermBypass != "bypass" {
		t.Errorf("PermBypass = %q, want %q", PermBypass, "bypass")
	}
}
