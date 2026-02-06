package restructure_test

// Notes:
// - OpenAI-specific tests for OpenAIRestructurer
// - Tests use black-box approach via package restructure_test
// - Shared mocks are defined in restructurer_test.go

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/alnah/go-transcript/internal/lang"
	"github.com/alnah/go-transcript/internal/restructure"
	"github.com/alnah/go-transcript/internal/template"
	"github.com/alnah/go-transcript/internal/transcribe"
)

// ---------------------------------------------------------------------------
// TestClassifyRestructureError - OpenAI error classification
// ---------------------------------------------------------------------------

func TestClassifyRestructureError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		wantErr error
		wantNil bool
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			wantNil: true,
		},
		{
			name:    "rate limit 429",
			err:     apiError(http.StatusTooManyRequests, "rate limit exceeded"),
			wantErr: transcribe.ErrRateLimit,
		},
		{
			name:    "auth failed 401",
			err:     apiError(http.StatusUnauthorized, "invalid api key"),
			wantErr: transcribe.ErrAuthFailed,
		},
		{
			name:    "request timeout 408",
			err:     apiError(http.StatusRequestTimeout, "request timed out"),
			wantErr: transcribe.ErrTimeout,
		},
		{
			name:    "gateway timeout 504",
			err:     apiError(http.StatusGatewayTimeout, "gateway timeout"),
			wantErr: transcribe.ErrTimeout,
		},
		{
			name:    "context length exceeded via status 400",
			err:     apiError(http.StatusBadRequest, "maximum context length exceeded"),
			wantErr: restructure.ErrTranscriptTooLong,
		},
		{
			name:    "context length exceeded via message pattern",
			err:     apiError(http.StatusBadRequest, "context_length issue"),
			wantErr: restructure.ErrTranscriptTooLong,
		},
		{
			name:    "context deadline exceeded",
			err:     context.DeadlineExceeded,
			wantErr: transcribe.ErrTimeout,
		},
		{
			name:    "context length in plain error string",
			err:     errors.New("context_length_exceeded"),
			wantErr: restructure.ErrTranscriptTooLong,
		},
		{
			name:    "maximum context length in plain error string",
			err:     errors.New("maximum context length exceeded"),
			wantErr: restructure.ErrTranscriptTooLong,
		},
		{
			name:    "unknown error passes through",
			err:     errors.New("random error"),
			wantErr: nil,
		},
		{
			name:    "server error 500 passes through",
			err:     apiError(http.StatusInternalServerError, "internal error"),
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := restructure.ClassifyRestructureError(tt.err)

			if tt.wantNil {
				if got != nil {
					t.Errorf("ClassifyRestructureError(%v) = %v, want nil", tt.err, got)
				}
				return
			}

			if tt.wantErr == nil {
				if got == nil {
					t.Errorf("ClassifyRestructureError(%v) = nil, want non-nil error", tt.err)
				}
				return
			}

			if !errors.Is(got, tt.wantErr) {
				t.Errorf("ClassifyRestructureError(%v) = %v, want error wrapping %v", tt.err, got, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestIsRetryableRestructureError - OpenAI retry decision
// ---------------------------------------------------------------------------

func TestIsRetryableRestructureError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "rate limit is retryable",
			err:  fmt.Errorf("wrapped: %w", transcribe.ErrRateLimit),
			want: true,
		},
		{
			name: "timeout is retryable",
			err:  fmt.Errorf("wrapped: %w", transcribe.ErrTimeout),
			want: true,
		},
		{
			name: "server error 500 is retryable",
			err:  apiError(http.StatusInternalServerError, "internal"),
			want: true,
		},
		{
			name: "server error 502 is retryable",
			err:  apiError(http.StatusBadGateway, "bad gateway"),
			want: true,
		},
		{
			name: "server error 503 is retryable",
			err:  apiError(http.StatusServiceUnavailable, "unavailable"),
			want: true,
		},
		{
			name: "server error 504 is retryable",
			err:  apiError(http.StatusGatewayTimeout, "timeout"),
			want: true,
		},
		{
			name: "context canceled is not retryable",
			err:  context.Canceled,
			want: false,
		},
		{
			name: "auth failed is not retryable",
			err:  fmt.Errorf("wrapped: %w", transcribe.ErrAuthFailed),
			want: false,
		},
		{
			name: "transcript too long is not retryable",
			err:  fmt.Errorf("wrapped: %w", restructure.ErrTranscriptTooLong),
			want: false,
		},
		{
			name: "unknown error is not retryable",
			err:  errors.New("random error"),
			want: false,
		},
		{
			name: "client error 400 is not retryable",
			err:  apiError(http.StatusBadRequest, "bad request"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := restructure.IsRetryableRestructureError(tt.err)
			if got != tt.want {
				t.Errorf("IsRetryableRestructureError() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestOpenAIRestructurer_Restructure - OpenAI restructuring
// ---------------------------------------------------------------------------

func TestOpenAIRestructurer_Restructure(t *testing.T) {
	t.Parallel()

	t.Run("happy path returns restructured content", func(t *testing.T) {
		t.Parallel()

		mock := &mockChatCompleter{
			response: successResponse("# Restructured Content\n\nThis is the result."),
		}

		r := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(mock),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		result, err := r.Restructure(context.Background(), "Raw transcript.", template.MustParseName("meeting"), lang.Language{})
		if err != nil {
			t.Fatalf("Restructure() unexpected error: %v", err)
		}

		want := "# Restructured Content\n\nThis is the result."
		if result != want {
			t.Errorf("Restructure() = %q, want %q", result, want)
		}

		if got, want := mock.CallCount(), 1; got != want {
			t.Errorf("CallCount() = %d, want %d", got, want)
		}
	})

	// Note: "invalid template returns error" test removed.
	// With the template.Name type, invalid templates are caught at parse time
	// (template.ParseName), not at restructure time. This is tested in template_test.go.

	t.Run("transcript too long returns error", func(t *testing.T) {
		t.Parallel()

		mock := &mockChatCompleter{}

		r := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(mock),
			restructure.WithMaxInputTokens(10),
		)

		longTranscript := strings.Repeat("x", 100)

		_, err := r.Restructure(context.Background(), longTranscript, template.MustParseName("meeting"), lang.Language{})
		if err == nil {
			t.Fatal("Restructure() with long transcript: got nil error, want ErrTranscriptTooLong")
		}

		if !errors.Is(err, restructure.ErrTranscriptTooLong) {
			t.Errorf("Restructure() error = %v, want ErrTranscriptTooLong", err)
		}

		if got := mock.CallCount(); got != 0 {
			t.Errorf("CallCount() = %d, want 0 (should not call API if transcript too long)", got)
		}
	})

	t.Run("adds language instruction for non-English", func(t *testing.T) {
		t.Parallel()

		mock := &mockChatCompleter{
			response: successResponse("Contenu restructuré."),
		}

		r := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(mock),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		_, err := r.Restructure(context.Background(), "transcript", template.MustParseName("meeting"), lang.MustParse("fr"))
		if err != nil {
			t.Fatalf("Restructure() unexpected error: %v", err)
		}

		prompt := mock.SystemPrompt()
		if !strings.Contains(prompt, "Respond in French") {
			t.Errorf("SystemPrompt() = %q, want containing %q", prompt, "Respond in French")
		}
	})

	t.Run("no language instruction for English", func(t *testing.T) {
		t.Parallel()

		mock := &mockChatCompleter{
			response: successResponse("Restructured content."),
		}

		r := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(mock),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		_, err := r.Restructure(context.Background(), "transcript", template.MustParseName("meeting"), lang.MustParse("en"))
		if err != nil {
			t.Fatalf("Restructure() unexpected error: %v", err)
		}

		prompt := mock.SystemPrompt()
		if strings.Contains(prompt, "Respond in") {
			t.Errorf("SystemPrompt() = %q, should not contain language instruction for English", prompt)
		}
	})

	t.Run("no language instruction for en-US", func(t *testing.T) {
		t.Parallel()

		mock := &mockChatCompleter{
			response: successResponse("Restructured content."),
		}

		r := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(mock),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		_, err := r.Restructure(context.Background(), "transcript", template.MustParseName("meeting"), lang.MustParse("en-US"))
		if err != nil {
			t.Fatalf("Restructure() unexpected error: %v", err)
		}

		prompt := mock.SystemPrompt()
		if strings.Contains(prompt, "Respond in") {
			t.Errorf("SystemPrompt() = %q, should not contain language instruction for en-US", prompt)
		}
	})

	t.Run("no language instruction for empty outputLang", func(t *testing.T) {
		t.Parallel()

		mock := &mockChatCompleter{
			response: successResponse("Content."),
		}

		r := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(mock),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		_, err := r.Restructure(context.Background(), "transcript", template.MustParseName("meeting"), lang.Language{})
		if err != nil {
			t.Fatalf("Restructure() unexpected error: %v", err)
		}

		prompt := mock.SystemPrompt()
		if strings.Contains(prompt, "Respond in") {
			t.Errorf("SystemPrompt() = %q, should not contain language instruction for empty lang", prompt)
		}
	})

	t.Run("API returns empty choices", func(t *testing.T) {
		t.Parallel()

		mock := &mockChatCompleter{
			response: openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{},
			},
		}

		r := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(mock),
			restructure.WithMaxRetries(0),
		)

		_, err := r.Restructure(context.Background(), "transcript", template.MustParseName("meeting"), lang.Language{})
		if err == nil {
			t.Fatal("Restructure() with empty choices: got nil error, want non-nil")
		}

		if !strings.Contains(err.Error(), "no response") {
			t.Errorf("Restructure() error = %q, want containing %q", err.Error(), "no response")
		}
	})
}

// ---------------------------------------------------------------------------
// TestOpenAIRetryBehavior - OpenAI retry with backoff
// ---------------------------------------------------------------------------

func TestOpenAIRetryBehavior(t *testing.T) {
	t.Parallel()

	t.Run("retries on rate limit then succeeds", func(t *testing.T) {
		t.Parallel()

		mock := &mockChatCompleter{
			errSequence: []error{
				apiError(http.StatusTooManyRequests, "rate limit"),
				apiError(http.StatusTooManyRequests, "rate limit"),
				nil,
			},
			response: successResponse("Success after retries"),
		}

		r := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(mock),
			restructure.WithMaxRetries(5),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		result, err := r.Restructure(context.Background(), "transcript", template.MustParseName("meeting"), lang.Language{})
		if err != nil {
			t.Fatalf("Restructure() unexpected error: %v", err)
		}

		want := "Success after retries"
		if result != want {
			t.Errorf("Restructure() = %q, want %q", result, want)
		}

		if got, want := mock.CallCount(), 3; got != want {
			t.Errorf("CallCount() = %d, want %d", got, want)
		}
	})

	t.Run("does not retry on auth error", func(t *testing.T) {
		t.Parallel()

		mock := &mockChatCompleter{
			err: apiError(http.StatusUnauthorized, "invalid key"),
		}

		r := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(mock),
			restructure.WithMaxRetries(5),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		_, err := r.Restructure(context.Background(), "transcript", template.MustParseName("meeting"), lang.Language{})
		if err == nil {
			t.Fatal("Restructure() with auth error: got nil error, want ErrAuthFailed")
		}

		if !errors.Is(err, transcribe.ErrAuthFailed) {
			t.Errorf("Restructure() error = %v, want ErrAuthFailed", err)
		}

		if got, want := mock.CallCount(), 1; got != want {
			t.Errorf("CallCount() = %d, want %d (no retry)", got, want)
		}
	})

	t.Run("does not retry on transcript too long", func(t *testing.T) {
		t.Parallel()

		mock := &mockChatCompleter{
			err: apiError(http.StatusBadRequest, "maximum context length exceeded"),
		}

		r := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(mock),
			restructure.WithMaxRetries(5),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		_, err := r.Restructure(context.Background(), "transcript", template.MustParseName("meeting"), lang.Language{})
		if err == nil {
			t.Fatal("Restructure() with context length error: got nil error, want ErrTranscriptTooLong")
		}

		if !errors.Is(err, restructure.ErrTranscriptTooLong) {
			t.Errorf("Restructure() error = %v, want ErrTranscriptTooLong", err)
		}

		if got, want := mock.CallCount(), 1; got != want {
			t.Errorf("CallCount() = %d, want %d (no retry)", got, want)
		}
	})

	t.Run("max retries exceeded", func(t *testing.T) {
		t.Parallel()

		mock := &mockChatCompleter{
			err: apiError(http.StatusTooManyRequests, "rate limit"),
		}

		r := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(mock),
			restructure.WithMaxRetries(2),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		_, err := r.Restructure(context.Background(), "transcript", template.MustParseName("meeting"), lang.Language{})
		if err == nil {
			t.Fatal("Restructure() after max retries: got nil error, want non-nil")
		}

		if !strings.Contains(err.Error(), "max retries") {
			t.Errorf("Restructure() error = %q, want containing %q", err.Error(), "max retries")
		}

		if got, want := mock.CallCount(), 3; got != want {
			t.Errorf("CallCount() = %d, want %d", got, want)
		}
	})

	t.Run("retries on server error 500", func(t *testing.T) {
		t.Parallel()

		mock := &mockChatCompleter{
			errSequence: []error{
				apiError(http.StatusInternalServerError, "server error"),
				nil,
			},
			response: successResponse("Success"),
		}

		r := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(mock),
			restructure.WithMaxRetries(3),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		result, err := r.Restructure(context.Background(), "transcript", template.MustParseName("meeting"), lang.Language{})
		if err != nil {
			t.Fatalf("Restructure() unexpected error: %v", err)
		}

		want := "Success"
		if result != want {
			t.Errorf("Restructure() = %q, want %q", result, want)
		}

		if got, want := mock.CallCount(), 2; got != want {
			t.Errorf("CallCount() = %d, want %d", got, want)
		}
	})
}

// ---------------------------------------------------------------------------
// OpenAI-specific helpers
// ---------------------------------------------------------------------------

// apiError creates an OpenAI API error with the given status code.
func apiError(statusCode int, message string) *openai.APIError {
	return &openai.APIError{
		HTTPStatusCode: statusCode,
		Message:        message,
	}
}
