package transcribe

import (
	"net/http"
	"time"
)

// Exports for testing. These allow black-box tests to inject dependencies
// without modifying the public API.

// NewTestTranscriber creates an OpenAITranscriber with a mock audioTranscriber.
// This allows testing without a real OpenAI client.
func NewTestTranscriber(client audioTranscriber, opts ...TranscriberOption) *OpenAITranscriber {
	t := &OpenAITranscriber{
		client:     client,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		apiKey:     "test-api-key",
		maxRetries: defaultMaxRetries,
		baseDelay:  defaultBaseDelay,
		maxDelay:   defaultMaxDelay,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// NewTestTranscriberWithHTTP creates an OpenAITranscriber with both mock audioTranscriber and httpDoer.
// This allows testing diarization which uses direct HTTP.
func NewTestTranscriberWithHTTP(client audioTranscriber, httpClient httpDoer, apiKey string, opts ...TranscriberOption) *OpenAITranscriber {
	t := &OpenAITranscriber{
		client:     client,
		httpClient: httpClient,
		apiKey:     apiKey,
		maxRetries: defaultMaxRetries,
		baseDelay:  defaultBaseDelay,
		maxDelay:   defaultMaxDelay,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Function exports for unit testing internal logic.
var (
	ClassifyError    = classifyError
	IsRetryableError = isRetryableError
)
