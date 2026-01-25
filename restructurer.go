package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// Restructurer transforms raw transcripts into structured markdown using templates.
type Restructurer interface {
	// Restructure transforms a transcript using the specified template.
	// outputLang specifies the output language (e.g., "en", "pt-BR").
	// Empty outputLang uses the template's native language (French).
	Restructure(ctx context.Context, transcript, templateName, outputLang string) (string, error)
}

// chatCompleter is an internal interface for OpenAI chat completion.
// *openai.Client implements this implicitly.
// This allows injecting mocks in tests.
type chatCompleter interface {
	CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// templateResolver resolves template names to prompts.
// Default implementation uses GetTemplate from templates.go.
type templateResolver func(name string) (string, error)

// OpenAIRestructurer restructures transcripts using OpenAI's chat completion API.
// It supports automatic retries with exponential backoff for transient errors.
type OpenAIRestructurer struct {
	client          chatCompleter
	model           string
	maxInputTokens  int
	maxRetries      int
	baseDelay       time.Duration
	maxDelay        time.Duration
	resolveTemplate templateResolver
}

// RestructurerOption configures an OpenAIRestructurer.
type RestructurerOption func(*OpenAIRestructurer)

// Default configuration values.
const (
	defaultRestructureModel      = "gpt-4o-mini"
	defaultMaxInputTokens        = 100000
	defaultCharsPerToken         = 3 // Conservative for French text
	defaultRestructureMaxRetries = 3 // Fewer retries than transcriber (longer latency)
	defaultRestructureBaseDelay  = 1 * time.Second
	defaultRestructureMaxDelay   = 30 * time.Second
)

// WithModel sets the model for restructuring.
func WithModel(model string) RestructurerOption {
	return func(r *OpenAIRestructurer) {
		r.model = model
	}
}

// WithMaxInputTokens sets the maximum input token limit.
func WithMaxInputTokens(max int) RestructurerOption {
	return func(r *OpenAIRestructurer) {
		r.maxInputTokens = max
	}
}

// WithRestructurerMaxRetries sets the maximum number of retry attempts.
func WithRestructurerMaxRetries(n int) RestructurerOption {
	return func(r *OpenAIRestructurer) {
		if n >= 0 {
			r.maxRetries = n
		}
	}
}

// WithRestructurerRetryDelays sets the base and max delays for exponential backoff.
func WithRestructurerRetryDelays(base, max time.Duration) RestructurerOption {
	return func(r *OpenAIRestructurer) {
		if base > 0 {
			r.baseDelay = base
		}
		if max > 0 {
			r.maxDelay = max
		}
	}
}

// withTemplateResolver sets a custom template resolver (for testing).
func withTemplateResolver(resolver templateResolver) RestructurerOption {
	return func(r *OpenAIRestructurer) {
		r.resolveTemplate = resolver
	}
}

// withChatCompleter sets a custom chat completer (for testing).
func withChatCompleter(cc chatCompleter) RestructurerOption {
	return func(r *OpenAIRestructurer) {
		r.client = cc
	}
}

// NewOpenAIRestructurer creates a new OpenAIRestructurer with the given client.
// Use options to customize model, token limits, and retry behavior.
func NewOpenAIRestructurer(client *openai.Client, opts ...RestructurerOption) *OpenAIRestructurer {
	r := &OpenAIRestructurer{
		client:          client,
		model:           defaultRestructureModel,
		maxInputTokens:  defaultMaxInputTokens,
		maxRetries:      defaultRestructureMaxRetries,
		baseDelay:       defaultRestructureBaseDelay,
		maxDelay:        defaultRestructureMaxDelay,
		resolveTemplate: GetTemplate,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Restructure transforms a raw transcript into structured markdown using the specified template.
// outputLang specifies the output language (e.g., "en", "pt-BR"). Empty uses template's native language (French).
// Returns ErrUnknownTemplate if the template name is invalid.
// Returns ErrTranscriptTooLong if the transcript exceeds the token limit (estimated).
// Automatically retries on transient errors (rate limits, timeouts, server errors).
//
// Token estimation uses len(text)/3 which is conservative for French text.
// The actual API limit is 128K tokens; we use 100K as a safety margin.
func (r *OpenAIRestructurer) Restructure(ctx context.Context, transcript, templateName, outputLang string) (string, error) {
	// 1. Resolve template
	prompt, err := r.resolveTemplate(templateName)
	if err != nil {
		return "", err
	}

	// 2. Add language instruction if output is not French (template's native language)
	if outputLang != "" && !IsFrench(outputLang) {
		langName := LanguageDisplayName(outputLang)
		prompt = fmt.Sprintf("Respond in %s.\n\n%s", langName, prompt)
	}

	// 3. Estimate tokens and check limit
	estimatedTokens := estimateTokens(transcript)
	if estimatedTokens > r.maxInputTokens {
		return "", fmt.Errorf("transcript too long (%dK tokens estimated, max %dK): %w",
			estimatedTokens/1000, r.maxInputTokens/1000, ErrTranscriptTooLong)
	}

	// 4. Build request
	req := openai.ChatCompletionRequest{
		Model: r.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: prompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: transcript,
			},
		},
		Temperature: 0, // Deterministic output for reproducibility
	}

	// 5. Call API with retry
	return r.restructureWithRetry(ctx, req)
}

// restructureWithRetry executes the restructuring with exponential backoff retry.
func (r *OpenAIRestructurer) restructureWithRetry(ctx context.Context, req openai.ChatCompletionRequest) (string, error) {
	cfg := retryConfig{
		maxRetries: r.maxRetries,
		baseDelay:  r.baseDelay,
		maxDelay:   r.maxDelay,
	}

	return retryWithBackoff(ctx, cfg, func() (string, error) {
		resp, err := r.client.CreateChatCompletion(ctx, req)
		if err != nil {
			return "", classifyRestructureError(err)
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("no response from API")
		}
		return resp.Choices[0].Message.Content, nil
	}, isRetryableRestructureError)
}

// estimateTokens estimates the number of tokens in a text.
// Uses len/3 as a conservative estimate for French text.
// English averages ~4 chars/token, French ~3.5 chars/token.
// We use 3 to err on the side of caution.
func estimateTokens(text string) int {
	return len(text) / defaultCharsPerToken
}

// classifyRestructureError maps OpenAI API errors to sentinel errors.
// Uses errors.As for robust error type checking instead of string matching.
func classifyRestructureError(err error) error {
	if err == nil {
		return nil
	}

	// Check for typed API errors first (most reliable).
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatusCode {
		case http.StatusTooManyRequests:
			return fmt.Errorf("%s: %w", apiErr.Message, ErrRateLimit)
		case http.StatusUnauthorized:
			return fmt.Errorf("%s: %w", apiErr.Message, ErrAuthFailed)
		case http.StatusRequestTimeout, http.StatusGatewayTimeout:
			return fmt.Errorf("%s: %w", apiErr.Message, ErrTimeout)
		case http.StatusBadRequest:
			// Check for context length exceeded in message.
			if strings.Contains(apiErr.Message, "context_length") ||
				strings.Contains(apiErr.Message, "maximum context length") {
				return fmt.Errorf("API rejected: %w", ErrTranscriptTooLong)
			}
		}
	}

	// Check for context timeout/deadline exceeded.
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("request timed out: %w", ErrTimeout)
	}

	// Fallback: check error message for context length (some errors may not be typed).
	errStr := err.Error()
	if strings.Contains(errStr, "context_length_exceeded") ||
		strings.Contains(errStr, "maximum context length") {
		return fmt.Errorf("API rejected: %w", ErrTranscriptTooLong)
	}

	return err
}

// isRetryableRestructureError determines if an error is transient and should be retried.
func isRetryableRestructureError(err error) bool {
	// Rate limits are retryable (with backoff).
	if errors.Is(err, ErrRateLimit) {
		return true
	}

	// Timeouts are retryable.
	if errors.Is(err, ErrTimeout) {
		return true
	}

	// Server errors (5xx) are retryable.
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatusCode {
		case http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			return true
		}
	}

	// Context cancellation is not retryable.
	if errors.Is(err, context.Canceled) {
		return false
	}

	// Auth errors are not retryable.
	if errors.Is(err, ErrAuthFailed) {
		return false
	}

	// Transcript too long is not retryable.
	if errors.Is(err, ErrTranscriptTooLong) {
		return false
	}

	return false
}
