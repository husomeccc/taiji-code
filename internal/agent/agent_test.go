package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"taiji-code/types"
)

// ===========================================================================
// truncate tests
// ===========================================================================

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "ASCII shorter than limit",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "ASCII exactly at limit",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "ASCII longer than limit",
			input:  "hello world",
			maxLen: 5,
			want:   "hello...",
		},
		{
			name:   "Chinese string truncated to 4 runes",
			input:  "你好世界测试",
			maxLen: 4,
			want:   "你好世界...",
		},
		{
			name:   "Chinese string within limit",
			input:  "你好",
			maxLen: 4,
			want:   "你好",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 10,
			want:   "",
		},
		{
			name:   "newlines replaced with spaces before truncation",
			input:  "hello\nworld\ntest",
			maxLen: 8,
			want:   "hello wo...",
		},
		{
			name:   "newlines replaced even when no truncation needed",
			input:  "a\nb",
			maxLen: 10,
			want:   "a b",
		},
		{
			name:   "only newlines",
			input:  "\n\n\n",
			maxLen: 2,
			want:   "  ...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// ===========================================================================
// estimateTokens tests
// ===========================================================================

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name    string
		byteLen int
		want    int
	}{
		{"0 bytes returns 0", 0, 0},
		{"negative returns 0", -10, 0},
		{"3 bytes returns 1", 3, 1},
		{"300 bytes returns 100", 300, 100},
		{"1 byte returns 1 (minimum)", 1, 1},
		{"2 bytes returns 1 (minimum)", 2, 1},
		{"6 bytes returns 2", 6, 2},
		{"1000 bytes returns 333", 1000, 333},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.byteLen)
			if got != tt.want {
				t.Errorf("estimateTokens(%d) = %d, want %d", tt.byteLen, got, tt.want)
			}
		})
	}
}

// ===========================================================================
// systemPromptLen tests
// ===========================================================================

func TestSystemPromptLen(t *testing.T) {
	t.Run("empty messages", func(t *testing.T) {
		got := systemPromptLen(nil)
		if got != 0 {
			t.Errorf("systemPromptLen(nil) = %d, want 0", got)
		}
	})

	t.Run("only system message", func(t *testing.T) {
		msgs := []types.Message{
			{Role: types.RoleSystem, Content: "hello system"},
		}
		got := systemPromptLen(msgs)
		// The function sums system + user + assistant content lengths.
		// With only a system message of len 12, the result is 12.
		want := len("hello system")
		if got != want {
			t.Errorf("systemPromptLen([system]) = %d, want %d", got, want)
		}
	})

	t.Run("mixed messages sums system user and assistant", func(t *testing.T) {
		msgs := []types.Message{
			{Role: types.RoleSystem, Content: "sys"},     // 3
			{Role: types.RoleUser, Content: "user msg"},  // 8
			{Role: types.RoleAssistant, Content: "resp"}, // 4
			{Role: types.RoleTool, Content: "tool data"}, // not counted
		}
		got := systemPromptLen(msgs)
		want := 3 + 8 + 4 // tool content is not counted
		if got != want {
			t.Errorf("systemPromptLen(mixed) = %d, want %d", got, want)
		}
	})

	t.Run("multiple system messages", func(t *testing.T) {
		msgs := []types.Message{
			{Role: types.RoleSystem, Content: "aaa"},
			{Role: types.RoleSystem, Content: "bb"},
			{Role: types.RoleUser, Content: "c"},
		}
		got := systemPromptLen(msgs)
		want := 3 + 2 + 1
		if got != want {
			t.Errorf("systemPromptLen(multi-system) = %d, want %d", got, want)
		}
	})
}

// ===========================================================================
// CostTracker tests
// ===========================================================================

func TestCostTracker_EstimateCost(t *testing.T) {
	tests := []struct {
		name  string
		ct    CostTracker
		want  float64
		tol   float64
	}{
		{
			name: "zero tokens",
			ct:   CostTracker{},
			want: 0.0,
			tol:  1e-9,
		},
		{
			name: "known input and output",
			ct:   CostTracker{InputTokens: 1000, OutputTokens: 500},
			// 1000 * 1.0 / 1e6 + 500 * 2.0 / 1e6 = 0.001 + 0.001 = 0.002
			want: 0.002,
			tol:  1e-9,
		},
		{
			name: "large token counts",
			ct:   CostTracker{InputTokens: 5_000_000, OutputTokens: 3_000_000},
			// 5e6 * 1.0 / 1e6 + 3e6 * 2.0 / 1e6 = 5 + 6 = 11
			want: 11.0,
			tol:  1e-9,
		},
		{
			name: "only input tokens",
			ct:   CostTracker{InputTokens: 2_000_000},
			want: 2.0,
			tol:  1e-9,
		},
		{
			name: "only output tokens",
			ct:   CostTracker{OutputTokens: 1_000_000},
			want: 2.0,
			tol:  1e-9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ct.EstimateCost()
			diff := got - tt.want
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.tol {
				t.Errorf("EstimateCost() = %f, want %f (tolerance %f)", got, tt.want, tt.tol)
			}
		})
	}
}

func TestCostTracker_FormatCost(t *testing.T) {
	t.Run("small cost uses 4 decimal places", func(t *testing.T) {
		ct := CostTracker{InputTokens: 100, OutputTokens: 50}
		got := ct.FormatCost()
		// cost = 100/1e6 + 50*2/1e6 = 0.0001 + 0.0001 = 0.0002
		// 0.0002 < 0.01 so format is ¥%.4f
		if !strings.HasPrefix(got, "¥") {
			t.Errorf("FormatCost() = %q, expected prefix ¥", got)
		}
		// Should have 4 decimal places
		parts := strings.Split(got, ".")
		if len(parts) != 2 || len(parts[1]) != 4 {
			t.Errorf("FormatCost() = %q, expected 4 decimal places", got)
		}
	})

	t.Run("large cost uses 2 decimal places", func(t *testing.T) {
		ct := CostTracker{InputTokens: 5_000_000, OutputTokens: 3_000_000}
		got := ct.FormatCost()
		// cost = 11.00, >= 0.01 so format is ¥%.2f
		want := "¥11.00"
		if got != want {
			t.Errorf("FormatCost() = %q, want %q", got, want)
		}
	})

	t.Run("zero cost", func(t *testing.T) {
		ct := CostTracker{}
		got := ct.FormatCost()
		want := "¥0.0000"
		if got != want {
			t.Errorf("FormatCost() = %q, want %q", got, want)
		}
	})
}

func TestCostTracker_FormatUsage(t *testing.T) {
	ct := CostTracker{
		InputTokens:  1000,
		OutputTokens: 500,
		TotalTokens:  1500,
		Requests:     3,
		ToolCalls:    2,
	}
	got := ct.FormatUsage()

	// Verify all expected components are present
	checks := []string{
		"1000 输入",
		"500 输出",
		"1500 总计",
		"请求: 3",
		"工具调用: 2",
		"费用:",
	}
	for _, check := range checks {
		if !strings.Contains(got, check) {
			t.Errorf("FormatUsage() = %q, missing %q", got, check)
		}
	}

	// Verify the overall format starts with "Token:"
	if !strings.HasPrefix(got, "Token:") {
		t.Errorf("FormatUsage() = %q, expected prefix 'Token:'", got)
	}
}

// ===========================================================================
// Compact tests
// ===========================================================================

// validateToolPairs checks that no tool result is orphaned (i.e., every
// RoleTool message is preceded by a RoleAssistant message that contains
// a ToolCall with matching ID).
func validateToolPairs(t *testing.T, msgs []types.Message) {
	t.Helper()
	for i, msg := range msgs {
		if msg.Role != types.RoleTool {
			continue
		}
		if i == 0 {
			t.Errorf("message[%d] is tool result %q but has no preceding assistant", i, msg.ToolCallID)
			continue
		}
		prev := msgs[i-1]
		if prev.Role != types.RoleAssistant || len(prev.ToolCalls) == 0 {
			t.Errorf("message[%d] is tool result %q but preceding message is %s (no tool calls)",
				i, msg.ToolCallID, prev.Role)
			continue
		}
		found := false
		for _, tc := range prev.ToolCalls {
			if tc.ID == msg.ToolCallID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("message[%d] is tool result %q but no matching tool call in preceding assistant",
				i, msg.ToolCallID)
		}
	}
}

func TestCompact_PreservesToolCallResultPairs(t *testing.T) {
	// Build a conversation with interleaved tool call/result pairs:
	//  [0] system
	//  [1] user
	//  [2] assistant (tool_call_1)
	//  [3] tool result_1
	//  [4] user
	//  [5] assistant (tool_call_2)
	//  [6] tool result_2
	//  [7] user
	//  [8] assistant (tool_call_3)
	//  [9] tool result_3
	//  [10] user
	//  [11] assistant (final)
	messages := []types.Message{
		{Role: types.RoleSystem, Content: "You are a helpful assistant."},
		{Role: types.RoleUser, Content: "Please help me with a task"},
		{Role: types.RoleAssistant, Content: "I'll help with that.",
			ToolCalls: []types.ToolCall{
				{ID: "call_1", Type: "function", Function: types.FunctionCall{Name: "read_file", Arguments: `{"path":"main.go"}`}},
			}},
		{Role: types.RoleTool, Content: "file contents here", ToolCallID: "call_1", Name: "read_file"},
		{Role: types.RoleUser, Content: "Now search for something"},
		{Role: types.RoleAssistant, Content: "Let me search.",
			ToolCalls: []types.ToolCall{
				{ID: "call_2", Type: "function", Function: types.FunctionCall{Name: "grep_search", Arguments: `{"pattern":"func"}`}},
			}},
		{Role: types.RoleTool, Content: "search results here", ToolCallID: "call_2", Name: "grep_search"},
		{Role: types.RoleUser, Content: "Edit the file please"},
		{Role: types.RoleAssistant, Content: "Making the edit.",
			ToolCalls: []types.ToolCall{
				{ID: "call_3", Type: "function", Function: types.FunctionCall{Name: "edit_file", Arguments: `{"path":"main.go"}`}},
			}},
		{Role: types.RoleTool, Content: "edit applied", ToolCallID: "call_3", Name: "edit_file"},
		{Role: types.RoleUser, Content: "Thanks!"},
		{Role: types.RoleAssistant, Content: "You're welcome!"},
	}

	ag := &Agent{
		messages:    make([]types.Message, len(messages)),
		autoCompact: 0,
	}
	copy(ag.messages, messages)

	result := ag.Compact()
	if result == "" {
		t.Fatal("Compact() returned empty string")
	}

	// Validate tool call/result pairing
	validateToolPairs(t, ag.messages)

	// Compact should reduce message count (12 -> fewer)
	if len(ag.messages) >= len(messages) {
		t.Errorf("Compact() did not reduce message count: before=%d, after=%d",
			len(messages), len(ag.messages))
	}

	// First message should still be system
	if len(ag.messages) == 0 || ag.messages[0].Role != types.RoleSystem {
		t.Error("Compact() removed the system message")
	}
}

func TestCompact_ShortConversation(t *testing.T) {
	// With <= 4 messages, Compact should return early
	ag := &Agent{
		messages: []types.Message{
			{Role: types.RoleSystem, Content: "system"},
			{Role: types.RoleUser, Content: "hello"},
			{Role: types.RoleAssistant, Content: "hi"},
		},
	}
	result := ag.Compact()
	if !strings.Contains(result, "已经很短") {
		t.Errorf("Compact() on short conversation = %q, expected 'already short' message", result)
	}
	if len(ag.messages) != 3 {
		t.Errorf("Compact() changed message count for short conversation: got %d, want 3", len(ag.messages))
	}
}

func TestCompact_PreservesRecentToolPair(t *testing.T) {
	// Build a conversation where the cutoff might fall between a tool call
	// and its result. Compact should shift the cutoff to keep the pair.
	//
	// 8 messages:
	//  [0] system
	//  [1] user
	//  [2] assistant (tool_call_A)
	//  [3] tool result_A
	//  [4] user
	//  [5] assistant (tool_call_B)
	//  [6] tool result_B
	//  [7] user
	//
	// cutoff = 8 - 6 = 2 (tool result_A) -> scan shifts to 3
	// recent = [3..7] = tool_A, user, assistant_B, tool_B, user
	messages := []types.Message{
		{Role: types.RoleSystem, Content: "System prompt"},
		{Role: types.RoleUser, Content: "First question"},
		{Role: types.RoleAssistant, Content: "",
			ToolCalls: []types.ToolCall{
				{ID: "call_A", Type: "function", Function: types.FunctionCall{Name: "bash", Arguments: `{"command":"ls"}`}},
			}},
		{Role: types.RoleTool, Content: "file1.go\nfile2.go", ToolCallID: "call_A", Name: "bash"},
		{Role: types.RoleUser, Content: "Tell me more"},
		{Role: types.RoleAssistant, Content: "",
			ToolCalls: []types.ToolCall{
				{ID: "call_B", Type: "function", Function: types.FunctionCall{Name: "read_file", Arguments: `{"path":"file1.go"}`}},
			}},
		{Role: types.RoleTool, Content: "package main", ToolCallID: "call_B", Name: "read_file"},
		{Role: types.RoleUser, Content: "Summarize please"},
	}

	ag := &Agent{
		messages: make([]types.Message, len(messages)),
	}
	copy(ag.messages, messages)

	ag.Compact()
	validateToolPairs(t, ag.messages)

	// The first message should be system
	if ag.messages[0].Role != types.RoleSystem {
		t.Error("First message should be system after Compact")
	}

	// There should be a summary user message after system
	if len(ag.messages) < 2 || ag.messages[1].Role != types.RoleUser {
		t.Error("Expected summary user message after system")
	}
}

// ===========================================================================
// SubAgentPool tests
// ===========================================================================

func TestNewSubAgentPool(t *testing.T) {
	pool := NewSubAgentPool(nil, nil, nil, "/tmp/test", 4096)

	if pool == nil {
		t.Fatal("NewSubAgentPool() returned nil")
	}
	if pool.tasks == nil {
		t.Error("tasks map should be initialized (non-nil)")
	}
	if len(pool.tasks) != 0 {
		t.Errorf("tasks map should be empty, got %d entries", len(pool.tasks))
	}
	if pool.workDir != "/tmp/test" {
		t.Errorf("workDir = %q, want %q", pool.workDir, "/tmp/test")
	}
	if pool.maxTokens != 4096 {
		t.Errorf("maxTokens = %d, want %d", pool.maxTokens, 4096)
	}
}

func TestSubAgentPool_SpawnAndGetTask(t *testing.T) {
	pool := NewSubAgentPool(nil, nil, nil, "/tmp/test", 4096)

	task := pool.SpawnTask("task-1", "Analyze code", "Please analyze the main.go file")
	if task == nil {
		t.Fatal("SpawnTask() returned nil")
	}
	if task.ID != "task-1" {
		t.Errorf("task.ID = %q, want %q", task.ID, "task-1")
	}
	if task.Description != "Analyze code" {
		t.Errorf("task.Description = %q, want %q", task.Description, "Analyze code")
	}
	if task.Status != "pending" {
		t.Errorf("task.Status = %q, want %q", task.Status, "pending")
	}

	// GetTask should find it
	got, ok := pool.GetTask("task-1")
	if !ok {
		t.Fatal("GetTask('task-1') returned false")
	}
	if got.ID != "task-1" {
		t.Errorf("GetTask().ID = %q, want %q", got.ID, "task-1")
	}

	// GetTask for non-existent ID
	_, ok = pool.GetTask("nonexistent")
	if ok {
		t.Error("GetTask('nonexistent') should return false")
	}
}

func TestSubAgentPool_GetAllTasks(t *testing.T) {
	pool := NewSubAgentPool(nil, nil, nil, "/tmp/test", 4096)

	pool.SpawnTask("a", "Task A", "Do A")
	pool.SpawnTask("b", "Task B", "Do B")
	pool.SpawnTask("c", "Task C", "Do C")

	all := pool.GetAllTasks()
	if len(all) != 3 {
		t.Errorf("GetAllTasks() returned %d tasks, want 3", len(all))
	}
}

func TestFormatTaskResults(t *testing.T) {
	tasks := []*SubAgentTask{
		{ID: "t1", Description: "Success task", Status: "done", Result: "All good"},
		{ID: "t2", Description: "Failed task", Status: "failed", Error: fmt.Errorf("something broke")},
	}

	got := FormatTaskResults(tasks)

	// Check success marker
	if !strings.Contains(got, "[t1]") {
		t.Error("FormatTaskResults() missing [t1]")
	}
	if !strings.Contains(got, "Success task") {
		t.Error("FormatTaskResults() missing 'Success task'")
	}
	if !strings.Contains(got, "All good") {
		t.Error("FormatTaskResults() missing result 'All good'")
	}

	// Check failure marker
	if !strings.Contains(got, "[t2]") {
		t.Error("FormatTaskResults() missing [t2]")
	}
	if !strings.Contains(got, "something broke") {
		t.Error("FormatTaskResults() missing error message")
	}
}

// ===========================================================================
// HookManager tests
// ===========================================================================

func TestNewHookManager(t *testing.T) {
	workDir := "testworkdir"
	mgr := NewHookManager(workDir)
	if mgr == nil {
		t.Fatal("NewHookManager() returned nil")
	}
	if mgr.hooks == nil {
		t.Error("hooks map should be initialized")
	}
	if mgr.workDir != workDir {
		t.Errorf("workDir = %q, want %q", mgr.workDir, workDir)
	}
}

func TestHookManager_HasHooks_Empty(t *testing.T) {
	mgr := NewHookManager("/tmp/test")

	events := []HookEvent{
		HookPreToolUse, HookPostToolUse, HookSessionStart, HookSessionEnd,
		HookStop, HookPreCompact, HookPostCompact, HookUserPrompt, HookNotification,
	}
	for _, event := range events {
		if mgr.HasHooks(event) {
			t.Errorf("HasHooks(%s) = true on empty manager, want false", event)
		}
	}
}

func TestHookManager_AddHook_And_HasHooks(t *testing.T) {
	mgr := NewHookManager("/tmp/test")

	mgr.AddHook(HookConfig{Event: HookPreToolUse, Command: "echo pre"})

	if !mgr.HasHooks(HookPreToolUse) {
		t.Error("HasHooks(HookPreToolUse) = false after adding, want true")
	}
	if mgr.HasHooks(HookPostToolUse) {
		t.Error("HasHooks(HookPostToolUse) = true, want false (not added)")
	}
}

func TestHookManager_MultipleHooksSameEvent(t *testing.T) {
	mgr := NewHookManager("/tmp/test")

	mgr.AddHook(HookConfig{Event: HookPreToolUse, Command: "echo first"})
	mgr.AddHook(HookConfig{Event: HookPreToolUse, Command: "echo second"})

	hooks := mgr.hooks[HookPreToolUse]
	if len(hooks) != 2 {
		t.Errorf("expected 2 hooks for PreToolUse, got %d", len(hooks))
	}
}

func TestHookManager_ListHooks_Empty(t *testing.T) {
	mgr := NewHookManager("/tmp/test")
	got := mgr.ListHooks()
	if !strings.Contains(got, "未注册任何Hook") {
		t.Errorf("ListHooks() on empty = %q, expected '未注册任何Hook'", got)
	}
}

func TestHookManager_ListHooks_WithHooks(t *testing.T) {
	mgr := NewHookManager("/tmp/test")
	mgr.AddHook(HookConfig{Event: HookPreToolUse, Command: "echo pre-tool"})
	mgr.AddHook(HookConfig{Event: HookPostToolUse, Command: "echo post-tool"})
	mgr.AddHook(HookConfig{Event: HookSessionStart, Command: "echo start"})

	got := mgr.ListHooks()

	if !strings.Contains(got, "PreToolUse") {
		t.Errorf("ListHooks() missing PreToolUse: %q", got)
	}
	if !strings.Contains(got, "PostToolUse") {
		t.Errorf("ListHooks() missing PostToolUse: %q", got)
	}
	if !strings.Contains(got, "SessionStart") {
		t.Errorf("ListHooks() missing SessionStart: %q", got)
	}
	if !strings.Contains(got, "echo pre-tool") {
		t.Errorf("ListHooks() missing command 'echo pre-tool': %q", got)
	}
}

func TestHookManager_Fire_NoHooks(t *testing.T) {
	mgr := NewHookManager("/tmp/test")
	ctx := context.Background()

	results := mgr.Fire(ctx, HookPreToolUse, map[string]interface{}{"tool_name": "bash"})
	if len(results) != 0 {
		t.Errorf("Fire() on empty manager returned %d results, want 0", len(results))
	}
}

func TestHookManager_Fire_WithHook(t *testing.T) {
	mgr := NewHookManager("/tmp/test")
	mgr.AddHook(HookConfig{Event: HookPreToolUse, Command: "echo check"})
	ctx := context.Background()

	results := mgr.Fire(ctx, HookPreToolUse, map[string]interface{}{
		"tool_name": "bash",
		"arguments": `{"command":"ls"}`,
	})
	if len(results) != 1 {
		t.Fatalf("Fire() returned %d results, want 1", len(results))
	}
	if results[0].Event != HookPreToolUse {
		t.Errorf("result.Event = %q, want %q", results[0].Event, HookPreToolUse)
	}
}

// ===========================================================================
// SkillManager tests
// ===========================================================================

func TestNewSkillManager(t *testing.T) {
	projectDir := "testproject"
	mgr := NewSkillManager(projectDir)
	if mgr == nil {
		t.Fatal("NewSkillManager() returned nil")
	}
	if mgr.skills == nil {
		t.Error("skills map should be initialized")
	}
	expectedDir := filepath.Join(projectDir, ".taiji", "skills")
	if mgr.skillsDir != expectedDir {
		t.Errorf("skillsDir = %q, want %q", mgr.skillsDir, expectedDir)
	}
}

func TestSkillManager_AllSkillContent_Empty(t *testing.T) {
	mgr := NewSkillManager("/tmp/test")
	got := mgr.AllSkillContent()
	if got != "" {
		t.Errorf("AllSkillContent() on empty = %q, want empty string", got)
	}
}

func TestSkillManager_ListSkills_Empty(t *testing.T) {
	mgr := NewSkillManager("/tmp/test")
	got := mgr.ListSkills()
	if !strings.Contains(got, "未安装任何技能") {
		t.Errorf("ListSkills() on empty = %q, expected '未安装任何技能'", got)
	}
}

func TestSkillManager_GetSkill_NotFound(t *testing.T) {
	mgr := NewSkillManager("/tmp/test")
	_, ok := mgr.GetSkill("nonexistent")
	if ok {
		t.Error("GetSkill('nonexistent') returned true, want false")
	}
}

func TestSkillManager_GetSkillContent_NotFound(t *testing.T) {
	mgr := NewSkillManager("/tmp/test")
	got := mgr.GetSkillContent("nonexistent")
	if got != "" {
		t.Errorf("GetSkillContent('nonexistent') = %q, want empty", got)
	}
}

func TestSkillManager_WithManualSkill(t *testing.T) {
	mgr := NewSkillManager("/tmp/test")

	// Manually inject a skill (simulating what LoadSkills would do)
	mgr.skills["test-skill"] = &Skill{
		Name:        "test-skill",
		Description: "A test skill for unit testing",
		Content:     "# Test Skill\nThis is a test skill content.",
		Path:        "/tmp/test/.taiji/skills/test-skill/SKILL.md",
	}

	// GetSkill should find it
	skill, ok := mgr.GetSkill("test-skill")
	if !ok {
		t.Fatal("GetSkill('test-skill') returned false")
	}
	if skill.Name != "test-skill" {
		t.Errorf("skill.Name = %q, want %q", skill.Name, "test-skill")
	}

	// GetSkillContent should return the content
	content := mgr.GetSkillContent("test-skill")
	if content != "# Test Skill\nThis is a test skill content." {
		t.Errorf("GetSkillContent() = %q, unexpected content", content)
	}

	// AllSkillContent should wrap in XML tags
	allContent := mgr.AllSkillContent()
	if !strings.Contains(allContent, "<available_skills>") {
		t.Error("AllSkillContent() missing <available_skills> tag")
	}
	if !strings.Contains(allContent, "</available_skills>") {
		t.Error("AllSkillContent() missing </available_skills> tag")
	}
	if !strings.Contains(allContent, `<skill name="test-skill">`) {
		t.Error("AllSkillContent() missing skill XML element")
	}
	if !strings.Contains(allContent, "This is a test skill content.") {
		t.Error("AllSkillContent() missing skill content body")
	}

	// ListSkills should show it
	listing := mgr.ListSkills()
	if !strings.Contains(listing, "test-skill") {
		t.Errorf("ListSkills() missing 'test-skill': %q", listing)
	}
}

// ===========================================================================
// Agent mode and output style tests
// ===========================================================================

func TestAgentMode_Constants(t *testing.T) {
	if ModeNormal != "normal" {
		t.Errorf("ModeNormal = %q, want %q", ModeNormal, "normal")
	}
	if ModePlan != "plan" {
		t.Errorf("ModePlan = %q, want %q", ModePlan, "plan")
	}
	if ModeAutoEdit != "auto_edit" {
		t.Errorf("ModeAutoEdit = %q, want %q", ModeAutoEdit, "auto_edit")
	}
}

// ===========================================================================
// HookEvent constants
// ===========================================================================

func TestHookEvent_Constants(t *testing.T) {
	events := map[HookEvent]string{
		HookPreToolUse:   "PreToolUse",
		HookPostToolUse:  "PostToolUse",
		HookSessionStart: "SessionStart",
		HookSessionEnd:   "SessionEnd",
		HookStop:         "Stop",
		HookPreCompact:   "PreCompact",
		HookPostCompact:  "PostCompact",
		HookUserPrompt:   "UserPromptSubmit",
		HookNotification: "Notification",
	}
	for event, want := range events {
		if string(event) != want {
			t.Errorf("HookEvent %v = %q, want %q", event, string(event), want)
		}
	}
}

// ===========================================================================
// GenerateID test
// ===========================================================================

func TestGenerateID(t *testing.T) {
	id := GenerateID()
	if !strings.HasPrefix(id, "session_") {
		t.Errorf("GenerateID() = %q, expected prefix 'session_'", id)
	}
	// Should be reasonably long (session_ + YYYYMMDD_HHMMSS = 8 + 15 = 23 chars)
	if len(id) < 20 {
		t.Errorf("GenerateID() = %q, seems too short", id)
	}

	// Two consecutive IDs should be valid (might be same if clock hasn't ticked)
	id2 := GenerateID()
	if id2 == "" {
		t.Error("GenerateID() returned empty string on second call")
	}
}

// ===========================================================================
// FormatSessionList test
// ===========================================================================

func TestFormatSessionList_Empty(t *testing.T) {
	got := FormatSessionList(nil)
	if !strings.Contains(got, "没有保存的会话") {
		t.Errorf("FormatSessionList(nil) = %q, expected '没有保存的会话'", got)
	}
}

// ===========================================================================
// SubAgentTool metadata tests
// ===========================================================================

func TestSubAgentTool_Metadata(t *testing.T) {
	tool := &SubAgentTool{Pool: nil}

	if tool.Name() != "sub_agent" {
		t.Errorf("SubAgentTool.Name() = %q, want %q", tool.Name(), "sub_agent")
	}
	if tool.Description() == "" {
		t.Error("SubAgentTool.Description() returned empty string")
	}
	if tool.PermissionLevel() != types.PermAskUser {
		t.Errorf("SubAgentTool.PermissionLevel() = %d, want %d", tool.PermissionLevel(), types.PermAskUser)
	}

	params := tool.Parameters()
	if params == nil {
		t.Fatal("SubAgentTool.Parameters() returned nil")
	}
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Parameters() missing 'properties' map")
	}
	for _, key := range []string{"task_id", "description", "prompt"} {
		if _, exists := props[key]; !exists {
			t.Errorf("Parameters() missing required property %q", key)
		}
	}
}
