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
// OpenAITranscriber.Transcribe Tests
// =============================================================================

func TestTranscribe_Success(t *testing.T) {
	mock := withAudioSuccess("Hello, world!")
	transcriber := NewOpenAITranscriber(nil, withAudioTranscriber(mock))

	result, err := transcriber.Transcribe(context.Background(), "/fake/audio.ogg", TranscribeOptions{})

	assertNoError(t, err)
	assertEqual(t, result, "Hello, world!")
	assertEqual(t, mock.CallCount(), 1)
}

func TestTranscribe_RequestBuilding(t *testing.T) {
	tests := []struct {
		name         string
		opts         TranscribeOptions
		wantModel    string
		wantFormat   openai.AudioResponseFormat
		wantLanguage string
		wantPrompt   string
	}{
		{
			name:         "default options",
			opts:         TranscribeOptions{},
			wantModel:    ModelGPT4oMiniTranscribe,
			wantFormat:   openai.AudioResponseFormatJSON,
			wantLanguage: "",
			wantPrompt:   "",
		},
		{
			name:         "with language",
			opts:         TranscribeOptions{Language: "pt-BR"},
			wantModel:    ModelGPT4oMiniTranscribe,
			wantFormat:   openai.AudioResponseFormatJSON,
			wantLanguage: "pt", // BaseLanguageCode extracts base
			wantPrompt:   "",
		},
		{
			name:         "with prompt",
			opts:         TranscribeOptions{Prompt: "Technical discussion about Kubernetes"},
			wantModel:    ModelGPT4oMiniTranscribe,
			wantFormat:   openai.AudioResponseFormatJSON,
			wantLanguage: "",
			wantPrompt:   "Technical discussion about Kubernetes",
		},
		{
			name:         "with diarization",
			opts:         TranscribeOptions{Diarize: true},
			wantModel:    ModelGPT4oTranscribeDiarize,
			wantFormat:   FormatDiarizedJSON,
			wantLanguage: "",
			wantPrompt:   "",
		},
		{
			name:         "all options combined",
			opts:         TranscribeOptions{Diarize: true, Language: "fr-CA", Prompt: "Meeting notes"},
			wantModel:    ModelGPT4oTranscribeDiarize,
			wantFormat:   FormatDiarizedJSON,
			wantLanguage: "fr",
			wantPrompt:   "Meeting notes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := withAudioSuccess("transcribed text")
			transcriber := NewOpenAITranscriber(nil, withAudioTranscriber(mock))

			_, err := transcriber.Transcribe(context.Background(), "/fake/audio.ogg", tt.opts)
			assertNoError(t, err)

			req := mock.LastRequest()
			if req == nil {
				t.Fatal("expected request to be captured")
			}

			assertEqual(t, req.Model, tt.wantModel)
			assertEqual(t, req.Format, tt.wantFormat)
			assertEqual(t, req.Language, tt.wantLanguage)
			assertEqual(t, req.Prompt, tt.wantPrompt)
			assertEqual(t, req.FilePath, "/fake/audio.ogg")
		})
	}
}

// TestTranscribe_Diarization tests the diarized response formatting.
// NOTE (V1): go-openai does not yet expose speaker information.
// Current format is "[Segment N] text" - will change to "[Speaker N] text"
// when go-openai adds proper diarization support.
func TestTranscribe_Diarization(t *testing.T) {
	tests := []struct {
		name       string
		segments   [][2]any
		fallback   string
		wantResult string
	}{
		{
			name: "formats segments",
			segments: [][2]any{
				{0, "Hello from segment zero"},
				{1, "And segment one"},
			},
			wantResult: "[Segment 0] Hello from segment zero\n[Segment 1] And segment one\n",
		},
		{
			name: "single segment",
			segments: [][2]any{
				{0, "Only one segment"},
			},
			wantResult: "[Segment 0] Only one segment\n",
		},
		{
			name:       "empty segments falls back to text",
			segments:   [][2]any{},
			fallback:   "Fallback text when no segments",
			wantResult: "Fallback text when no segments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build mock response
			resp := openai.AudioResponse{Text: tt.fallback}
			for _, seg := range tt.segments {
				id, _ := seg[0].(int)
				text, _ := seg[1].(string)
				resp.Segments = append(resp.Segments, struct {
					ID               int     `json:"id"`
					Seek             int     `json:"seek"`
					Start            float64 `json:"start"`
					End              float64 `json:"end"`
					Text             string  `json:"text"`
					Tokens           []int   `json:"tokens"`
					Temperature      float64 `json:"temperature"`
					AvgLogprob       float64 `json:"avg_logprob"`
					CompressionRatio float64 `json:"compression_ratio"`
					NoSpeechProb     float64 `json:"no_speech_prob"`
					Transient        bool    `json:"transient"`
				}{ID: id, Text: text})
			}

			mock := newMockAudioTranscriber(mockAudioResponse{response: resp})
			transcriber := NewOpenAITranscriber(nil, withAudioTranscriber(mock))

			result, err := transcriber.Transcribe(context.Background(), "/fake/audio.ogg", TranscribeOptions{Diarize: true})

			assertNoError(t, err)
			assertEqual(t, result, tt.wantResult)
		})
	}
}

// =============================================================================
// Retry Logic Tests
// =============================================================================

func TestTranscribe_Retry(t *testing.T) {
	tests := []struct {
		name           string
		responses      []mockAudioResponse
		wantCalls      int
		wantError      error
		wantSuccess    bool
		wantResultText string
	}{
		{
			name: "retries on rate limit then succeeds",
			responses: []mockAudioResponse{
				{err: apiError(http.StatusTooManyRequests, "rate limit exceeded")},
				{response: openai.AudioResponse{Text: "success after retry"}},
			},
			wantCalls:      2,
			wantSuccess:    true,
			wantResultText: "success after retry",
		},
		{
			name: "retries on timeout then succeeds",
			responses: []mockAudioResponse{
				{err: apiError(http.StatusRequestTimeout, "request timeout")},
				{response: openai.AudioResponse{Text: "success after timeout"}},
			},
			wantCalls:      2,
			wantSuccess:    true,
			wantResultText: "success after timeout",
		},
		{
			name: "retries on 500 then succeeds",
			responses: []mockAudioResponse{
				{err: apiError(http.StatusInternalServerError, "internal server error")},
				{response: openai.AudioResponse{Text: "success after 500"}},
			},
			wantCalls:      2,
			wantSuccess:    true,
			wantResultText: "success after 500",
		},
		{
			name: "retries on 502 then succeeds",
			responses: []mockAudioResponse{
				{err: apiError(http.StatusBadGateway, "bad gateway")},
				{response: openai.AudioResponse{Text: "success after 502"}},
			},
			wantCalls:      2,
			wantSuccess:    true,
			wantResultText: "success after 502",
		},
		{
			name: "retries on 503 then succeeds",
			responses: []mockAudioResponse{
				{err: apiError(http.StatusServiceUnavailable, "service unavailable")},
				{response: openai.AudioResponse{Text: "success after 503"}},
			},
			wantCalls:      2,
			wantSuccess:    true,
			wantResultText: "success after 503",
		},
		{
			name: "retries on 504 then succeeds",
			responses: []mockAudioResponse{
				{err: apiError(http.StatusGatewayTimeout, "gateway timeout")},
				{response: openai.AudioResponse{Text: "success after 504"}},
			},
			wantCalls:      2,
			wantSuccess:    true,
			wantResultText: "success after 504",
		},
		{
			name: "no retry on 401 auth failed",
			responses: []mockAudioResponse{
				{err: apiError(http.StatusUnauthorized, "invalid api key")},
			},
			wantCalls:   1,
			wantSuccess: false,
			wantError:   ErrAuthFailed,
		},
		{
			name: "no retry on quota exceeded",
			responses: []mockAudioResponse{
				{err: apiError(http.StatusTooManyRequests, "quota exceeded")},
			},
			wantCalls:   1,
			wantSuccess: false,
			wantError:   ErrQuotaExceeded,
		},
		{
			name: "no retry on billing issue",
			responses: []mockAudioResponse{
				{err: apiError(http.StatusTooManyRequests, "billing limit reached")},
			},
			wantCalls:   1,
			wantSuccess: false,
			wantError:   ErrQuotaExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := withAudioSequence(tt.responses...)
			transcriber := NewOpenAITranscriber(nil,
				withAudioTranscriber(mock),
				WithRetryDelays(1*time.Millisecond, 1*time.Millisecond),
			)

			result, err := transcriber.Transcribe(context.Background(), "/fake/audio.ogg", TranscribeOptions{})

			assertEqual(t, mock.CallCount(), tt.wantCalls)

			if tt.wantSuccess {
				assertNoError(t, err)
				assertEqual(t, result, tt.wantResultText)
			} else {
				assertError(t, err, tt.wantError)
			}
		})
	}
}

func TestTranscribe_FailsAfterMaxRetries(t *testing.T) {
	// Use maxRetries=2 for faster test (3 total attempts)
	mock := withAudioError(apiError(http.StatusTooManyRequests, "rate limit"))
	transcriber := NewOpenAITranscriber(nil,
		withAudioTranscriber(mock),
		WithMaxRetries(2),
		WithRetryDelays(1*time.Millisecond, 1*time.Millisecond),
	)

	_, err := transcriber.Transcribe(context.Background(), "/fake/audio.ogg", TranscribeOptions{})

	// 1 initial + 2 retries = 3 calls
	assertEqual(t, mock.CallCount(), 3)

	if err == nil {
		t.Fatal("expected error after max retries")
	}
	assertContains(t, err.Error(), "max retries (2) exceeded")
	assertError(t, err, ErrRateLimit)
}

func TestTranscribe_NoRetryOnContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Mock returns context.Canceled error (simulating cancelled request)
	mock := withAudioError(context.Canceled)
	transcriber := NewOpenAITranscriber(nil,
		withAudioTranscriber(mock),
		WithRetryDelays(1*time.Millisecond, 1*time.Millisecond),
	)

	_, err := transcriber.Transcribe(ctx, "/fake/audio.ogg", TranscribeOptions{})

	// Should not retry on context.Canceled
	assertEqual(t, mock.CallCount(), 1)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestTranscribe_ContextCancellationDuringRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// First call fails with retryable error, then we cancel
	callCount := 0
	mock := newMockAudioTranscriber()
	mock.responses = []mockAudioResponse{
		{err: apiError(http.StatusTooManyRequests, "rate limit")},
		{response: openai.AudioResponse{Text: "should not reach"}},
	}

	// Intercept to cancel after first call
	originalMock := mock
	interceptingMock := &mockAudioTranscriber{}
	interceptingMock.responses = originalMock.responses
	interceptingMock.calls = originalMock.calls

	transcriber := NewOpenAITranscriber(nil,
		withAudioTranscriber(&cancellingMock{
			inner:     mock,
			cancelFn:  cancel,
			cancelAt:  1,
			callCount: &callCount,
		}),
		WithRetryDelays(50*time.Millisecond, 50*time.Millisecond), // Enough time to cancel
	)

	_, err := transcriber.Transcribe(ctx, "/fake/audio.ogg", TranscribeOptions{})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// cancellingMock wraps a mock and cancels context after N calls
type cancellingMock struct {
	inner     *mockAudioTranscriber
	cancelFn  context.CancelFunc
	cancelAt  int
	callCount *int
}

func (m *cancellingMock) CreateTranscription(ctx context.Context, req openai.AudioRequest) (openai.AudioResponse, error) {
	*m.callCount++
	if *m.callCount >= m.cancelAt {
		m.cancelFn()
	}
	return m.inner.CreateTranscription(ctx, req)
}

// =============================================================================
// classifyError Tests
// =============================================================================

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name      string
		inputErr  error
		wantError error
	}{
		{
			name:      "429 rate limit",
			inputErr:  apiError(http.StatusTooManyRequests, "rate limit exceeded"),
			wantError: ErrRateLimit,
		},
		{
			name:      "429 with quota in message",
			inputErr:  apiError(http.StatusTooManyRequests, "quota exceeded"),
			wantError: ErrQuotaExceeded,
		},
		{
			name:      "429 with billing in message",
			inputErr:  apiError(http.StatusTooManyRequests, "billing limit"),
			wantError: ErrQuotaExceeded,
		},
		{
			name:      "401 unauthorized",
			inputErr:  apiError(http.StatusUnauthorized, "invalid api key"),
			wantError: ErrAuthFailed,
		},
		{
			name:      "408 request timeout",
			inputErr:  apiError(http.StatusRequestTimeout, "timeout"),
			wantError: ErrTimeout,
		},
		{
			name:      "504 gateway timeout",
			inputErr:  apiError(http.StatusGatewayTimeout, "gateway timeout"),
			wantError: ErrTimeout,
		},
		{
			name:      "context deadline exceeded",
			inputErr:  context.DeadlineExceeded,
			wantError: ErrTimeout,
		},
		{
			name:      "unknown error passes through",
			inputErr:  errors.New("some random error"),
			wantError: nil, // No sentinel, error passes through
		},
		{
			name:      "500 internal server error - no classification",
			inputErr:  apiError(http.StatusInternalServerError, "internal error"),
			wantError: nil, // 500 is not classified to a sentinel, just retryable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.inputErr)

			if tt.wantError == nil {
				// For unclassified errors, verify original error is preserved
				if tt.inputErr != result && !errors.Is(result, tt.inputErr) {
					// It's ok if the error was wrapped but original is inside
					if !strings.Contains(result.Error(), tt.inputErr.Error()) {
						t.Errorf("expected original error to be preserved, got %v", result)
					}
				}
			} else {
				assertError(t, result, tt.wantError)
			}
		})
	}
}

// =============================================================================
// isRetryableError Tests
// =============================================================================

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		inputErr  error
		wantRetry bool
	}{
		{
			name:      "ErrRateLimit is retryable",
			inputErr:  ErrRateLimit,
			wantRetry: true,
		},
		{
			name:      "wrapped ErrRateLimit is retryable",
			inputErr:  errors.Join(errors.New("context"), ErrRateLimit),
			wantRetry: true,
		},
		{
			name:      "ErrTimeout is retryable",
			inputErr:  ErrTimeout,
			wantRetry: true,
		},
		{
			name:      "500 internal server error is retryable",
			inputErr:  apiError(http.StatusInternalServerError, "internal"),
			wantRetry: true,
		},
		{
			name:      "502 bad gateway is retryable",
			inputErr:  apiError(http.StatusBadGateway, "bad gateway"),
			wantRetry: true,
		},
		{
			name:      "503 service unavailable is retryable",
			inputErr:  apiError(http.StatusServiceUnavailable, "unavailable"),
			wantRetry: true,
		},
		{
			name:      "504 gateway timeout is retryable",
			inputErr:  apiError(http.StatusGatewayTimeout, "timeout"),
			wantRetry: true,
		},
		{
			name:      "ErrAuthFailed is not retryable",
			inputErr:  ErrAuthFailed,
			wantRetry: false,
		},
		{
			name:      "ErrQuotaExceeded is not retryable",
			inputErr:  ErrQuotaExceeded,
			wantRetry: false,
		},
		{
			name:      "context.Canceled is not retryable",
			inputErr:  context.Canceled,
			wantRetry: false,
		},
		{
			name:      "random error is not retryable",
			inputErr:  errors.New("some error"),
			wantRetry: false,
		},
		{
			name:      "400 bad request is not retryable",
			inputErr:  apiError(http.StatusBadRequest, "bad request"),
			wantRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.inputErr)
			assertEqual(t, result, tt.wantRetry)
		})
	}
}

// =============================================================================
// TranscribeAll Tests
// =============================================================================

func TestTranscribeAll_EmptyChunks(t *testing.T) {
	mock := withAudioSuccess("should not be called")
	transcriber := NewOpenAITranscriber(nil, withAudioTranscriber(mock))

	results, err := TranscribeAll(context.Background(), nil, transcriber, TranscribeOptions{}, 3)

	assertNoError(t, err)
	if results != nil {
		t.Errorf("expected nil results for empty chunks, got %v", results)
	}
	assertEqual(t, mock.CallCount(), 0)
}

func TestTranscribeAll_SingleChunk(t *testing.T) {
	mock := withAudioSuccess("single chunk result")
	transcriber := NewOpenAITranscriber(nil, withAudioTranscriber(mock))

	chunks := []Chunk{
		{Path: "/fake/chunk0.ogg", Index: 0},
	}

	results, err := TranscribeAll(context.Background(), chunks, transcriber, TranscribeOptions{}, 3)

	assertNoError(t, err)
	assertEqual(t, len(results), 1)
	assertEqual(t, results[0], "single chunk result")
	assertEqual(t, mock.CallCount(), 1)
}

func TestTranscribeAll_PreservesOrder(t *testing.T) {
	// Mock returns the file path as the result to verify ordering
	mock := &pathReturningMock{}
	transcriber := &pathReturningTranscriber{mock: mock}

	chunks := []Chunk{
		{Path: "/fake/chunk0.ogg", Index: 0},
		{Path: "/fake/chunk1.ogg", Index: 1},
		{Path: "/fake/chunk2.ogg", Index: 2},
		{Path: "/fake/chunk3.ogg", Index: 3},
		{Path: "/fake/chunk4.ogg", Index: 4},
	}

	results, err := TranscribeAll(context.Background(), chunks, transcriber, TranscribeOptions{}, 2)

	assertNoError(t, err)
	assertEqual(t, len(results), 5)

	// Verify each result matches its chunk's path (order preserved)
	for i, chunk := range chunks {
		assertEqual(t, results[i], chunk.Path)
	}
}

// pathReturningMock returns the file path as transcription result
type pathReturningMock struct{}

type pathReturningTranscriber struct {
	mock *pathReturningMock
}

func (t *pathReturningTranscriber) Transcribe(_ context.Context, audioPath string, _ TranscribeOptions) (string, error) {
	return audioPath, nil
}

func TestTranscribeAll_InvalidMaxParallel(t *testing.T) {
	tests := []struct {
		name        string
		maxParallel int
	}{
		{"negative", -1},
		{"zero", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := withAudioSuccess("result")
			transcriber := NewOpenAITranscriber(nil, withAudioTranscriber(mock))

			chunks := []Chunk{
				{Path: "/fake/chunk0.ogg", Index: 0},
				{Path: "/fake/chunk1.ogg", Index: 1},
			}

			results, err := TranscribeAll(context.Background(), chunks, transcriber, TranscribeOptions{}, tt.maxParallel)

			// Should work despite invalid maxParallel (defaults to 1)
			assertNoError(t, err)
			assertEqual(t, len(results), 2)
		})
	}
}

func TestTranscribeAll_ErrorPropagation(t *testing.T) {
	// Use a failing transcriber that always returns auth error
	failingTranscriber := &failingOnIndexTranscriber{
		failIndex: 1,
		failError: apiError(http.StatusUnauthorized, "auth failed"),
	}

	chunks := []Chunk{
		{Path: "/fake/chunk0.ogg", Index: 0},
		{Path: "/fake/chunk1.ogg", Index: 1},
		{Path: "/fake/chunk2.ogg", Index: 2},
	}

	_, err := TranscribeAll(context.Background(), chunks, failingTranscriber, TranscribeOptions{}, 1)

	if err == nil {
		t.Fatal("expected error")
	}

	// Error should contain chunk info (chunk.Index from the Chunk struct)
	assertContains(t, err.Error(), "chunk 1")
	assertContains(t, err.Error(), "/fake/chunk1.ogg")
	assertError(t, err, ErrAuthFailed)
}

// failingOnIndexTranscriber fails on a specific chunk index
type failingOnIndexTranscriber struct {
	failIndex int
	failError error
}

func (t *failingOnIndexTranscriber) Transcribe(_ context.Context, audioPath string, _ TranscribeOptions) (string, error) {
	// Extract index from path (simplified: check if path contains the fail index)
	if strings.Contains(audioPath, "chunk1") {
		return "", classifyError(t.failError)
	}
	return "success", nil
}

func TestTranscribeAll_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create a mock that blocks until context is cancelled
	blockingMock := &blockingTranscriber{
		cancelFn: cancel,
		results:  []string{"result0", "result1", "result2"},
	}

	chunks := []Chunk{
		{Path: "/fake/chunk0.ogg", Index: 0},
		{Path: "/fake/chunk1.ogg", Index: 1},
		{Path: "/fake/chunk2.ogg", Index: 2},
	}

	_, err := TranscribeAll(ctx, chunks, blockingMock, TranscribeOptions{}, 1)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// blockingTranscriber cancels context after first call
type blockingTranscriber struct {
	cancelFn  context.CancelFunc
	results   []string
	callCount int
}

func (t *blockingTranscriber) Transcribe(ctx context.Context, _ string, _ TranscribeOptions) (string, error) {
	if t.callCount == 0 {
		t.callCount++
		t.cancelFn()
		// Return first result, but next call will see cancelled context
		return t.results[0], nil
	}
	// Check if context is cancelled
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
		result := t.results[t.callCount]
		t.callCount++
		return result, nil
	}
}

// =============================================================================
// formatDiarizedResponse Tests (direct unit test)
// =============================================================================

func TestFormatDiarizedResponse(t *testing.T) {
	tests := []struct {
		name     string
		response openai.AudioResponse
		want     string
	}{
		{
			name: "multiple segments",
			response: openai.AudioResponse{
				Segments: []struct {
					ID               int     `json:"id"`
					Seek             int     `json:"seek"`
					Start            float64 `json:"start"`
					End              float64 `json:"end"`
					Text             string  `json:"text"`
					Tokens           []int   `json:"tokens"`
					Temperature      float64 `json:"temperature"`
					AvgLogprob       float64 `json:"avg_logprob"`
					CompressionRatio float64 `json:"compression_ratio"`
					NoSpeechProb     float64 `json:"no_speech_prob"`
					Transient        bool    `json:"transient"`
				}{
					{ID: 0, Text: "First segment"},
					{ID: 1, Text: "Second segment"},
					{ID: 2, Text: "Third segment"},
				},
			},
			want: "[Segment 0] First segment\n[Segment 1] Second segment\n[Segment 2] Third segment\n",
		},
		{
			name: "empty segments returns Text field",
			response: openai.AudioResponse{
				Text:     "Fallback text",
				Segments: nil,
			},
			want: "Fallback text",
		},
		{
			name: "segment with special characters",
			response: openai.AudioResponse{
				Segments: []struct {
					ID               int     `json:"id"`
					Seek             int     `json:"seek"`
					Start            float64 `json:"start"`
					End              float64 `json:"end"`
					Text             string  `json:"text"`
					Tokens           []int   `json:"tokens"`
					Temperature      float64 `json:"temperature"`
					AvgLogprob       float64 `json:"avg_logprob"`
					CompressionRatio float64 `json:"compression_ratio"`
					NoSpeechProb     float64 `json:"no_speech_prob"`
					Transient        bool    `json:"transient"`
				}{
					{ID: 0, Text: "Text with \"quotes\" and 'apostrophes'"},
				},
			},
			want: "[Segment 0] Text with \"quotes\" and 'apostrophes'\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDiarizedResponse(tt.response)
			assertEqual(t, result, tt.want)
		})
	}
}
