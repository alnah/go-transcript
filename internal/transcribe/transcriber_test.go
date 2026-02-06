package transcribe_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/alnah/go-transcript/internal/audio"
	"github.com/alnah/go-transcript/internal/lang"
	"github.com/alnah/go-transcript/internal/transcribe"
)

// Notes:
// - Black-box testing via package transcribe_test.
// - Uses export_test.go to inject mock audioTranscriber.
// - Tests use short delays (1ms) to avoid slow tests while still exercising backoff.
// - Parallelism tests use channel-based synchronization, not timing.
//
// Coverage gaps (intentional):
// - Exact backoff timing (1s, 2s, 4s...) - implementation detail.
// - Precise maxParallel verification - only smoke-tested via channel blocking.
// - Network I/O with real OpenAI client - requires integration tests.

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// mockAudioTranscriber implements audioTranscriber for testing.
type mockAudioTranscriber struct {
	mu        sync.Mutex
	calls     []openai.AudioRequest
	responses []openai.AudioResponse
	errors    []error
	callIndex int
}

func (m *mockAudioTranscriber) CreateTranscription(ctx context.Context, req openai.AudioRequest) (openai.AudioResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, req)

	idx := m.callIndex
	m.callIndex++

	if idx < len(m.errors) && m.errors[idx] != nil {
		return openai.AudioResponse{}, m.errors[idx]
	}
	if idx < len(m.responses) {
		return m.responses[idx], nil
	}
	return openai.AudioResponse{}, nil
}

func (m *mockAudioTranscriber) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockAudioTranscriber) LastRequest() openai.AudioRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return openai.AudioRequest{}
	}
	return m.calls[len(m.calls)-1]
}

// mockHTTPClient implements httpDoer for testing diarization HTTP calls.
type mockHTTPClient struct {
	mu              sync.Mutex
	requests        []*http.Request
	requestBodies   [][]byte // Captured request bodies
	responses       []*http.Response
	errors          []error
	callIndex       int
	statusCode      int
	responseBody    string
	chunkingCapture string // Captured chunking_strategy value
}

func newMockHTTPClient(statusCode int, responseBody string) *mockHTTPClient {
	return &mockHTTPClient{
		statusCode:   statusCode,
		responseBody: responseBody,
	}
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Capture request body for verification
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		m.requestBodies = append(m.requestBodies, body)
		// Check for chunking_strategy in the multipart form
		if bytes.Contains(body, []byte("chunking_strategy")) {
			m.chunkingCapture = "found"
			// Extract value if possible
			if idx := bytes.Index(body, []byte("chunking_strategy")); idx != -1 {
				// Look for "auto" after the field name
				if bytes.Contains(body[idx:], []byte("auto")) {
					m.chunkingCapture = "auto"
				}
			}
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	m.requests = append(m.requests, req)

	idx := m.callIndex
	m.callIndex++

	if idx < len(m.errors) && m.errors[idx] != nil {
		return nil, m.errors[idx]
	}

	if idx < len(m.responses) {
		return m.responses[idx], nil
	}

	// Default response
	return &http.Response{
		StatusCode: m.statusCode,
		Body:       io.NopCloser(bytes.NewReader([]byte(m.responseBody))),
		Header:     make(http.Header),
	}, nil
}

func (m *mockHTTPClient) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

func (m *mockHTTPClient) HasChunkingStrategy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.chunkingCapture != ""
}

func (m *mockHTTPClient) ChunkingStrategyValue() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.chunkingCapture
}

// createTempAudioFile creates a temporary file for testing diarization.
// Returns the path and a cleanup function.
func createTempAudioFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ogg")
	if err := os.WriteFile(path, []byte("fake audio content"), 0644); err != nil {
		t.Fatalf("failed to create temp audio file: %v", err)
	}
	return path
}

// mockTranscriber implements transcribe.Transcriber for TranscribeAll tests.
type mockTranscriber struct {
	mu         sync.Mutex
	results    map[string]string
	errors     map[string]error
	blocking   chan struct{} // if set, blocks until closed
	started    chan struct{} // signals when a call starts
	concurrent int32         // atomic counter for concurrent calls
	maxConc    int32         // max concurrent calls observed
}

func newMockTranscriber() *mockTranscriber {
	return &mockTranscriber{
		results: make(map[string]string),
		errors:  make(map[string]error),
	}
}

func (m *mockTranscriber) Transcribe(ctx context.Context, audioPath string, opts transcribe.Options) (string, error) {
	// Track concurrent calls
	current := atomic.AddInt32(&m.concurrent, 1)
	defer atomic.AddInt32(&m.concurrent, -1)

	// Update max concurrent
	for {
		old := atomic.LoadInt32(&m.maxConc)
		if current <= old || atomic.CompareAndSwapInt32(&m.maxConc, old, current) {
			break
		}
	}

	// Signal that we started
	if m.started != nil {
		select {
		case m.started <- struct{}{}:
		default:
		}
	}

	// Block if blocking channel is set
	if m.blocking != nil {
		select {
		case <-m.blocking:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	m.mu.Lock()
	err := m.errors[audioPath]
	result := m.results[audioPath]
	m.mu.Unlock()

	if err != nil {
		return "", err
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// TestNewOpenAITranscriber - Constructor and options
// ---------------------------------------------------------------------------

func TestNewOpenAITranscriber(t *testing.T) {
	t.Parallel()

	t.Run("creates transcriber with defaults", func(t *testing.T) {
		t.Parallel()
		mock := &mockAudioTranscriber{
			responses: []openai.AudioResponse{{Text: "hello"}},
		}
		tr := transcribe.NewTestTranscriber(mock)

		result, err := tr.Transcribe(context.Background(), "test.mp3", transcribe.Options{})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}
		if result != "hello" {
			t.Errorf("got %q, want %q", result, "hello")
		}
	})
}

// ---------------------------------------------------------------------------
// TestTranscribe_Success - Successful transcription cases
// ---------------------------------------------------------------------------

func TestTranscribe_Success(t *testing.T) {
	t.Parallel()

	t.Run("returns text from response", func(t *testing.T) {
		t.Parallel()
		mock := &mockAudioTranscriber{
			responses: []openai.AudioResponse{{Text: "transcribed text"}},
		}
		tr := transcribe.NewTestTranscriber(mock)

		result, err := tr.Transcribe(context.Background(), "audio.mp3", transcribe.Options{})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}
		if result != "transcribed text" {
			t.Errorf("got %q, want %q", result, "transcribed text")
		}
	})

	t.Run("passes language as base code", func(t *testing.T) {
		t.Parallel()
		mock := &mockAudioTranscriber{
			responses: []openai.AudioResponse{{Text: "bonjour"}},
		}
		tr := transcribe.NewTestTranscriber(mock)

		_, err := tr.Transcribe(context.Background(), "audio.mp3", transcribe.Options{
			Language: lang.MustParse("fr-FR"), // Should be converted to "fr"
		})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}

		req := mock.LastRequest()
		if req.Language != "fr" {
			t.Errorf("Language = %q, want %q", req.Language, "fr")
		}
	})

	t.Run("passes prompt to API", func(t *testing.T) {
		t.Parallel()
		mock := &mockAudioTranscriber{
			responses: []openai.AudioResponse{{Text: "kubernetes discussion"}},
		}
		tr := transcribe.NewTestTranscriber(mock)

		prompt := "Technical discussion about Kubernetes"
		_, err := tr.Transcribe(context.Background(), "audio.mp3", transcribe.Options{
			Prompt: prompt,
		})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}

		req := mock.LastRequest()
		if req.Prompt != prompt {
			t.Errorf("Prompt = %q, want %q", req.Prompt, prompt)
		}
	})

	t.Run("uses correct model for standard transcription", func(t *testing.T) {
		t.Parallel()
		mock := &mockAudioTranscriber{
			responses: []openai.AudioResponse{{Text: "text"}},
		}
		tr := transcribe.NewTestTranscriber(mock)

		_, err := tr.Transcribe(context.Background(), "audio.mp3", transcribe.Options{
			Diarize: false,
		})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}

		req := mock.LastRequest()
		if req.Model != transcribe.ModelGPT4oMiniTranscribe {
			t.Errorf("Model = %q, want %q", req.Model, transcribe.ModelGPT4oMiniTranscribe)
		}
	})

	t.Run("uses diarize model when diarize is true", func(t *testing.T) {
		t.Parallel()

		// Diarization uses direct HTTP, so we need a mock HTTP client and real file
		audioPath := createTempAudioFile(t)
		httpMock := newMockHTTPClient(http.StatusOK, `{"text": "diarized text", "segments": []}`)

		tr := transcribe.NewTestTranscriberWithHTTP(
			&mockAudioTranscriber{}, // Not used for diarization
			httpMock,
			"test-api-key",
		)

		result, err := tr.Transcribe(context.Background(), audioPath, transcribe.Options{
			Diarize: true,
		})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}
		if result != "diarized text" {
			t.Errorf("got %q, want %q", result, "diarized text")
		}

		// Verify HTTP was called (not the go-openai client)
		if httpMock.CallCount() != 1 {
			t.Errorf("HTTP call count = %d, want 1", httpMock.CallCount())
		}
	})
}

// ---------------------------------------------------------------------------
// TestTranscribe_Diarization - Diarized output formatting via HTTP
// ---------------------------------------------------------------------------

func TestTranscribe_Diarization(t *testing.T) {
	t.Parallel()

	t.Run("formats segments with speaker labels", func(t *testing.T) {
		t.Parallel()
		audioPath := createTempAudioFile(t)

		// Response with speaker information
		responseJSON := `{
			"text": "Hello there General Kenobi",
			"segments": [
				{"id": "seg_001", "start": 0.0, "end": 1.5, "text": "Hello there", "speaker": "Speaker A"},
				{"id": "seg_002", "start": 1.5, "end": 3.0, "text": "General Kenobi", "speaker": "Speaker B"}
			]
		}`
		httpMock := newMockHTTPClient(http.StatusOK, responseJSON)

		tr := transcribe.NewTestTranscriberWithHTTP(
			&mockAudioTranscriber{},
			httpMock,
			"test-api-key",
		)

		result, err := tr.Transcribe(context.Background(), audioPath, transcribe.Options{
			Diarize: true,
		})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}

		// Verify speaker markers are present
		speakerPattern := regexp.MustCompile(`\[Speaker [AB]\]`)
		matches := speakerPattern.FindAllString(result, -1)
		if len(matches) != 2 {
			t.Errorf("expected 2 speaker markers, got %d in: %q", len(matches), result)
		}

		// Verify content is present
		if !regexp.MustCompile(`Hello there`).MatchString(result) {
			t.Errorf("result should contain 'Hello there': %q", result)
		}
		if !regexp.MustCompile(`General Kenobi`).MatchString(result) {
			t.Errorf("result should contain 'General Kenobi': %q", result)
		}
	})

	t.Run("falls back to text when no segments", func(t *testing.T) {
		t.Parallel()
		audioPath := createTempAudioFile(t)

		responseJSON := `{"text": "fallback text", "segments": []}`
		httpMock := newMockHTTPClient(http.StatusOK, responseJSON)

		tr := transcribe.NewTestTranscriberWithHTTP(
			&mockAudioTranscriber{},
			httpMock,
			"test-api-key",
		)

		result, err := tr.Transcribe(context.Background(), audioPath, transcribe.Options{
			Diarize: true,
		})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}

		if result != "fallback text" {
			t.Errorf("got %q, want %q", result, "fallback text")
		}
	})

	t.Run("uses speaker ID as fallback when speaker field empty", func(t *testing.T) {
		t.Parallel()
		audioPath := createTempAudioFile(t)

		// Response without speaker field (uses ID as fallback)
		responseJSON := `{
			"text": "Hello",
			"segments": [
				{"id": "seg_001", "start": 0.0, "end": 1.0, "text": "Hello", "speaker": ""}
			]
		}`
		httpMock := newMockHTTPClient(http.StatusOK, responseJSON)

		tr := transcribe.NewTestTranscriberWithHTTP(
			&mockAudioTranscriber{},
			httpMock,
			"test-api-key",
		)

		result, err := tr.Transcribe(context.Background(), audioPath, transcribe.Options{
			Diarize: true,
		})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}

		// Should use "Speaker <id>" format when speaker field is empty
		if !regexp.MustCompile(`\[Speaker seg_001\]`).MatchString(result) {
			t.Errorf("expected speaker ID fallback in: %q", result)
		}
	})

	t.Run("sends chunking_strategy auto parameter", func(t *testing.T) {
		t.Parallel()
		audioPath := createTempAudioFile(t)

		responseJSON := `{"text": "transcribed", "segments": []}`
		httpMock := newMockHTTPClient(http.StatusOK, responseJSON)

		tr := transcribe.NewTestTranscriberWithHTTP(
			&mockAudioTranscriber{},
			httpMock,
			"test-api-key",
		)

		_, err := tr.Transcribe(context.Background(), audioPath, transcribe.Options{
			Diarize: true,
		})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}

		// Verify chunking_strategy was sent
		if !httpMock.HasChunkingStrategy() {
			t.Error("chunking_strategy was not included in request")
		}
		if httpMock.ChunkingStrategyValue() != "auto" {
			t.Errorf("chunking_strategy = %q, want %q", httpMock.ChunkingStrategyValue(), "auto")
		}
	})

	t.Run("returns error for nonexistent audio file", func(t *testing.T) {
		t.Parallel()

		httpMock := newMockHTTPClient(http.StatusOK, `{"text": "ok", "segments": []}`)
		tr := transcribe.NewTestTranscriberWithHTTP(
			&mockAudioTranscriber{},
			httpMock,
			"test-api-key",
		)

		_, err := tr.Transcribe(context.Background(), "/nonexistent/path/audio.ogg", transcribe.Options{
			Diarize: true,
		})
		if err == nil {
			t.Fatal("expected error for nonexistent file, got nil")
		}
		// HTTP should NOT have been called
		if httpMock.CallCount() != 0 {
			t.Errorf("HTTP call count = %d, want 0", httpMock.CallCount())
		}
	})

	t.Run("passes language to diarization request", func(t *testing.T) {
		t.Parallel()
		audioPath := createTempAudioFile(t)

		httpMock := newMockHTTPClient(http.StatusOK, `{"text": "bonjour", "segments": []}`)
		tr := transcribe.NewTestTranscriberWithHTTP(
			&mockAudioTranscriber{},
			httpMock,
			"test-api-key",
		)

		_, err := tr.Transcribe(context.Background(), audioPath, transcribe.Options{
			Diarize:  true,
			Language: lang.MustParse("fr-FR"),
		})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}

		// Verify language was included in request body
		if len(httpMock.requestBodies) == 0 {
			t.Fatal("no request body captured")
		}
		body := string(httpMock.requestBodies[0])
		if !bytes.Contains([]byte(body), []byte("language")) {
			t.Error("language field not found in request")
		}
	})
}

// ---------------------------------------------------------------------------
// TestTranscribe_DiarizationErrors - HTTP error handling for diarization
// ---------------------------------------------------------------------------

func TestTranscribe_DiarizationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		statusCode   int
		responseBody string
		wantSentinel error
	}{
		{
			name:         "401 unauthorized returns ErrAuthFailed",
			statusCode:   http.StatusUnauthorized,
			responseBody: `{"error": {"message": "Invalid API key", "type": "invalid_request_error"}}`,
			wantSentinel: transcribe.ErrAuthFailed,
		},
		{
			name:         "429 rate limit returns ErrRateLimit",
			statusCode:   http.StatusTooManyRequests,
			responseBody: `{"error": {"message": "Rate limit exceeded", "type": "rate_limit_error"}}`,
			wantSentinel: transcribe.ErrRateLimit,
		},
		{
			name:         "429 with quota message returns ErrQuotaExceeded",
			statusCode:   http.StatusTooManyRequests,
			responseBody: `{"error": {"message": "You exceeded your quota", "type": "insufficient_quota"}}`,
			wantSentinel: transcribe.ErrQuotaExceeded,
		},
		{
			name:         "429 with billing message returns ErrQuotaExceeded",
			statusCode:   http.StatusTooManyRequests,
			responseBody: `{"error": {"message": "Please check your billing details", "type": "billing_error"}}`,
			wantSentinel: transcribe.ErrQuotaExceeded,
		},
		{
			name:         "400 bad request returns ErrBadRequest",
			statusCode:   http.StatusBadRequest,
			responseBody: `{"error": {"message": "Invalid file format", "type": "invalid_request_error"}}`,
			wantSentinel: transcribe.ErrBadRequest,
		},
		{
			name:         "408 timeout returns ErrTimeout",
			statusCode:   http.StatusRequestTimeout,
			responseBody: `{"error": {"message": "Request timeout", "type": "timeout"}}`,
			wantSentinel: transcribe.ErrTimeout,
		},
		{
			name:         "504 gateway timeout returns ErrTimeout",
			statusCode:   http.StatusGatewayTimeout,
			responseBody: `{"error": {"message": "Gateway timeout", "type": "timeout"}}`,
			wantSentinel: transcribe.ErrTimeout,
		},
		{
			name:         "500 server error returns ErrTimeout (retryable)",
			statusCode:   http.StatusInternalServerError,
			responseBody: `{"error": {"message": "Internal server error", "type": "server_error"}}`,
			wantSentinel: transcribe.ErrTimeout,
		},
		{
			name:         "502 bad gateway returns ErrTimeout (retryable)",
			statusCode:   http.StatusBadGateway,
			responseBody: `{"error": {"message": "Bad gateway", "type": "server_error"}}`,
			wantSentinel: transcribe.ErrTimeout,
		},
		{
			name:         "503 service unavailable returns ErrTimeout (retryable)",
			statusCode:   http.StatusServiceUnavailable,
			responseBody: `{"error": {"message": "Service unavailable", "type": "server_error"}}`,
			wantSentinel: transcribe.ErrTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			audioPath := createTempAudioFile(t)

			httpMock := newMockHTTPClient(tt.statusCode, tt.responseBody)
			tr := transcribe.NewTestTranscriberWithHTTP(
				&mockAudioTranscriber{},
				httpMock,
				"test-api-key",
				transcribe.WithMaxRetries(0), // Disable retries to get immediate error
			)

			_, err := tr.Transcribe(context.Background(), audioPath, transcribe.Options{
				Diarize: true,
			})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tt.wantSentinel) {
				t.Errorf("error = %v, want sentinel %v", err, tt.wantSentinel)
			}
		})
	}

	t.Run("malformed JSON error response returns generic error", func(t *testing.T) {
		t.Parallel()
		audioPath := createTempAudioFile(t)

		httpMock := newMockHTTPClient(http.StatusBadRequest, `not valid json`)
		tr := transcribe.NewTestTranscriberWithHTTP(
			&mockAudioTranscriber{},
			httpMock,
			"test-api-key",
			transcribe.WithMaxRetries(0),
		)

		_, err := tr.Transcribe(context.Background(), audioPath, transcribe.Options{
			Diarize: true,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		// Should contain HTTP status and raw body
		if !regexp.MustCompile(`HTTP 400`).MatchString(err.Error()) {
			t.Errorf("error should mention HTTP status: %v", err)
		}
	})

	t.Run("diarization retries on rate limit", func(t *testing.T) {
		t.Parallel()
		audioPath := createTempAudioFile(t)

		httpMock := &mockHTTPClient{
			statusCode:   http.StatusOK,
			responseBody: `{"text": "success", "segments": []}`,
			responses: []*http.Response{
				// First call: rate limit error
				{
					StatusCode: http.StatusTooManyRequests,
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"error": {"message": "Rate limit"}}`))),
					Header:     make(http.Header),
				},
				// Second call: success
				{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"text": "success after retry", "segments": []}`))),
					Header:     make(http.Header),
				},
			},
		}

		tr := transcribe.NewTestTranscriberWithHTTP(
			&mockAudioTranscriber{},
			httpMock,
			"test-api-key",
			transcribe.WithMaxRetries(3),
			transcribe.WithRetryDelays(1*time.Millisecond, 10*time.Millisecond),
		)

		result, err := tr.Transcribe(context.Background(), audioPath, transcribe.Options{
			Diarize: true,
		})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}
		if result != "success after retry" {
			t.Errorf("got %q, want %q", result, "success after retry")
		}
		if httpMock.CallCount() != 2 {
			t.Errorf("HTTP call count = %d, want 2", httpMock.CallCount())
		}
	})

	t.Run("diarization does not retry on auth failure", func(t *testing.T) {
		t.Parallel()
		audioPath := createTempAudioFile(t)

		httpMock := newMockHTTPClient(http.StatusUnauthorized, `{"error": {"message": "Invalid API key"}}`)
		tr := transcribe.NewTestTranscriberWithHTTP(
			&mockAudioTranscriber{},
			httpMock,
			"test-api-key",
			transcribe.WithMaxRetries(5),
			transcribe.WithRetryDelays(1*time.Millisecond, 10*time.Millisecond),
		)

		_, err := tr.Transcribe(context.Background(), audioPath, transcribe.Options{
			Diarize: true,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		// Should NOT have retried
		if httpMock.CallCount() != 1 {
			t.Errorf("HTTP call count = %d, want 1 (no retry)", httpMock.CallCount())
		}
	})

	t.Run("malformed diarize response returns parse error", func(t *testing.T) {
		t.Parallel()
		audioPath := createTempAudioFile(t)

		httpMock := newMockHTTPClient(http.StatusOK, `{"text": "ok", "segments": "not_an_array"}`)
		tr := transcribe.NewTestTranscriberWithHTTP(
			&mockAudioTranscriber{},
			httpMock,
			"test-api-key",
			transcribe.WithMaxRetries(0),
		)

		_, err := tr.Transcribe(context.Background(), audioPath, transcribe.Options{
			Diarize: true,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !regexp.MustCompile(`failed to parse`).MatchString(err.Error()) {
			t.Errorf("error should mention parse failure: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// TestTranscribe_ErrorClassification - Error wrapping and sentinel errors
// ---------------------------------------------------------------------------

func TestTranscribe_ErrorClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		apiError     *openai.APIError
		wantSentinel error
	}{
		{
			name: "401 unauthorized returns ErrAuthFailed",
			apiError: &openai.APIError{
				HTTPStatusCode: http.StatusUnauthorized,
				Message:        "Invalid API key",
			},
			wantSentinel: transcribe.ErrAuthFailed,
		},
		{
			name: "429 with quota message returns ErrQuotaExceeded",
			apiError: &openai.APIError{
				HTTPStatusCode: http.StatusTooManyRequests,
				Message:        "You have exceeded your quota",
			},
			wantSentinel: transcribe.ErrQuotaExceeded,
		},
		{
			name: "429 with billing message returns ErrQuotaExceeded",
			apiError: &openai.APIError{
				HTTPStatusCode: http.StatusTooManyRequests,
				Message:        "Please check your billing details",
			},
			wantSentinel: transcribe.ErrQuotaExceeded,
		},
		{
			name: "429 rate limit returns ErrRateLimit",
			apiError: &openai.APIError{
				HTTPStatusCode: http.StatusTooManyRequests,
				Message:        "Rate limit exceeded",
			},
			wantSentinel: transcribe.ErrRateLimit,
		},
		{
			name: "408 timeout returns ErrTimeout",
			apiError: &openai.APIError{
				HTTPStatusCode: http.StatusRequestTimeout,
				Message:        "Request timeout",
			},
			wantSentinel: transcribe.ErrTimeout,
		},
		{
			name: "504 gateway timeout returns ErrTimeout",
			apiError: &openai.APIError{
				HTTPStatusCode: http.StatusGatewayTimeout,
				Message:        "Gateway timeout",
			},
			wantSentinel: transcribe.ErrTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := &mockAudioTranscriber{
				errors: []error{tt.apiError},
			}
			// Disable retries to get immediate error
			tr := transcribe.NewTestTranscriber(mock, transcribe.WithMaxRetries(0))

			_, err := tr.Transcribe(context.Background(), "audio.mp3", transcribe.Options{})
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !errors.Is(err, tt.wantSentinel) {
				t.Errorf("error = %v, want sentinel %v", err, tt.wantSentinel)
			}
		})
	}

	t.Run("context deadline exceeded returns ErrTimeout", func(t *testing.T) {
		t.Parallel()

		mock := &mockAudioTranscriber{
			errors: []error{context.DeadlineExceeded},
		}
		tr := transcribe.NewTestTranscriber(mock, transcribe.WithMaxRetries(0))

		_, err := tr.Transcribe(context.Background(), "audio.mp3", transcribe.Options{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if !errors.Is(err, transcribe.ErrTimeout) {
			t.Errorf("error = %v, want sentinel %v", err, transcribe.ErrTimeout)
		}
	})
}

// ---------------------------------------------------------------------------
// TestTranscribe_Retry - Retry behavior with backoff
// ---------------------------------------------------------------------------

func TestTranscribe_Retry(t *testing.T) {
	t.Parallel()

	t.Run("retries on rate limit and succeeds", func(t *testing.T) {
		t.Parallel()

		rateLimitErr := &openai.APIError{
			HTTPStatusCode: http.StatusTooManyRequests,
			Message:        "Rate limit exceeded",
		}
		mock := &mockAudioTranscriber{
			errors:    []error{rateLimitErr, rateLimitErr, nil},
			responses: []openai.AudioResponse{{}, {}, {Text: "success"}},
		}
		tr := transcribe.NewTestTranscriber(mock,
			transcribe.WithMaxRetries(5),
			transcribe.WithRetryDelays(1*time.Millisecond, 10*time.Millisecond),
		)

		result, err := tr.Transcribe(context.Background(), "audio.mp3", transcribe.Options{})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}
		if result != "success" {
			t.Errorf("got %q, want %q", result, "success")
		}
		if mock.CallCount() != 3 {
			t.Errorf("call count = %d, want 3", mock.CallCount())
		}
	})

	t.Run("retries on server error 500", func(t *testing.T) {
		t.Parallel()

		serverErr := &openai.APIError{
			HTTPStatusCode: http.StatusInternalServerError,
			Message:        "Internal server error",
		}
		mock := &mockAudioTranscriber{
			errors:    []error{serverErr, nil},
			responses: []openai.AudioResponse{{}, {Text: "recovered"}},
		}
		tr := transcribe.NewTestTranscriber(mock,
			transcribe.WithMaxRetries(3),
			transcribe.WithRetryDelays(1*time.Millisecond, 10*time.Millisecond),
		)

		result, err := tr.Transcribe(context.Background(), "audio.mp3", transcribe.Options{})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}
		if result != "recovered" {
			t.Errorf("got %q, want %q", result, "recovered")
		}
	})

	t.Run("does not retry on auth failure", func(t *testing.T) {
		t.Parallel()

		authErr := &openai.APIError{
			HTTPStatusCode: http.StatusUnauthorized,
			Message:        "Invalid API key",
		}
		mock := &mockAudioTranscriber{
			errors: []error{authErr},
		}
		tr := transcribe.NewTestTranscriber(mock,
			transcribe.WithMaxRetries(5),
			transcribe.WithRetryDelays(1*time.Millisecond, 10*time.Millisecond),
		)

		_, err := tr.Transcribe(context.Background(), "audio.mp3", transcribe.Options{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if mock.CallCount() != 1 {
			t.Errorf("call count = %d, want 1 (no retry)", mock.CallCount())
		}
	})

	t.Run("does not retry on quota exceeded", func(t *testing.T) {
		t.Parallel()

		quotaErr := &openai.APIError{
			HTTPStatusCode: http.StatusTooManyRequests,
			Message:        "You exceeded your quota",
		}
		mock := &mockAudioTranscriber{
			errors: []error{quotaErr},
		}
		tr := transcribe.NewTestTranscriber(mock,
			transcribe.WithMaxRetries(5),
			transcribe.WithRetryDelays(1*time.Millisecond, 10*time.Millisecond),
		)

		_, err := tr.Transcribe(context.Background(), "audio.mp3", transcribe.Options{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if mock.CallCount() != 1 {
			t.Errorf("call count = %d, want 1 (no retry)", mock.CallCount())
		}
	})

	t.Run("max retries exceeded wraps error", func(t *testing.T) {
		t.Parallel()

		rateLimitErr := &openai.APIError{
			HTTPStatusCode: http.StatusTooManyRequests,
			Message:        "Rate limit exceeded",
		}
		mock := &mockAudioTranscriber{
			errors: []error{rateLimitErr, rateLimitErr, rateLimitErr},
		}
		tr := transcribe.NewTestTranscriber(mock,
			transcribe.WithMaxRetries(2), // 3 attempts total
			transcribe.WithRetryDelays(1*time.Millisecond, 10*time.Millisecond),
		)

		_, err := tr.Transcribe(context.Background(), "audio.mp3", transcribe.Options{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if mock.CallCount() != 3 {
			t.Errorf("call count = %d, want 3", mock.CallCount())
		}

		// Should mention max retries in error message
		if !regexp.MustCompile(`max retries.*exceeded`).MatchString(err.Error()) {
			t.Errorf("error should mention max retries: %v", err)
		}
	})

	t.Run("context cancellation stops retries", func(t *testing.T) {
		t.Parallel()

		rateLimitErr := &openai.APIError{
			HTTPStatusCode: http.StatusTooManyRequests,
			Message:        "Rate limit exceeded",
		}
		mock := &mockAudioTranscriber{
			errors: []error{rateLimitErr, rateLimitErr, rateLimitErr, rateLimitErr, rateLimitErr},
		}
		tr := transcribe.NewTestTranscriber(mock,
			transcribe.WithMaxRetries(10),
			transcribe.WithRetryDelays(50*time.Millisecond, 100*time.Millisecond),
		)

		ctx, cancel := context.WithCancel(context.Background())
		// Cancel after a short delay
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		_, err := tr.Transcribe(ctx, "audio.mp3", transcribe.Options{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want context.Canceled", err)
		}
		// Should have stopped before all retries
		if mock.CallCount() >= 5 {
			t.Errorf("call count = %d, should be less than 5 (cancelled early)", mock.CallCount())
		}
	})
}

// ---------------------------------------------------------------------------
// TestTranscribe_Options - Option functions
// ---------------------------------------------------------------------------

func TestTranscribe_Options(t *testing.T) {
	t.Parallel()

	t.Run("WithMaxRetries(0) disables retries", func(t *testing.T) {
		t.Parallel()

		rateLimitErr := &openai.APIError{
			HTTPStatusCode: http.StatusTooManyRequests,
			Message:        "Rate limit exceeded",
		}
		mock := &mockAudioTranscriber{
			errors: []error{rateLimitErr, rateLimitErr},
		}
		tr := transcribe.NewTestTranscriber(mock, transcribe.WithMaxRetries(0))

		_, err := tr.Transcribe(context.Background(), "audio.mp3", transcribe.Options{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if mock.CallCount() != 1 {
			t.Errorf("call count = %d, want 1 (no retries)", mock.CallCount())
		}
	})

	t.Run("WithMaxRetries negative is ignored", func(t *testing.T) {
		t.Parallel()

		rateLimitErr := &openai.APIError{
			HTTPStatusCode: http.StatusTooManyRequests,
			Message:        "Rate limit exceeded",
		}
		mock := &mockAudioTranscriber{
			errors:    []error{rateLimitErr, nil},
			responses: []openai.AudioResponse{{}, {Text: "ok"}},
		}
		// Negative should be ignored, keeping default (5)
		tr := transcribe.NewTestTranscriber(mock,
			transcribe.WithMaxRetries(-1),
			transcribe.WithRetryDelays(1*time.Millisecond, 10*time.Millisecond),
		)

		result, err := tr.Transcribe(context.Background(), "audio.mp3", transcribe.Options{})
		if err != nil {
			t.Errorf("Transcribe() unexpected error: %v", err)
		}
		if result != "ok" {
			t.Errorf("got %q, want %q", result, "ok")
		}
		// Should have retried (default retries applied)
		if mock.CallCount() != 2 {
			t.Errorf("call count = %d, want 2", mock.CallCount())
		}
	})
}

// ---------------------------------------------------------------------------
// TestRetryWithBackoff - Generic retry utility edge cases
// ---------------------------------------------------------------------------

func TestRetryWithBackoff(t *testing.T) {
	t.Parallel()

	t.Run("success on first try returns immediately", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		result, err := transcribe.RetryWithBackoff(
			context.Background(),
			transcribe.RetryConfig{MaxRetries: 5, BaseDelay: time.Second, MaxDelay: time.Minute},
			func() (string, error) {
				callCount++
				return "immediate", nil
			},
			func(error) bool { return true },
		)

		if err != nil {
			t.Errorf("RetryWithBackoff() unexpected error: %v", err)
		}
		if result != "immediate" {
			t.Errorf("got %q, want %q", result, "immediate")
		}
		if callCount != 1 {
			t.Errorf("call count = %d, want 1", callCount)
		}
	})

	t.Run("shouldRetry false stops immediately", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		testErr := errors.New("non-retryable")
		_, err := transcribe.RetryWithBackoff(
			context.Background(),
			transcribe.RetryConfig{MaxRetries: 5, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
			func() (string, error) {
				callCount++
				return "", testErr
			},
			func(error) bool { return false }, // Never retry
		)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if callCount != 1 {
			t.Errorf("call count = %d, want 1 (no retry)", callCount)
		}
	})

	t.Run("MaxRetries 0 means single attempt", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		testErr := errors.New("always fails")
		_, err := transcribe.RetryWithBackoff(
			context.Background(),
			transcribe.RetryConfig{MaxRetries: 0, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
			func() (string, error) {
				callCount++
				return "", testErr
			},
			func(error) bool { return true },
		)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if callCount != 1 {
			t.Errorf("call count = %d, want 1", callCount)
		}
	})

	t.Run("already cancelled context returns immediately", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel before calling

		callCount := 0
		_, err := transcribe.RetryWithBackoff(
			ctx,
			transcribe.RetryConfig{MaxRetries: 5, BaseDelay: time.Second, MaxDelay: time.Minute},
			func() (string, error) {
				callCount++
				return "", errors.New("should retry")
			},
			func(error) bool { return true },
		)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want context.Canceled", err)
		}
		// First call happens, then context check on retry wait
		if callCount != 1 {
			t.Errorf("call count = %d, want 1", callCount)
		}
	})

	t.Run("negative MaxRetries normalized to 0", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		testErr := errors.New("always fails")
		_, err := transcribe.RetryWithBackoff(
			context.Background(),
			transcribe.RetryConfig{MaxRetries: -5, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
			func() (string, error) {
				callCount++
				return "", testErr
			},
			func(error) bool { return true },
		)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		// Negative MaxRetries becomes 0, so single attempt
		if callCount != 1 {
			t.Errorf("call count = %d, want 1", callCount)
		}
	})

	t.Run("zero BaseDelay normalized to 1ms", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		_, err := transcribe.RetryWithBackoff(
			context.Background(),
			transcribe.RetryConfig{MaxRetries: 1, BaseDelay: 0, MaxDelay: time.Millisecond},
			func() (string, error) {
				callCount++
				if callCount < 2 {
					return "", errors.New("retry")
				}
				return "ok", nil
			},
			func(error) bool { return true },
		)

		if err != nil {
			t.Errorf("RetryWithBackoff() unexpected error: %v", err)
		}
		if callCount != 2 {
			t.Errorf("call count = %d, want 2", callCount)
		}
	})

	t.Run("zero MaxDelay normalized to BaseDelay", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		_, err := transcribe.RetryWithBackoff(
			context.Background(),
			transcribe.RetryConfig{MaxRetries: 1, BaseDelay: time.Millisecond, MaxDelay: 0},
			func() (string, error) {
				callCount++
				if callCount < 2 {
					return "", errors.New("retry")
				}
				return "ok", nil
			},
			func(error) bool { return true },
		)

		if err != nil {
			t.Errorf("RetryWithBackoff() unexpected error: %v", err)
		}
		if callCount != 2 {
			t.Errorf("call count = %d, want 2", callCount)
		}
	})
}

// ---------------------------------------------------------------------------
// TestTranscribeAll - Parallel batch transcription
// ---------------------------------------------------------------------------

func TestTranscribeAll(t *testing.T) {
	t.Parallel()

	t.Run("empty chunks returns nil", func(t *testing.T) {
		t.Parallel()

		results, err := transcribe.TranscribeAll(
			context.Background(),
			nil,
			newMockTranscriber(),
			transcribe.Options{},
			4,
		)

		if err != nil {
			t.Errorf("TranscribeAll() unexpected error: %v", err)
		}
		if results != nil {
			t.Errorf("got %v, want nil", results)
		}
	})

	t.Run("single chunk returns single result", func(t *testing.T) {
		t.Parallel()

		mock := newMockTranscriber()
		mock.results["/path/chunk0.mp3"] = "hello world"

		chunks := []audio.Chunk{
			{Path: "/path/chunk0.mp3", Index: 0},
		}

		results, err := transcribe.TranscribeAll(
			context.Background(),
			chunks,
			mock,
			transcribe.Options{},
			4,
		)

		if err != nil {
			t.Errorf("TranscribeAll() unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("got %d results, want 1", len(results))
		}
		if results[0] != "hello world" {
			t.Errorf("results[0] = %q, want %q", results[0], "hello world")
		}
	})

	t.Run("multiple chunks return results in order", func(t *testing.T) {
		t.Parallel()

		mock := newMockTranscriber()
		mock.results["/path/chunk0.mp3"] = "first"
		mock.results["/path/chunk1.mp3"] = "second"
		mock.results["/path/chunk2.mp3"] = "third"

		chunks := []audio.Chunk{
			{Path: "/path/chunk0.mp3", Index: 0},
			{Path: "/path/chunk1.mp3", Index: 1},
			{Path: "/path/chunk2.mp3", Index: 2},
		}

		results, err := transcribe.TranscribeAll(
			context.Background(),
			chunks,
			mock,
			transcribe.Options{},
			4,
		)

		if err != nil {
			t.Errorf("TranscribeAll() unexpected error: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("got %d results, want 3", len(results))
		}
		// Order must match chunks, not completion order
		if results[0] != "first" || results[1] != "second" || results[2] != "third" {
			t.Errorf("results = %v, want [first, second, third]", results)
		}
	})

	t.Run("first error aborts and returns error", func(t *testing.T) {
		t.Parallel()

		mock := newMockTranscriber()
		mock.results["/path/chunk0.mp3"] = "ok"
		mock.errors["/path/chunk1.mp3"] = errors.New("transcription failed")
		mock.results["/path/chunk2.mp3"] = "ok"

		chunks := []audio.Chunk{
			{Path: "/path/chunk0.mp3", Index: 0},
			{Path: "/path/chunk1.mp3", Index: 1},
			{Path: "/path/chunk2.mp3", Index: 2},
		}

		_, err := transcribe.TranscribeAll(
			context.Background(),
			chunks,
			mock,
			transcribe.Options{},
			4,
		)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !regexp.MustCompile(`chunk 1`).MatchString(err.Error()) {
			t.Errorf("error should mention chunk index: %v", err)
		}
	})

	t.Run("context cancellation propagates", func(t *testing.T) {
		t.Parallel()

		mock := newMockTranscriber()
		mock.blocking = make(chan struct{})
		mock.started = make(chan struct{}, 10)

		chunks := []audio.Chunk{
			{Path: "/path/chunk0.mp3", Index: 0},
			{Path: "/path/chunk1.mp3", Index: 1},
		}

		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan error)
		go func() {
			_, err := transcribe.TranscribeAll(ctx, chunks, mock, transcribe.Options{}, 4)
			done <- err
		}()

		// Wait for at least one to start
		<-mock.started
		cancel()

		err := <-done
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want context.Canceled", err)
		}
	})

	t.Run("maxParallel 1 serializes requests", func(t *testing.T) {
		t.Parallel()

		mock := newMockTranscriber()
		mock.results["/path/chunk0.mp3"] = "a"
		mock.results["/path/chunk1.mp3"] = "b"
		mock.results["/path/chunk2.mp3"] = "c"

		chunks := []audio.Chunk{
			{Path: "/path/chunk0.mp3", Index: 0},
			{Path: "/path/chunk1.mp3", Index: 1},
			{Path: "/path/chunk2.mp3", Index: 2},
		}

		results, err := transcribe.TranscribeAll(
			context.Background(),
			chunks,
			mock,
			transcribe.Options{},
			1, // Serial execution
		)

		if err != nil {
			t.Errorf("TranscribeAll() unexpected error: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("got %d results, want 3", len(results))
		}

		// With maxParallel=1, max concurrent should be 1
		if atomic.LoadInt32(&mock.maxConc) > 1 {
			t.Errorf("maxConcurrent = %d, want <= 1", mock.maxConc)
		}
	})

	t.Run("maxParallel 0 treated as 1", func(t *testing.T) {
		t.Parallel()

		mock := newMockTranscriber()
		mock.results["/path/chunk0.mp3"] = "ok"

		chunks := []audio.Chunk{
			{Path: "/path/chunk0.mp3", Index: 0},
		}

		results, err := transcribe.TranscribeAll(
			context.Background(),
			chunks,
			mock,
			transcribe.Options{},
			0, // Invalid, should be treated as 1
		)

		if err != nil {
			t.Errorf("TranscribeAll() unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("got %d results, want 1", len(results))
		}
	})

	t.Run("negative maxParallel treated as 1", func(t *testing.T) {
		t.Parallel()

		mock := newMockTranscriber()
		mock.results["/path/chunk0.mp3"] = "ok"

		chunks := []audio.Chunk{
			{Path: "/path/chunk0.mp3", Index: 0},
		}

		results, err := transcribe.TranscribeAll(
			context.Background(),
			chunks,
			mock,
			transcribe.Options{},
			-5, // Invalid, should be treated as 1
		)

		if err != nil {
			t.Errorf("TranscribeAll() unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("got %d results, want 1", len(results))
		}
	})
}

// ---------------------------------------------------------------------------
// TestClassifyError - Exported internal function
// ---------------------------------------------------------------------------

func TestClassifyError(t *testing.T) {
	t.Parallel()

	t.Run("non-API error passes through", func(t *testing.T) {
		t.Parallel()

		originalErr := errors.New("network error")
		result := transcribe.ClassifyError(originalErr)

		if result != originalErr {
			t.Errorf("error should pass through unchanged: got %v, want %v", result, originalErr)
		}
	})

	t.Run("unknown status code passes through with wrapping", func(t *testing.T) {
		t.Parallel()

		apiErr := &openai.APIError{
			HTTPStatusCode: http.StatusTeapot, // 418
			Message:        "I'm a teapot",
		}
		result := transcribe.ClassifyError(apiErr)

		// Should return the original error since it's not a recognized status
		if result != apiErr {
			t.Errorf("unknown status should pass through: got %v", result)
		}
	})

	t.Run("400 Bad Request returns ErrBadRequest", func(t *testing.T) {
		t.Parallel()

		apiErr := &openai.APIError{
			HTTPStatusCode: http.StatusBadRequest,
			Message:        "Invalid request",
		}
		result := transcribe.ClassifyError(apiErr)

		if !errors.Is(result, transcribe.ErrBadRequest) {
			t.Errorf("expected ErrBadRequest, got %v", result)
		}
	})

	t.Run("403 Forbidden returns ErrBadRequest", func(t *testing.T) {
		t.Parallel()

		apiErr := &openai.APIError{
			HTTPStatusCode: http.StatusForbidden,
			Message:        "Access denied",
		}
		result := transcribe.ClassifyError(apiErr)

		if !errors.Is(result, transcribe.ErrBadRequest) {
			t.Errorf("expected ErrBadRequest, got %v", result)
		}
	})

	t.Run("404 Not Found returns ErrBadRequest", func(t *testing.T) {
		t.Parallel()

		apiErr := &openai.APIError{
			HTTPStatusCode: http.StatusNotFound,
			Message:        "Model not found",
		}
		result := transcribe.ClassifyError(apiErr)

		if !errors.Is(result, transcribe.ErrBadRequest) {
			t.Errorf("expected ErrBadRequest, got %v", result)
		}
	})
}

// ---------------------------------------------------------------------------
// TestIsRetryableError - Exported internal function
// ---------------------------------------------------------------------------

func TestIsRetryableError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "ErrRateLimit is retryable",
			err:  transcribe.ErrRateLimit,
			want: true,
		},
		{
			name: "ErrTimeout is retryable",
			err:  transcribe.ErrTimeout,
			want: true,
		},
		{
			name: "wrapped ErrRateLimit is retryable",
			err:  errors.Join(errors.New("context"), transcribe.ErrRateLimit),
			want: true,
		},
		{
			name: "ErrAuthFailed is not retryable",
			err:  transcribe.ErrAuthFailed,
			want: false,
		},
		{
			name: "ErrQuotaExceeded is not retryable",
			err:  transcribe.ErrQuotaExceeded,
			want: false,
		},
		{
			name: "context.Canceled is not retryable",
			err:  context.Canceled,
			want: false,
		},
		{
			name: "500 Internal Server Error is retryable",
			err:  &openai.APIError{HTTPStatusCode: http.StatusInternalServerError},
			want: true,
		},
		{
			name: "502 Bad Gateway is retryable",
			err:  &openai.APIError{HTTPStatusCode: http.StatusBadGateway},
			want: true,
		},
		{
			name: "503 Service Unavailable is retryable",
			err:  &openai.APIError{HTTPStatusCode: http.StatusServiceUnavailable},
			want: true,
		},
		{
			name: "504 Gateway Timeout is retryable",
			err:  &openai.APIError{HTTPStatusCode: http.StatusGatewayTimeout},
			want: true,
		},
		{
			name: "400 Bad Request is not retryable",
			err:  &openai.APIError{HTTPStatusCode: http.StatusBadRequest},
			want: false,
		},
		{
			name: "random error is not retryable",
			err:  errors.New("random error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := transcribe.IsRetryableError(tt.err)
			if got != tt.want {
				t.Errorf("IsRetryableError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
