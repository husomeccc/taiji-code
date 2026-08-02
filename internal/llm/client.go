package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"taiji-code/types"
	"time"
)

// Client communicates with the DeepSeek API (OpenAI-compatible)
type Client struct {
	apiKey      string
	baseURL     string
	model       string
	temperature float64
	http        *http.Client
}

// NewClient creates a new LLM client
func NewClient(apiKey, baseURL, model string) *Client {
	return &Client{
		apiKey:      apiKey,
		baseURL:     strings.TrimRight(baseURL, "/"),
		model:       model,
		temperature: 0.7,
		http:        &http.Client{Timeout: 120 * time.Second}, // 120秒超时
	}
}

// SetTemperature sets the sampling temperature
func (c *Client) SetTemperature(t float64) {
	c.temperature = t
}

// SetModel changes the model
func (c *Client) SetModel(model string) {
	c.model = model
}

// GetModel returns current model name
func (c *Client) GetModel() string {
	return c.model
}

// ChatRequest is the request body for chat completions
type ChatRequest struct {
	Model       string                 `json:"model"`
	Messages    []types.Message        `json:"messages"`
	Tools       []types.ToolDefinition `json:"tools,omitempty"`
	MaxTokens   int                    `json:"max_tokens,omitempty"`
	Temperature float64                `json:"temperature,omitempty"`
	Stream      bool                   `json:"stream"`
}

// StreamChunk is a parsed SSE chunk
type StreamChunk struct {
	Delta        types.DeltaMessage
	FinishReason string
}

// Chat sends a non-streaming request
func (c *Client) Chat(ctx context.Context, messages []types.Message, tools []types.ToolDefinition, maxTokens int) (*types.ChatResponse, error) {
	req := ChatRequest{
		Model:       c.model,
		Messages:    messages,
		Tools:       tools,
		MaxTokens:   maxTokens,
		Temperature: c.temperature,
		Stream:      false,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp types.ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &chatResp, nil
}

// StreamChat sends a streaming request and returns chunks via channel
func (c *Client) StreamChat(ctx context.Context, messages []types.Message, tools []types.ToolDefinition, maxTokens int) (<-chan StreamChunk, <-chan error) {
	ch := make(chan StreamChunk, 32)
	errCh := make(chan error, 1)

	req := ChatRequest{
		Model:       c.model,
		Messages:    messages,
		Tools:       tools,
		MaxTokens:   maxTokens,
		Temperature: c.temperature,
		Stream:      true,
	}

	body, err := json.Marshal(req)
	if err != nil {
		errCh <- fmt.Errorf("marshal request: %w", err)
		close(ch)
		return ch, errCh
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		errCh <- fmt.Errorf("create request: %w", err)
		close(ch)
		return ch, errCh
	}
	c.setHeaders(httpReq)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		errCh <- fmt.Errorf("send request: %w", err)
		close(ch)
		return ch, errCh
	}

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		errCh <- fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
		close(ch)
		return ch, errCh
	}

	go func() {
		defer close(ch)
		defer close(errCh)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		// Increase buffer size for large chunks
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()

			if line == "" {
				continue
			}

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			if data == "[DONE]" {
				return
			}

			var delta types.StreamDelta
			if err := json.Unmarshal([]byte(data), &delta); err != nil {
				// Skip unparseable chunks
				continue
			}

			chunk := StreamChunk{}
			if len(delta.Choices) > 0 {
				chunk.Delta = delta.Choices[0].Delta
				if delta.Choices[0].FinishReason != nil {
					chunk.FinishReason = *delta.Choices[0].FinishReason
				}
			}

			ch <- chunk
		}

		if err := scanner.Err(); err != nil {
			if !errors.Is(err, io.EOF) {
				errCh <- fmt.Errorf("read stream: %w", err)
			}
		}
	}()

	return ch, errCh
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}
