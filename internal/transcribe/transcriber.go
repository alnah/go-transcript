package transcribe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"golang.org/x/sync/errgroup"

	"github.com/alnah/go-transcript/internal/audio"
	"github.com/alnah/go-transcript/internal/lang"
)

// OpenAI transcription model and format identifiers.
// These are not yet defined in go-openai, so we define them locally.
const (
	// ModelGPT4oMiniTranscribe is the cost-effective transcription model ($0.003/min).
	ModelGPT4oMiniTranscribe = "gpt-4o-mini-transcribe"

	// ModelGPT4oTranscribeDiarize is the transcription model with speaker identification.
	ModelGPT4oTranscribeDiarize = "gpt-4o-transcribe-diarize"

	// FormatDiarizedJSON is the response format for diarized transcription.
	// Not yet a constant in go-openai.
	FormatDiarizedJSON openai.AudioResponseFormat = "diarized_json"
)

// Parallelism configuration.
const (
	// MaxRecommendedParallel is the recommended upper limit for concurrent API requests.
	// Higher values may trigger rate limiting.
	MaxRecommendedParallel = 10
)

// Default retry configuration per specification.
const (
	defaultMaxRetries = 5
	defaultBaseDelay  = 1 * time.Second
	defaultMaxDelay   = 30 * time.Second
)

// RetryConfig holds retry parameters for exponential backoff.
// Exported so other packages (e.g., restructure) can use RetryWithBackoff.
//
// All fields must be non-negative. Invalid values are normalized:
// - MaxRetries < 0 becomes 0 (single attempt)
// - BaseDelay <= 0 becomes 1ms
// - MaxDelay <= 0 becomes BaseDelay
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// normalize ensures all RetryConfig fields have valid values.
func (c *RetryConfig) normalize() {
	if c.MaxRetries < 0 {
		c.MaxRetries = 0
	}
	if c.BaseDelay <= 0 {
		c.BaseDelay = time.Millisecond
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = c.BaseDelay
	}
}

// RetryWithBackoff executes fn with exponential backoff retry.
// It retries only if shouldRetry returns true for the error.
// Returns the result of the last attempt.
//
// Invalid RetryConfig values are normalized (see RetryConfig documentation).
func RetryWithBackoff[T any](
	ctx context.Context,
	cfg RetryConfig,
	fn func() (T, error),
	shouldRetry func(error) bool,
) (T, error) {
	cfg.normalize()

	var zero T
	var lastErr error
	delay := cfg.BaseDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return zero, ctx.Err()
			case <-timer.C:
			}
			// Exponential backoff with cap.
			delay = min(delay*2, cfg.MaxDelay)
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err
		if !shouldRetry(lastErr) {
			return zero, lastErr
		}
	}

	return zero, fmt.Errorf("max retries (%d) exceeded: %w", cfg.MaxRetries, lastErr)
}

// Options configures transcription behavior.
type Options struct {
	// Diarize enables speaker identification in the transcript.
	// When true, uses gpt-4o-transcribe-diarize model.
	//
	// LIMITATION (V1): The go-openai library does not yet support proper speaker
	// diarization parsing. Output is formatted as "[Segment N] text" rather than
	// "[Speaker N] text". This will be updated when go-openai adds support.
	// TODO(diarization): Track upstream support at https://github.com/sashabaranov/go-openai/issues/924
	Diarize bool

	// Prompt provides context to improve transcription accuracy.
	// Useful for domain-specific vocabulary, acronyms, or expected content.
	// Example: "Technical discussion about Kubernetes and Docker containers."
	// Note: Prompt can also hint at the language if Language is not set.
	Prompt string

	// Language specifies the audio language.
	// Zero value means auto-detect (recommended for most use cases).
	Language lang.Language
}

// Transcriber transcribes audio files to text.
type Transcriber interface {
	// Transcribe converts an audio file to text.
	// audioPath must be a file in a supported format: mp3, mp4, mpeg, mpga, m4a, wav, webm, ogg.
	// Returns the transcribed text or an error.
	Transcribe(ctx context.Context, audioPath string, opts Options) (string, error)
}

// audioTranscriber is an internal interface for OpenAI audio transcription.
// *openai.Client implements this implicitly.
// This allows injecting mocks in tests.
type audioTranscriber interface {
	CreateTranscription(ctx context.Context, req openai.AudioRequest) (openai.AudioResponse, error)
}

// Compile-time interface compliance checks.
var (
	_ Transcriber      = (*OpenAITranscriber)(nil)
	_ audioTranscriber = (*openai.Client)(nil)
)

// OpenAITranscriber transcribes audio using OpenAI's transcription API.
// It supports automatic retries with exponential backoff for transient errors.
type OpenAITranscriber struct {
	client     audioTranscriber
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
}

// TranscriberOption configures an OpenAITranscriber.
type TranscriberOption func(*OpenAITranscriber)

// WithMaxRetries sets the maximum number of retry attempts.
func WithMaxRetries(n int) TranscriberOption {
	return func(t *OpenAITranscriber) {
		if n >= 0 {
			t.maxRetries = n
		}
	}
}

// WithRetryDelays sets the base and max delays for exponential backoff.
func WithRetryDelays(base, max time.Duration) TranscriberOption {
	return func(t *OpenAITranscriber) {
		if base > 0 {
			t.baseDelay = base
		}
		if max > 0 {
			t.maxDelay = max
		}
	}
}

// NewOpenAITranscriber creates a new OpenAITranscriber.
// The client is injected to enable testing with mocks.
func NewOpenAITranscriber(client *openai.Client, opts ...TranscriberOption) *OpenAITranscriber {
	t := &OpenAITranscriber{
		client:     client,
		maxRetries: defaultMaxRetries,
		baseDelay:  defaultBaseDelay,
		maxDelay:   defaultMaxDelay,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Transcribe transcribes an audio file using OpenAI's API.
// It automatically retries on transient errors (rate limits, timeouts, server errors).
func (t *OpenAITranscriber) Transcribe(ctx context.Context, audioPath string, opts Options) (string, error) {
	model := ModelGPT4oMiniTranscribe
	format := openai.AudioResponseFormatJSON

	// Diarization requires a different model and response format.
	if opts.Diarize {
		model = ModelGPT4oTranscribeDiarize
		format = FormatDiarizedJSON
	}

	req := openai.AudioRequest{
		Model:    model,
		FilePath: audioPath,
		Format:   format,
		Prompt:   opts.Prompt,
		Language: opts.Language.BaseCode(), // OpenAI only accepts ISO 639-1 base codes
	}

	return t.transcribeWithRetry(ctx, req, opts.Diarize)
}

// transcribeWithRetry executes the transcription with exponential backoff retry.
func (t *OpenAITranscriber) transcribeWithRetry(ctx context.Context, req openai.AudioRequest, diarize bool) (string, error) {
	cfg := RetryConfig{
		MaxRetries: t.maxRetries,
		BaseDelay:  t.baseDelay,
		MaxDelay:   t.maxDelay,
	}

	return RetryWithBackoff(ctx, cfg, func() (string, error) {
		resp, err := t.client.CreateTranscription(ctx, req)
		if err != nil {
			return "", classifyError(err)
		}
		if diarize {
			return formatDiarizedResponse(resp), nil
		}
		return resp.Text, nil
	}, isRetryableError)
}

// formatDiarizedResponse formats a diarized transcript response.
//
// LIMITATION (V1): The go-openai library does not yet expose speaker information
// from the diarized_json response format. We format segments with their IDs as a
// fallback. When proper speaker diarization support is added to go-openai, this
// function should be updated to use "[Speaker N]" format instead of "[Segment N]".
//
// Expected future format: "[Speaker 1] text\n[Speaker 2] text\n..."
// Current format: "[Segment 0] text\n[Segment 1] text\n..."
//
// TODO(diarization): Track upstream support at https://github.com/sashabaranov/go-openai/issues/924
func formatDiarizedResponse(resp openai.AudioResponse) string {
	if len(resp.Segments) == 0 {
		return resp.Text
	}

	var b strings.Builder
	for _, seg := range resp.Segments {
		fmt.Fprintf(&b, "[Segment %d] %s\n", seg.ID, seg.Text)
	}
	return b.String()
}

// classifyError maps OpenAI API errors to sentinel errors.
func classifyError(err error) error {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatusCode {
		case http.StatusTooManyRequests:
			// Distinguish between temporary rate limit and quota exceeded (billing issue).
			// Quota exceeded should not be retried - it requires user action.
			if strings.Contains(apiErr.Message, "quota") ||
				strings.Contains(apiErr.Message, "billing") {
				return fmt.Errorf("%s: %w", apiErr.Message, ErrQuotaExceeded)
			}
			return fmt.Errorf("%s: %w", apiErr.Message, ErrRateLimit)
		case http.StatusUnauthorized:
			return fmt.Errorf("%s: %w", apiErr.Message, ErrAuthFailed)
		case http.StatusRequestTimeout, http.StatusGatewayTimeout:
			return fmt.Errorf("%s: %w", apiErr.Message, ErrTimeout)
		case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound:
			return fmt.Errorf("%s: %w", apiErr.Message, ErrBadRequest)
		}
	}

	// Check for context timeout/deadline exceeded.
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("request timed out: %w", ErrTimeout)
	}

	return err
}

// isRetryableError determines if an error is transient and should be retried.
func isRetryableError(err error) bool {
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

	return false
}

// TranscribeAll transcribes multiple audio chunks in parallel.
// Results are returned in the same order as the input chunks.
// If any chunk fails, the entire operation is aborted and the error is returned.
// maxParallel limits the number of concurrent API requests (1-MaxRecommendedParallel recommended).
func TranscribeAll(
	ctx context.Context,
	chunks []audio.Chunk,
	t Transcriber,
	opts Options,
	maxParallel int,
) ([]string, error) {
	if len(chunks) == 0 {
		return nil, nil
	}

	if maxParallel < 1 {
		maxParallel = 1
	}

	results := make([]string, len(chunks))
	// Semaphore channel for concurrency control.
	// Not closed explicitly: it's local to this function and will be GC'd.
	sem := make(chan struct{}, maxParallel)

	g, ctx := errgroup.WithContext(ctx)

	for i, chunk := range chunks {
		g.Go(func() error {
			// Acquire semaphore slot.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}
			defer func() { <-sem }()

			text, err := t.Transcribe(ctx, chunk.Path, opts)
			if err != nil {
				return fmt.Errorf("chunk %d (%s): %w", chunk.Index, filepath.Base(chunk.Path), err)
			}
			results[i] = text
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}
