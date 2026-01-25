package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// mockChatCompleter is a mock implementation of chatCompleter for testing.
type mockChatCompleter struct {
	response openai.ChatCompletionResponse
	err      error
	// captured captures the request for verification
	captured *openai.ChatCompletionRequest
}

func (m *mockChatCompleter) CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	m.captured = &req
	return m.response, m.err
}

func TestOpenAIRestructurer_Restructure_Success(t *testing.T) {
	mock := &mockChatCompleter{
		response: openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "# Restructured Output\n\n- Point 1\n- Point 2"}},
			},
		},
	}

	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		withTemplateResolver(func(name string) (string, error) {
			if name == "brainstorm" {
				return "Test prompt", nil
			}
			return "", ErrUnknownTemplate
		}),
	)

	result, err := r.Restructure(context.Background(), "Raw transcript content", "brainstorm", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "# Restructured Output\n\n- Point 1\n- Point 2" {
		t.Errorf("unexpected result: %s", result)
	}

	// Verify request was built correctly
	if mock.captured == nil {
		t.Fatal("request was not captured")
	}
	if mock.captured.Model != defaultRestructureModel {
		t.Errorf("expected model %s, got %s", defaultRestructureModel, mock.captured.Model)
	}
	if mock.captured.Temperature != 0 {
		t.Errorf("expected temperature 0, got %f", mock.captured.Temperature)
	}
	if len(mock.captured.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(mock.captured.Messages))
	}
	if mock.captured.Messages[0].Role != openai.ChatMessageRoleSystem {
		t.Errorf("expected system role, got %s", mock.captured.Messages[0].Role)
	}
	if mock.captured.Messages[1].Role != openai.ChatMessageRoleUser {
		t.Errorf("expected user role, got %s", mock.captured.Messages[1].Role)
	}
}

func TestOpenAIRestructurer_Restructure_UnknownTemplate(t *testing.T) {
	r := NewOpenAIRestructurer(nil,
		withChatCompleter(&mockChatCompleter{}),
	)

	_, err := r.Restructure(context.Background(), "transcript", "nonexistent", "")
	if !errors.Is(err, ErrUnknownTemplate) {
		t.Errorf("expected ErrUnknownTemplate, got %v", err)
	}
}

func TestOpenAIRestructurer_Restructure_TranscriptTooLong(t *testing.T) {
	r := NewOpenAIRestructurer(nil,
		withChatCompleter(&mockChatCompleter{}),
		withTemplateResolver(func(name string) (string, error) {
			return "prompt", nil
		}),
		WithMaxInputTokens(100), // Very low limit for testing
	)

	// 400 chars / 3 = ~133 tokens > 100 limit
	longTranscript := strings.Repeat("a", 400)

	_, err := r.Restructure(context.Background(), longTranscript, "brainstorm", "")
	if !errors.Is(err, ErrTranscriptTooLong) {
		t.Errorf("expected ErrTranscriptTooLong, got %v", err)
	}
}

func TestOpenAIRestructurer_Restructure_APIContextLengthExceeded(t *testing.T) {
	mock := &mockChatCompleter{
		err: errors.New("status code: 400, message: context_length_exceeded"),
	}

	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		withTemplateResolver(func(name string) (string, error) {
			return "prompt", nil
		}),
	)

	_, err := r.Restructure(context.Background(), "transcript", "brainstorm", "")
	if !errors.Is(err, ErrTranscriptTooLong) {
		t.Errorf("expected ErrTranscriptTooLong, got %v", err)
	}
}

func TestOpenAIRestructurer_Restructure_RateLimit(t *testing.T) {
	mock := &mockChatCompleter{
		err: &openai.APIError{
			HTTPStatusCode: http.StatusTooManyRequests,
			Message:        "rate limit exceeded",
		},
	}

	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		withTemplateResolver(func(name string) (string, error) {
			return "prompt", nil
		}),
	)

	_, err := r.Restructure(context.Background(), "transcript", "brainstorm", "")
	if !errors.Is(err, ErrRateLimit) {
		t.Errorf("expected ErrRateLimit, got %v", err)
	}
}

func TestOpenAIRestructurer_Restructure_AuthFailed(t *testing.T) {
	mock := &mockChatCompleter{
		err: &openai.APIError{
			HTTPStatusCode: http.StatusUnauthorized,
			Message:        "Incorrect API key provided",
		},
	}

	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		withTemplateResolver(func(name string) (string, error) {
			return "prompt", nil
		}),
	)

	_, err := r.Restructure(context.Background(), "transcript", "brainstorm", "")
	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("expected ErrAuthFailed, got %v", err)
	}
}

func TestOpenAIRestructurer_Restructure_Timeout(t *testing.T) {
	mock := &mockChatCompleter{
		err: context.DeadlineExceeded,
	}

	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		withTemplateResolver(func(name string) (string, error) {
			return "prompt", nil
		}),
	)

	_, err := r.Restructure(context.Background(), "transcript", "brainstorm", "")
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout, got %v", err)
	}
}

func TestOpenAIRestructurer_Restructure_EmptyResponse(t *testing.T) {
	mock := &mockChatCompleter{
		response: openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{}, // Empty
		},
	}

	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		withTemplateResolver(func(name string) (string, error) {
			return "prompt", nil
		}),
	)

	_, err := r.Restructure(context.Background(), "transcript", "brainstorm", "")
	if err == nil {
		t.Error("expected error for empty response")
	}
	if !strings.Contains(err.Error(), "no response") {
		t.Errorf("expected 'no response' error, got %v", err)
	}
}

func TestOpenAIRestructurer_WithOptions(t *testing.T) {
	mock := &mockChatCompleter{
		response: openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "output"}},
			},
		},
	}

	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		withTemplateResolver(func(name string) (string, error) {
			return "prompt", nil
		}),
		WithModel("gpt-4o"),
		WithMaxInputTokens(50000),
	)

	_, err := r.Restructure(context.Background(), "short", "test", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.captured.Model != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %s", mock.captured.Model)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"empty", "", 0},
		{"short", "abc", 1},
		{"medium", strings.Repeat("a", 300), 100},
		{"long", strings.Repeat("a", 3000), 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.text)
			if got != tt.expected {
				t.Errorf("estimateTokens(%d chars) = %d, want %d", len(tt.text), got, tt.expected)
			}
		})
	}
}

func TestClassifyRestructureError(t *testing.T) {
	tests := []struct {
		name        string
		inputErr    error
		expectedErr error
	}{
		{"nil", nil, nil},
		{"context_length", errors.New("context_length_exceeded"), ErrTranscriptTooLong},
		{"max_context", errors.New("maximum context length"), ErrTranscriptTooLong},
		{"deadline", context.DeadlineExceeded, ErrTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyRestructureError(tt.inputErr)
			if tt.expectedErr == nil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if !errors.Is(got, tt.expectedErr) {
				t.Errorf("expected %v, got %v", tt.expectedErr, got)
			}
		})
	}
}

func TestIsRetryableRestructureError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"rate_limit", ErrRateLimit, true},
		{"timeout", ErrTimeout, true},
		{"auth_failed", ErrAuthFailed, false},
		{"transcript_too_long", ErrTranscriptTooLong, false},
		{"context_canceled", context.Canceled, false},
		{"generic_error", errors.New("some error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableRestructureError(tt.err)
			if got != tt.expected {
				t.Errorf("isRetryableRestructureError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestOpenAIRestructurer_UsesRealGetTemplate(t *testing.T) {
	mock := &mockChatCompleter{
		response: openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "output"}},
			},
		},
	}

	// Use real GetTemplate (not mocked)
	r := NewOpenAIRestructurer(nil, withChatCompleter(mock))

	_, err := r.Restructure(context.Background(), "transcript", "brainstorm", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the real brainstorm prompt was used
	if !strings.Contains(mock.captured.Messages[0].Content, "brainstorming") {
		t.Errorf("expected brainstorm prompt, got: %s", mock.captured.Messages[0].Content)
	}
}

func TestOpenAIRestructurer_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mock := &mockChatCompleter{
		err: context.Canceled,
	}

	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		withTemplateResolver(func(name string) (string, error) {
			return "prompt", nil
		}),
	)

	_, err := r.Restructure(ctx, "transcript", "brainstorm", "")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}
