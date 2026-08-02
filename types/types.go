package types

import "encoding/json"

// Role represents a message role
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// StopReason indicates why the model stopped generating
type StopReason string

const (
	StopEndTurn      StopReason = "end_turn"
	StopToolUse      StopReason = "tool_use"
	StopMaxTokens    StopReason = "max_tokens"
	StopModelOverriden StopReason = "model_overriden"
)

// Message represents a conversation message
type Message struct {
	Role       Role        `json:"role"`
	Content    string      `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	Name       string      `json:"name,omitempty"`
	StopReason StopReason  `json:"stop_reason,omitempty"`
}

// ToolCall represents a tool invocation request
type ToolCall struct {
	Index    int          `json:"index,omitempty"` // Used in streaming
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall represents the function name and arguments
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ParseArgs parses the JSON arguments into a map
func (fc *FunctionCall) ParseArgs() (map[string]interface{}, error) {
	var args map[string]interface{}
	err := json.Unmarshal([]byte(fc.Arguments), &args)
	return args, err
}

// ToolResult holds the output of a tool execution
type ToolResult struct {
	ToolCallID string
	Name       string
	Content    string
	IsError    bool
}

// ToolDefinition describes a tool for the LLM
type ToolDefinition struct {
	Type     string       `json:"type"` // "function"
	Function FunctionDef  `json:"function"`
}

// FunctionDef describes a function's schema
type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Usage tracks token consumption
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResponse is the full API response
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice is a response choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// StreamDelta is a chunk in SSE streaming
type StreamDelta struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []DeltaChoice `json:"choices"`
}

// DeltaChoice contains the incremental update
type DeltaChoice struct {
	Index        int          `json:"index"`
	Delta        DeltaMessage `json:"delta"`
	FinishReason *string      `json:"finish_reason,omitempty"`
}

// DeltaMessage is the incremental message content
type DeltaMessage struct {
	Role      Role       `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// PermissionLevel defines access control for tools
type PermissionLevel int

const (
	PermAutoApprove PermissionLevel = iota // Safe operations, no prompt
	PermAskUser                            // Need user confirmation
	PermDeny                               // Always denied
)
