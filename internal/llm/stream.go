package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"taiji-code/types"
)

// AccumulateStream reads all chunks and builds a complete message.
// onContent is called for each content piece (for real-time display).
func (c *Client) AccumulateStream(ctx context.Context, messages []types.Message, tools []types.ToolDefinition, maxTokens int, onContent func(string)) (*types.Message, error) {
	ch, errCh := c.StreamChat(ctx, messages, tools, maxTokens)

	var content strings.Builder
	toolCalls := make(map[int]*types.ToolCall) // indexed by tool call index
	var finishReason string

	for {
		select {
		case err, ok := <-errCh:
			if ok && err != nil {
				return nil, err
			}
			// errCh closed, drain remaining chunks
			for chunk := range ch {
				processStreamChunk(&chunk, &content, toolCalls, &finishReason, onContent)
			}
			return buildFinalMessage(content.String(), toolCalls, finishReason), nil

		case chunk, ok := <-ch:
			if !ok {
				return buildFinalMessage(content.String(), toolCalls, finishReason), nil
			}
			processStreamChunk(&chunk, &content, toolCalls, &finishReason, onContent)
		}
	}
}

func processStreamChunk(chunk *StreamChunk, content *strings.Builder, toolCalls map[int]*types.ToolCall, finishReason *string, onContent func(string)) {
	// Accumulate content
	if chunk.Delta.Content != "" {
		content.WriteString(chunk.Delta.Content)
		if onContent != nil {
			onContent(chunk.Delta.Content)
		}
	}

	// Accumulate tool calls using Index field
	for _, tc := range chunk.Delta.ToolCalls {
		idx := tc.Index

		if existing, ok := toolCalls[idx]; ok {
			// Continuation of existing tool call
			if tc.Function.Name != "" {
				existing.Function.Name += tc.Function.Name
			}
			existing.Function.Arguments += tc.Function.Arguments
			if tc.ID != "" {
				existing.ID = tc.ID
			}
		} else {
			// New tool call
			toolCalls[idx] = &types.ToolCall{
				Index: idx,
				ID:    tc.ID,
				Type:  "function",
				Function: types.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}

	if chunk.FinishReason != "" {
		*finishReason = chunk.FinishReason
	}
}

func buildFinalMessage(content string, toolCalls map[int]*types.ToolCall, finishReason string) *types.Message {
	msg := &types.Message{
		Role:    types.RoleAssistant,
		Content: content,
	}

	// Reconstruct tool calls in order
	if len(toolCalls) > 0 {
		maxIdx := 0
		for idx := range toolCalls {
			if idx > maxIdx {
				maxIdx = idx
			}
		}
		for i := 0; i <= maxIdx; i++ {
			if tc, ok := toolCalls[i]; ok {
				msg.ToolCalls = append(msg.ToolCalls, *tc)
			}
		}
	}

	if len(msg.ToolCalls) > 0 || finishReason == "tool_calls" {
		msg.StopReason = types.StopToolUse
	} else {
		msg.StopReason = types.StopEndTurn
	}

	return msg
}

// CountTokens estimates token count (rough: 1 token ≈ 4 chars for English, ~1.5 chars for Chinese)
func CountTokens(text string) int {
	chineseCount := 0
	asciiCount := 0
	for _, r := range text {
		if r > 127 {
			chineseCount++
		} else {
			asciiCount++
		}
	}
	return chineseCount + asciiCount/4
}

// FormatMessages formats messages for debugging
func FormatMessages(messages []types.Message) string {
	var sb strings.Builder
	for i, msg := range messages {
		sb.WriteString(fmt.Sprintf("[%d] %s: ", i, msg.Role))
		if msg.Content != "" {
			content := msg.Content
			runes := []rune(content)
			if len(runes) > 100 {
				content = string(runes[:100]) + "..."
			}
			sb.WriteString(content)
		}
		if len(msg.ToolCalls) > 0 {
			data, _ := json.Marshal(msg.ToolCalls)
			sb.WriteString(string(data))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
