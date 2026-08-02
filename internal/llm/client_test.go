package llm

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// NewClient
// ---------------------------------------------------------------------------

func TestNewClient_Fields(t *testing.T) {
	c := NewClient("test-key", "https://api.example.com/", "deepseek-chat")
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.apiKey != "test-key" {
		t.Errorf("apiKey = %q, want %q", c.apiKey, "test-key")
	}
	// Trailing slash should be trimmed
	if c.baseURL != "https://api.example.com" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "https://api.example.com")
	}
	if c.model != "deepseek-chat" {
		t.Errorf("model = %q, want %q", c.model, "deepseek-chat")
	}
	if c.http == nil {
		t.Error("http client should not be nil")
	}
}

func TestNewClient_DefaultTemperature(t *testing.T) {
	c := NewClient("key", "https://api.example.com", "model")
	if c.temperature != 0.7 {
		t.Errorf("default temperature = %f, want 0.7", c.temperature)
	}
}

func TestNewClient_BaseURLNoTrailingSlash(t *testing.T) {
	c := NewClient("key", "https://api.example.com", "model")
	if c.baseURL != "https://api.example.com" {
		t.Errorf("baseURL = %q, want no trailing slash", c.baseURL)
	}
}

func TestNewClient_BaseURLWithTrailingSlash(t *testing.T) {
	c := NewClient("key", "https://api.example.com///", "model")
	if c.baseURL != "https://api.example.com" {
		t.Errorf("baseURL = %q, want trailing slashes trimmed", c.baseURL)
	}
}

// ---------------------------------------------------------------------------
// SetTemperature
// ---------------------------------------------------------------------------

func TestSetTemperature(t *testing.T) {
	c := NewClient("key", "https://api.example.com", "model")
	c.SetTemperature(0.9)
	if c.temperature != 0.9 {
		t.Errorf("temperature = %f, want 0.9", c.temperature)
	}
}

func TestSetTemperature_Zero(t *testing.T) {
	c := NewClient("key", "https://api.example.com", "model")
	c.SetTemperature(0.0)
	if c.temperature != 0.0 {
		t.Errorf("temperature = %f, want 0.0", c.temperature)
	}
}

func TestSetTemperature_One(t *testing.T) {
	c := NewClient("key", "https://api.example.com", "model")
	c.SetTemperature(1.0)
	if c.temperature != 1.0 {
		t.Errorf("temperature = %f, want 1.0", c.temperature)
	}
}

// ---------------------------------------------------------------------------
// SetModel / GetModel
// ---------------------------------------------------------------------------

func TestSetModel_GetModel(t *testing.T) {
	c := NewClient("key", "https://api.example.com", "deepseek-chat")
	if c.GetModel() != "deepseek-chat" {
		t.Errorf("GetModel() = %q, want %q", c.GetModel(), "deepseek-chat")
	}

	c.SetModel("deepseek-coder")
	if c.GetModel() != "deepseek-coder" {
		t.Errorf("after SetModel: GetModel() = %q, want %q", c.GetModel(), "deepseek-coder")
	}
}

func TestSetModel_Empty(t *testing.T) {
	c := NewClient("key", "https://api.example.com", "model")
	c.SetModel("")
	if c.GetModel() != "" {
		t.Errorf("GetModel() = %q, want empty", c.GetModel())
	}
}

// ---------------------------------------------------------------------------
// ChatRequest JSON marshaling (temperature field)
// ---------------------------------------------------------------------------

func TestChatRequest_JSONIncludesTemperature(t *testing.T) {
	req := ChatRequest{
		Model:       "deepseek-chat",
		Temperature: 0.85,
		Stream:      false,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	tempVal, ok := parsed["temperature"]
	if !ok {
		t.Fatal("JSON output does not contain 'temperature' field")
	}
	tempFloat, ok := tempVal.(float64)
	if !ok {
		t.Fatalf("temperature is not a number: %T", tempVal)
	}
	if tempFloat != 0.85 {
		t.Errorf("temperature in JSON = %f, want 0.85", tempFloat)
	}
}

func TestChatRequest_JSONModelField(t *testing.T) {
	req := ChatRequest{
		Model:  "deepseek-coder",
		Stream: true,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	modelVal, ok := parsed["model"]
	if !ok {
		t.Fatal("JSON output does not contain 'model' field")
	}
	if modelVal != "deepseek-coder" {
		t.Errorf("model in JSON = %v, want %q", modelVal, "deepseek-coder")
	}
}

func TestChatRequest_StreamField(t *testing.T) {
	req := ChatRequest{
		Model:  "test",
		Stream: true,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)
	if parsed["stream"] != true {
		t.Errorf("stream in JSON = %v, want true", parsed["stream"])
	}
}

// ---------------------------------------------------------------------------
// DefaultRetryConfig
// ---------------------------------------------------------------------------

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.InitialDelay != 1*time.Second {
		t.Errorf("InitialDelay = %v, want 1s", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want 30s", cfg.MaxDelay)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("Multiplier = %f, want 2.0", cfg.Multiplier)
	}
}

// ---------------------------------------------------------------------------
// isRetryableError
// ---------------------------------------------------------------------------

func TestIsRetryableError_Nil(t *testing.T) {
	if isRetryableError(nil) {
		t.Error("isRetryableError(nil) should be false")
	}
}

func TestIsRetryableError_RateLimit(t *testing.T) {
	err := fmt.Errorf("API error 429: rate limit exceeded")
	if !isRetryableError(err) {
		t.Error("expected rate limit error (429) to be retryable")
	}
}

func TestIsRetryableError_InternalServerError(t *testing.T) {
	err := fmt.Errorf("API error 500: internal server error")
	if !isRetryableError(err) {
		t.Error("expected 500 error to be retryable")
	}
}

func TestIsRetryableError_BadGateway(t *testing.T) {
	err := fmt.Errorf("API error 502: bad gateway")
	if !isRetryableError(err) {
		t.Error("expected 502 error to be retryable")
	}
}

func TestIsRetryableError_ServiceUnavailable(t *testing.T) {
	err := fmt.Errorf("API error 503: service unavailable")
	if !isRetryableError(err) {
		t.Error("expected 503 error to be retryable")
	}
}

func TestIsRetryableError_GatewayTimeout(t *testing.T) {
	err := fmt.Errorf("API error 504: gateway timeout")
	if !isRetryableError(err) {
		t.Error("expected 504 error to be retryable")
	}
}

func TestIsRetryableError_ConnectionReset(t *testing.T) {
	err := fmt.Errorf("connection reset by peer")
	if !isRetryableError(err) {
		t.Error("expected 'connection reset' error to be retryable")
	}
}

func TestIsRetryableError_ConnectionRefused(t *testing.T) {
	err := fmt.Errorf("dial tcp 127.0.0.1:8080: connection refused")
	if !isRetryableError(err) {
		t.Error("expected 'connection refused' error to be retryable")
	}
}

func TestIsRetryableError_EOF(t *testing.T) {
	err := fmt.Errorf("unexpected EOF")
	if !isRetryableError(err) {
		t.Error("expected EOF error to be retryable")
	}
}

func TestIsRetryableError_Timeout(t *testing.T) {
	err := fmt.Errorf("request timeout exceeded")
	if !isRetryableError(err) {
		t.Error("expected 'timeout' error to be retryable")
	}
}

func TestIsRetryableError_TemporaryFailure(t *testing.T) {
	err := fmt.Errorf("temporary failure in name resolution")
	if !isRetryableError(err) {
		t.Error("expected 'temporary failure' error to be retryable")
	}
}

func TestIsRetryableError_NonRetryable(t *testing.T) {
	// 401 unauthorized should NOT be retryable
	err := fmt.Errorf("API error 401: unauthorized")
	if isRetryableError(err) {
		t.Error("expected 401 error to NOT be retryable")
	}
}

func TestIsRetryableError_403Forbidden(t *testing.T) {
	err := fmt.Errorf("API error 403: forbidden")
	if isRetryableError(err) {
		t.Error("expected 403 error to NOT be retryable")
	}
}

func TestIsRetryableError_404NotFound(t *testing.T) {
	err := fmt.Errorf("API error 404: not found")
	if isRetryableError(err) {
		t.Error("expected 404 error to NOT be retryable")
	}
}

func TestIsRetryableError_InvalidRequest(t *testing.T) {
	err := fmt.Errorf("invalid request body: missing model field")
	if isRetryableError(err) {
		t.Error("expected generic client error to NOT be retryable")
	}
}

// ---------------------------------------------------------------------------
// calculateBackoff
// ---------------------------------------------------------------------------

func TestCalculateBackoff_FirstAttempt(t *testing.T) {
	cfg := DefaultRetryConfig()
	delay := calculateBackoff(1, cfg)
	// First attempt: InitialDelay * Multiplier^0 = 1s, with +/-25% jitter
	// So delay should be between 0.75s and 1.25s
	if delay < 750*time.Millisecond || delay > 1250*time.Millisecond {
		t.Errorf("first attempt backoff = %v, expected between 750ms and 1250ms", delay)
	}
}

func TestCalculateBackoff_IncreasingDelay(t *testing.T) {
	cfg := DefaultRetryConfig()
	// Run multiple times to account for jitter; compare averages
	var delays []time.Duration
	for attempt := 1; attempt <= 3; attempt++ {
		total := time.Duration(0)
		iterations := 100
		for i := 0; i < iterations; i++ {
			total += calculateBackoff(attempt, cfg)
		}
		delays = append(delays, total/time.Duration(iterations))
	}
	// Each subsequent attempt should have a higher average delay
	if delays[1] <= delays[0] {
		t.Errorf("attempt 2 avg delay (%v) should be > attempt 1 (%v)", delays[1], delays[0])
	}
	if delays[2] <= delays[1] {
		t.Errorf("attempt 3 avg delay (%v) should be > attempt 2 (%v)", delays[2], delays[1])
	}
}

func TestCalculateBackoff_CappedAtMaxDelay(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   10,
		InitialDelay: 10 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   10.0,
	}
	// With large multiplier, raw delay would be huge; should be capped at MaxDelay
	delay := calculateBackoff(5, cfg)
	// Allow jitter margin: max is MaxDelay + 25% = 37.5s
	maxExpected := time.Duration(float64(cfg.MaxDelay) * 1.26)
	if delay > maxExpected {
		t.Errorf("backoff = %v, should be capped near MaxDelay (%v)", delay, maxExpected)
	}
}

// ---------------------------------------------------------------------------
// containsIgnoreCase
// ---------------------------------------------------------------------------

func TestContainsIgnoreCase(t *testing.T) {
	cases := []struct {
		s, substr string
		want      bool
	}{
		{"Hello World", "hello", true},
		{"Hello World", "WORLD", true},
		{"Hello World", "xyz", false},
		{"API error 429", "api error 429", true},
		{"", "test", false},
		{"test", "", false},
		{"abc", "abcd", false},
		{"connection refused", "CONNECTION REFUSED", true},
	}
	for _, tc := range cases {
		got := containsIgnoreCase(tc.s, tc.substr)
		if got != tc.want {
			t.Errorf("containsIgnoreCase(%q, %q) = %v, want %v", tc.s, tc.substr, got, tc.want)
		}
	}
}
