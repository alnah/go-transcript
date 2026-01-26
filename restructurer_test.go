package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// =============================================================================
// Options - Defensive Behavior
// =============================================================================

// TestWithRestructurerMaxRetries_IgnoresNegative verifies that negative values
// are ignored, preserving the default. This documents the defensive check in the code.
func TestWithRestructurerMaxRetries_IgnoresNegative(t *testing.T) {
	mock := withChatSuccess("result")

	// Create with negative maxRetries - should be ignored
	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		WithRestructurerMaxRetries(-1),
		WithRestructurerRetryDelays(1*time.Millisecond, 1*time.Millisecond),
	)

	// Trigger retries by returning errors then success
	mock.responses = []mockChatResponse{
		{err: apiError(http.StatusTooManyRequests, "rate limited")},
		{err: apiError(http.StatusTooManyRequests, "rate limited")},
		{err: apiError(http.StatusTooManyRequests, "rate limited")},
		{response: openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: "success"}}},
		}},
	}
	mock.callIndex = 0

	// Should use default maxRetries (3), so 4 calls total (initial + 3 retries)
	_, err := r.Restructure(context.Background(), "test transcript", TemplateBrainstorm, "")
	assertNoError(t, err)

	// Default is 3 retries = 4 total calls
	if mock.CallCount() != 4 {
		t.Errorf("expected 4 calls (default maxRetries=3), got %d", mock.CallCount())
	}
}

// TestWithRestructurerRetryDelays_IgnoresInvalid verifies that zero/negative delays
// are ignored, preserving defaults. This documents the defensive checks in the code.
func TestWithRestructurerRetryDelays_IgnoresInvalid(t *testing.T) {
	// Test that zero values don't cause issues
	r := NewOpenAIRestructurer(nil,
		withChatCompleter(withChatSuccess("ok")),
		WithRestructurerRetryDelays(0, 0),                  // Both invalid
		WithRestructurerRetryDelays(-1, -1),                // Both negative
		WithRestructurerRetryDelays(1*time.Millisecond, 0), // Only base valid
		WithRestructurerRetryDelays(0, 1*time.Millisecond), // Only max valid
	)

	// Should not panic and should work with defaults
	result, err := r.Restructure(context.Background(), "test", TemplateBrainstorm, "")
	assertNoError(t, err)
	assertEqual(t, result, "ok")
}

// =============================================================================
// Happy Path
// =============================================================================

// TestRestructure_Success verifies the basic happy path: valid template, short transcript,
// mock returns content.
func TestRestructure_Success(t *testing.T) {
	expectedOutput := "# Restructured Content\n\n- Point 1\n- Point 2"
	mock := withChatSuccess(expectedOutput)

	r := NewOpenAIRestructurer(nil, withChatCompleter(mock))

	result, err := r.Restructure(context.Background(), "raw transcript content", TemplateBrainstorm, "")

	assertNoError(t, err)
	assertEqual(t, result, expectedOutput)
	assertEqual(t, mock.CallCount(), 1)
}

// TestRestructure_RequestFormat verifies the OpenAI request is correctly formatted:
// - Model is gpt-4o-mini
// - Temperature is 0 (deterministic)
// - System message contains the template prompt
// - User message contains the transcript
func TestRestructure_RequestFormat(t *testing.T) {
	mock := withChatSuccess("output")
	r := NewOpenAIRestructurer(nil, withChatCompleter(mock))

	transcript := "This is the raw transcript to restructure."
	_, err := r.Restructure(context.Background(), transcript, TemplateMeeting, "")
	assertNoError(t, err)

	req := mock.LastRequest()
	if req == nil {
		t.Fatal("expected a request to be made")
	}

	// Verify model
	if req.Model != "gpt-4o-mini" {
		t.Errorf("expected model 'gpt-4o-mini', got %q", req.Model)
	}

	// Verify temperature (deterministic output)
	if req.Temperature != 0 {
		t.Errorf("expected temperature 0, got %v", req.Temperature)
	}

	// Verify message structure
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}

	// System message contains template
	systemMsg := req.Messages[0]
	if systemMsg.Role != openai.ChatMessageRoleSystem {
		t.Errorf("expected first message role 'system', got %q", systemMsg.Role)
	}
	// Meeting template should contain "meeting"
	if !strings.Contains(systemMsg.Content, "meeting") {
		t.Errorf("system message should contain meeting template, got %q", systemMsg.Content[:min(100, len(systemMsg.Content))])
	}

	// User message contains transcript
	userMsg := req.Messages[1]
	if userMsg.Role != openai.ChatMessageRoleUser {
		t.Errorf("expected second message role 'user', got %q", userMsg.Role)
	}
	if userMsg.Content != transcript {
		t.Errorf("expected user message to be transcript, got %q", userMsg.Content)
	}
}

// TestRestructure_EmptyTranscript verifies that empty transcripts are accepted.
// This is a valid edge case (user might want to test the pipeline).
func TestRestructure_EmptyTranscript(t *testing.T) {
	mock := withChatSuccess("empty result")
	r := NewOpenAIRestructurer(nil, withChatCompleter(mock))

	result, err := r.Restructure(context.Background(), "", TemplateBrainstorm, "")

	assertNoError(t, err)
	assertEqual(t, result, "empty result")
}

// =============================================================================
// Language Handling
// =============================================================================

// TestRestructure_OutputLang_English verifies that English output languages
// do NOT add a "Respond in" instruction (template is natively English).
func TestRestructure_OutputLang_English(t *testing.T) {
	cases := []struct {
		name       string
		outputLang string
	}{
		{"empty", ""},
		{"en", "en"},
		{"en_lowercase", "en"},
		{"en-US", "en-US"},
		{"en-GB", "en-GB"},
		{"EN_uppercase", "EN"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := withChatSuccess("result")
			r := NewOpenAIRestructurer(nil, withChatCompleter(mock))

			_, err := r.Restructure(context.Background(), "transcript", TemplateBrainstorm, tc.outputLang)
			assertNoError(t, err)

			req := mock.LastRequest()
			systemPrompt := req.Messages[0].Content

			// Should NOT contain "Respond in" for English
			if strings.Contains(systemPrompt, "Respond in") {
				t.Errorf("English output should not add language instruction, got: %s", systemPrompt[:min(50, len(systemPrompt))])
			}
		})
	}
}

// TestRestructure_OutputLang_NonEnglish verifies that non-English output languages
// add a "Respond in {Language}." instruction at the beginning of the prompt.
// Display names come from LanguageDisplayName() in language.go.
func TestRestructure_OutputLang_NonEnglish(t *testing.T) {
	cases := []struct {
		name             string
		outputLang       string
		expectedContains string
	}{
		{"french", "fr", "Respond in French"},
		{"french_CA", "fr-CA", "Respond in Canadian French"},
		{"portuguese_BR", "pt-BR", "Respond in Brazilian Portuguese"},
		{"spanish", "es", "Respond in Spanish"},
		{"german", "de", "Respond in German"},
		{"chinese", "zh", "Respond in Chinese"},
		{"japanese", "ja", "Respond in Japanese"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := withChatSuccess("result")
			r := NewOpenAIRestructurer(nil, withChatCompleter(mock))

			_, err := r.Restructure(context.Background(), "transcript", TemplateBrainstorm, tc.outputLang)
			assertNoError(t, err)

			req := mock.LastRequest()
			systemPrompt := req.Messages[0].Content

			// Should contain language instruction
			if !strings.Contains(systemPrompt, tc.expectedContains) {
				t.Errorf("expected prompt to contain %q, got: %s", tc.expectedContains, systemPrompt[:min(100, len(systemPrompt))])
			}

			// Language instruction should be at the beginning
			if !strings.HasPrefix(systemPrompt, "Respond in") {
				t.Errorf("language instruction should be at the start, got: %s", systemPrompt[:min(50, len(systemPrompt))])
			}

			// Original English template should follow
			if !strings.Contains(systemPrompt, "Rules") {
				t.Error("original English template should be preserved after language instruction")
			}
		})
	}
}

// =============================================================================
// Validation Errors
// =============================================================================

// TestRestructure_UnknownTemplate verifies that invalid template names
// return ErrUnknownTemplate without making any API calls.
func TestRestructure_UnknownTemplate(t *testing.T) {
	mock := withChatSuccess("should not be called")
	r := NewOpenAIRestructurer(nil, withChatCompleter(mock))

	_, err := r.Restructure(context.Background(), "transcript", "invalid_template", "")

	assertError(t, err, ErrUnknownTemplate)
	assertEqual(t, mock.CallCount(), 0) // No API call should be made
}

// TestRestructure_TranscriptTooLong verifies token limit enforcement.
// The limit is 100K tokens, estimated at 3 chars/token = 300K chars.
func TestRestructure_TranscriptTooLong(t *testing.T) {
	t.Run("clearly_over_limit", func(t *testing.T) {
		mock := withChatSuccess("should not be called")
		r := NewOpenAIRestructurer(nil, withChatCompleter(mock))

		// 400K chars = ~133K tokens (over 100K limit)
		longTranscript := strings.Repeat("a", 400_000)

		_, err := r.Restructure(context.Background(), longTranscript, TemplateBrainstorm, "")

		assertError(t, err, ErrTranscriptTooLong)
		assertEqual(t, mock.CallCount(), 0)
	})

	t.Run("at_exact_limit_succeeds", func(t *testing.T) {
		mock := withChatSuccess("result")
		r := NewOpenAIRestructurer(nil, withChatCompleter(mock))

		// Exactly 300K chars = 100K tokens (at limit, should pass)
		exactTranscript := strings.Repeat("a", 300_000)

		_, err := r.Restructure(context.Background(), exactTranscript, TemplateBrainstorm, "")

		assertNoError(t, err)
		assertEqual(t, mock.CallCount(), 1)
	})

	t.Run("just_over_limit_fails", func(t *testing.T) {
		mock := withChatSuccess("should not be called")
		r := NewOpenAIRestructurer(nil, withChatCompleter(mock))

		// 300K + 3 chars = 100001 tokens (just over limit)
		overTranscript := strings.Repeat("a", 300_003)

		_, err := r.Restructure(context.Background(), overTranscript, TemplateBrainstorm, "")

		assertError(t, err, ErrTranscriptTooLong)
		assertEqual(t, mock.CallCount(), 0)
	})
}

// =============================================================================
// Retry Logic
// =============================================================================

// TestRestructure_Retries verifies that transient errors trigger retries.
func TestRestructure_Retries(t *testing.T) {
	cases := []struct {
		name          string
		initialErrors []error
		expectedCalls int
	}{
		{
			name: "rate_limit_then_success",
			initialErrors: []error{
				apiError(http.StatusTooManyRequests, "rate limited"),
				apiError(http.StatusTooManyRequests, "rate limited"),
			},
			expectedCalls: 3, // 2 failures + 1 success
		},
		{
			name: "timeout_then_success",
			initialErrors: []error{
				apiError(http.StatusGatewayTimeout, "timeout"),
			},
			expectedCalls: 2, // 1 failure + 1 success
		},
		{
			name: "server_500_then_success",
			initialErrors: []error{
				apiError(http.StatusInternalServerError, "server error"),
			},
			expectedCalls: 2,
		},
		{
			name: "server_502_then_success",
			initialErrors: []error{
				apiError(http.StatusBadGateway, "bad gateway"),
			},
			expectedCalls: 2,
		},
		{
			name: "server_503_then_success",
			initialErrors: []error{
				apiError(http.StatusServiceUnavailable, "unavailable"),
			},
			expectedCalls: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build response sequence: errors followed by success
			responses := make([]mockChatResponse, 0, len(tc.initialErrors)+1)
			for _, err := range tc.initialErrors {
				responses = append(responses, mockChatResponse{err: err})
			}
			responses = append(responses, mockChatResponse{
				response: openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: "success"}}},
				},
			})

			mock := withChatSequence(responses...)
			r := NewOpenAIRestructurer(nil,
				withChatCompleter(mock),
				WithRestructurerRetryDelays(1*time.Millisecond, 10*time.Millisecond),
			)

			result, err := r.Restructure(context.Background(), "transcript", TemplateBrainstorm, "")

			assertNoError(t, err)
			assertEqual(t, result, "success")
			assertEqual(t, mock.CallCount(), tc.expectedCalls)
		})
	}
}

// TestRestructure_FailsAfterMaxRetries verifies that after exhausting retries,
// the last error is returned.
func TestRestructure_FailsAfterMaxRetries(t *testing.T) {
	// Create mock that always returns rate limit error
	mock := withChatError(apiError(http.StatusTooManyRequests, "rate limited"))

	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		WithRestructurerMaxRetries(2), // 2 retries = 3 total calls
		WithRestructurerRetryDelays(1*time.Millisecond, 1*time.Millisecond),
	)

	_, err := r.Restructure(context.Background(), "transcript", TemplateBrainstorm, "")

	// Should fail with rate limit error (wrapped)
	assertError(t, err, ErrRateLimit)
	// Should have made initial + 2 retries = 3 calls
	assertEqual(t, mock.CallCount(), 3)
	// Error message should mention max retries
	if !strings.Contains(err.Error(), "max retries") {
		t.Errorf("expected error to mention 'max retries', got: %v", err)
	}
}

// TestRestructure_NoRetry verifies that non-transient errors fail immediately.
func TestRestructure_NoRetry(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		expectedErr error
	}{
		{
			name:        "auth_failed",
			err:         apiError(http.StatusUnauthorized, "invalid api key"),
			expectedErr: ErrAuthFailed,
		},
		{
			name:        "context_canceled",
			err:         context.Canceled,
			expectedErr: context.Canceled,
		},
		{
			name:        "context_length_exceeded",
			err:         apiError(http.StatusBadRequest, "context_length_exceeded"),
			expectedErr: ErrTranscriptTooLong,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := withChatError(tc.err)
			r := NewOpenAIRestructurer(nil,
				withChatCompleter(mock),
				WithRestructurerMaxRetries(3),
				WithRestructurerRetryDelays(1*time.Millisecond, 1*time.Millisecond),
			)

			_, err := r.Restructure(context.Background(), "transcript", TemplateBrainstorm, "")

			assertError(t, err, tc.expectedErr)
			// Should fail immediately - only 1 call
			assertEqual(t, mock.CallCount(), 1)
		})
	}
}

// TestRestructure_ContextCancellation verifies that context cancellation
// stops retries immediately.
func TestRestructure_ContextCancellation(t *testing.T) {
	mock := withChatError(apiError(http.StatusTooManyRequests, "rate limited"))

	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		WithRestructurerMaxRetries(5),
		WithRestructurerRetryDelays(100*time.Millisecond, 100*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := r.Restructure(ctx, "transcript", TemplateBrainstorm, "")

	// Should return context error
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// =============================================================================
// classifyRestructureError
// =============================================================================

// TestClassifyRestructureError verifies the error classification logic.
func TestClassifyRestructureError(t *testing.T) {
	cases := []struct {
		name        string
		input       error
		expectedErr error
		shouldWrap  bool // true if we expect the sentinel to be wrapped
	}{
		{
			name:        "nil_returns_nil",
			input:       nil,
			expectedErr: nil,
			shouldWrap:  false,
		},
		{
			name:        "rate_limit_429",
			input:       apiError(http.StatusTooManyRequests, "rate limited"),
			expectedErr: ErrRateLimit,
			shouldWrap:  true,
		},
		{
			name:        "auth_failed_401",
			input:       apiError(http.StatusUnauthorized, "invalid key"),
			expectedErr: ErrAuthFailed,
			shouldWrap:  true,
		},
		{
			name:        "timeout_408",
			input:       apiError(http.StatusRequestTimeout, "request timeout"),
			expectedErr: ErrTimeout,
			shouldWrap:  true,
		},
		{
			name:        "timeout_504",
			input:       apiError(http.StatusGatewayTimeout, "gateway timeout"),
			expectedErr: ErrTimeout,
			shouldWrap:  true,
		},
		{
			name:        "context_length_from_400",
			input:       apiError(http.StatusBadRequest, "maximum context length exceeded"),
			expectedErr: ErrTranscriptTooLong,
			shouldWrap:  true,
		},
		{
			name:        "context_length_from_message",
			input:       apiError(http.StatusBadRequest, "context_length error"),
			expectedErr: ErrTranscriptTooLong,
			shouldWrap:  true,
		},
		{
			name:        "deadline_exceeded",
			input:       context.DeadlineExceeded,
			expectedErr: ErrTimeout,
			shouldWrap:  true,
		},
		{
			name:        "context_length_from_string_error",
			input:       errors.New("context_length_exceeded somewhere"),
			expectedErr: ErrTranscriptTooLong,
			shouldWrap:  true,
		},
		{
			name:        "unknown_error_passthrough",
			input:       errors.New("some random error"),
			expectedErr: nil, // Returns original error unchanged
			shouldWrap:  false,
		},
		{
			name:        "server_error_500_not_classified",
			input:       apiError(http.StatusInternalServerError, "server error"),
			expectedErr: nil, // 500 errors are not classified to sentinels (but are retryable)
			shouldWrap:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := classifyRestructureError(tc.input)

			if tc.expectedErr == nil {
				if tc.input == nil {
					if result != nil {
						t.Errorf("expected nil, got %v", result)
					}
				} else {
					// Unknown errors should pass through unchanged
					if result != tc.input {
						t.Errorf("expected original error, got %v", result)
					}
				}
				return
			}

			if tc.shouldWrap {
				if !errors.Is(result, tc.expectedErr) {
					t.Errorf("expected error wrapping %v, got %v", tc.expectedErr, result)
				}
			}
		})
	}
}

// =============================================================================
// isRetryableRestructureError
// =============================================================================

// TestIsRetryableRestructureError verifies the retry decision logic.
func TestIsRetryableRestructureError(t *testing.T) {
	cases := []struct {
		name     string
		input    error
		expected bool
	}{
		{
			name:     "rate_limit_is_retryable",
			input:    ErrRateLimit,
			expected: true,
		},
		{
			name:     "wrapped_rate_limit_is_retryable",
			input:    errors.Join(errors.New("context"), ErrRateLimit),
			expected: true,
		},
		{
			name:     "timeout_is_retryable",
			input:    ErrTimeout,
			expected: true,
		},
		{
			name:     "server_500_is_retryable",
			input:    apiError(http.StatusInternalServerError, "error"),
			expected: true,
		},
		{
			name:     "server_502_is_retryable",
			input:    apiError(http.StatusBadGateway, "error"),
			expected: true,
		},
		{
			name:     "server_503_is_retryable",
			input:    apiError(http.StatusServiceUnavailable, "error"),
			expected: true,
		},
		{
			name:     "server_504_is_retryable",
			input:    apiError(http.StatusGatewayTimeout, "error"),
			expected: true,
		},
		{
			name:     "auth_failed_not_retryable",
			input:    ErrAuthFailed,
			expected: false,
		},
		{
			name:     "transcript_too_long_not_retryable",
			input:    ErrTranscriptTooLong,
			expected: false,
		},
		{
			name:     "context_canceled_not_retryable",
			input:    context.Canceled,
			expected: false,
		},
		{
			name:     "unknown_error_not_retryable",
			input:    errors.New("random error"),
			expected: false,
		},
		{
			name:     "client_400_not_retryable",
			input:    apiError(http.StatusBadRequest, "bad request"),
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := isRetryableRestructureError(tc.input)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

// =============================================================================
// estimateTokens
// =============================================================================

// TestEstimateTokens documents the token estimation logic: 3 chars per token.
// This is conservative for French text (actual average is ~3.5 chars/token).
func TestEstimateTokens(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty", "", 0},
		{"3_chars", "abc", 1},
		{"6_chars", "abcdef", 2},
		{"300k_chars", strings.Repeat("a", 300_000), 100_000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := estimateTokens(tc.input)
			if result != tc.expected {
				t.Errorf("expected %d tokens, got %d", tc.expected, result)
			}
		})
	}
}

// =============================================================================
// restructureWithCustomPrompt
// =============================================================================

// TestRestructureWithCustomPrompt verifies the custom prompt method used by MapReduce.
// Key behaviors: uses custom prompt (no template resolution), no token limit check.
func TestRestructureWithCustomPrompt(t *testing.T) {
	t.Run("uses_custom_prompt", func(t *testing.T) {
		mock := withChatSuccess("merged result")
		r := NewOpenAIRestructurer(nil, withChatCompleter(mock))

		customPrompt := "This is a custom merge prompt for MapReduce."
		content := "Content to process"

		result, err := r.restructureWithCustomPrompt(context.Background(), content, customPrompt)

		assertNoError(t, err)
		assertEqual(t, result, "merged result")

		req := mock.LastRequest()

		// System message should be the custom prompt (not a resolved template)
		if req.Messages[0].Content != customPrompt {
			t.Errorf("expected custom prompt, got: %s", req.Messages[0].Content[:min(50, len(req.Messages[0].Content))])
		}

		// User message should be the content
		if req.Messages[1].Content != content {
			t.Errorf("expected content, got: %s", req.Messages[1].Content)
		}
	})

	t.Run("no_token_limit_check", func(t *testing.T) {
		mock := withChatSuccess("result")
		r := NewOpenAIRestructurer(nil, withChatCompleter(mock))

		// 400K chars would fail Restructure() but should pass here
		longContent := strings.Repeat("a", 400_000)

		_, err := r.restructureWithCustomPrompt(context.Background(), longContent, "prompt")

		assertNoError(t, err)
		assertEqual(t, mock.CallCount(), 1) // Should have called API
	})
}

// =============================================================================
// Edge Cases
// =============================================================================

// TestRestructure_APIReturnsEmptyChoices verifies handling when API returns
// a response with no choices (unexpected but possible).
func TestRestructure_APIReturnsEmptyChoices(t *testing.T) {
	mock := newMockChatCompleter(mockChatResponse{
		response: openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{}, // Empty choices
		},
	})

	r := NewOpenAIRestructurer(nil, withChatCompleter(mock))

	_, err := r.Restructure(context.Background(), "transcript", TemplateBrainstorm, "")

	if err == nil {
		t.Error("expected error for empty choices")
	}
	if !strings.Contains(err.Error(), "no response") {
		t.Errorf("expected 'no response' error, got: %v", err)
	}
}

// TestRestructure_WithCustomModel verifies the WithModel option works.
func TestRestructure_WithCustomModel(t *testing.T) {
	mock := withChatSuccess("result")
	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		WithModel("gpt-4-turbo"),
	)

	_, err := r.Restructure(context.Background(), "transcript", TemplateBrainstorm, "")
	assertNoError(t, err)

	req := mock.LastRequest()
	if req.Model != "gpt-4-turbo" {
		t.Errorf("expected model 'gpt-4-turbo', got %q", req.Model)
	}
}

// TestRestructure_WithCustomMaxTokens verifies the WithMaxInputTokens option works.
func TestRestructure_WithCustomMaxTokens(t *testing.T) {
	mock := withChatSuccess("result")
	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		WithMaxInputTokens(1000), // Very low limit: 3000 chars
	)

	// 4000 chars = ~1333 tokens, should fail with limit of 1000
	_, err := r.Restructure(context.Background(), strings.Repeat("a", 4000), TemplateBrainstorm, "")

	assertError(t, err, ErrTranscriptTooLong)
	assertEqual(t, mock.CallCount(), 0)
}
