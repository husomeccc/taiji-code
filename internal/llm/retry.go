package llm

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"time"
	"taiji-code/types"
)

// RetryConfig controls retry behavior
type RetryConfig struct {
	MaxRetries   int           // Maximum number of retries
	InitialDelay time.Duration // Initial backoff delay
	MaxDelay     time.Duration // Maximum backoff delay
	Multiplier   float64       // Backoff multiplier
}

// DefaultRetryConfig returns sensible retry defaults
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
}

// ChatWithRetry sends a chat request with retry logic
func (c *Client) ChatWithRetry(ctx context.Context, messages []types.Message, tools []types.ToolDefinition, maxTokens int, retryCfg RetryConfig) (*types.ChatResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= retryCfg.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := calculateBackoff(attempt, retryCfg)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := c.Chat(ctx, messages, tools, maxTokens)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Only retry on retryable errors
		if !isRetryableError(err) {
			return nil, err
		}
	}

	return nil, fmt.Errorf("请求失败(重试%d次): %w", retryCfg.MaxRetries, lastErr)
}

// AccumulateStreamWithRetry streams with retry on failure
func (c *Client) AccumulateStreamWithRetry(ctx context.Context, messages []types.Message, tools []types.ToolDefinition, maxTokens int, onContent func(string), retryCfg RetryConfig) (*types.Message, error) {
	var lastErr error

	for attempt := 0; attempt <= retryCfg.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := calculateBackoff(attempt, retryCfg)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		// 重试时不转发内容到UI，避免重复显示
		contentCb := onContent
		if attempt > 0 {
			contentCb = nil
		}

		msg, err := c.AccumulateStream(ctx, messages, tools, maxTokens, contentCb)
		if err == nil {
			return msg, nil
		}

		lastErr = err

		if !isRetryableError(err) {
			return nil, err
		}
	}

	return nil, fmt.Errorf("流式请求失败(重试%d次): %w", retryCfg.MaxRetries, lastErr)
}

// isRetryableError checks if an error is worth retrying
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Network errors
	if _, ok := err.(net.Error); ok {
		return true
	}

	// Timeout errors
	if isTimeout(err) {
		return true
	}

	// HTTP status based
	retryablePatterns := []string{
		"API error 429", // Rate limit
		"API error 500", // Internal server error
		"API error 502", // Bad gateway
		"API error 503", // Service unavailable
		"API error 504", // Gateway timeout
		"connection reset",
		"connection refused",
		"EOF",
		"timeout",
		"temporary failure",
	}

	for _, pattern := range retryablePatterns {
		if containsIgnoreCase(errStr, pattern) {
			return true
		}
	}

	return false
}

func isTimeout(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && len(substr) > 0 && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			sc := s[i+j]
			tc := substr[j]
			if tc >= 'A' && tc <= 'Z' {
				tc += 32
			}
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if sc != tc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// calculateBackoff computes exponential backoff with jitter
func calculateBackoff(attempt int, cfg RetryConfig) time.Duration {
	delay := float64(cfg.InitialDelay) * math.Pow(cfg.Multiplier, float64(attempt-1))
	if delay > float64(cfg.MaxDelay) {
		delay = float64(cfg.MaxDelay)
	}

	// Add jitter (±25%)
	jitter := delay * 0.25 * (rand.Float64()*2 - 1)
	delay += jitter

	return time.Duration(delay)
}
