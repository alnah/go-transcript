package transcribe

import (
	"time"
)

// Exports for testing. These allow black-box tests to inject dependencies
// without modifying the public API.

// NewTestTranscriber creates an OpenAITranscriber with a mock httpDoer and test base URL.
// This allows testing without a real OpenAI API.
func NewTestTranscriber(httpClient httpDoer, baseURL string, opts ...TranscriberOption) *OpenAITranscriber {
	t := &OpenAITranscriber{
		httpClient: httpClient,
		apiKey:     "test-api-key",
		baseURL:    baseURL,
		maxRetries: defaultMaxRetries,
		baseDelay:  defaultBaseDelay,
		maxDelay:   defaultMaxDelay,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Option exports for dependency injection in tests.
var TestWithHTTPClient = WithHTTPClient

// MinimalRetryOpts returns options that minimize retry delays for tests.
func MinimalRetryOpts() []TranscriberOption {
	return []TranscriberOption{
		WithMaxRetries(0),
		WithRetryDelays(time.Millisecond, time.Millisecond),
	}
}

// Function exports for unit testing internal logic.
var (
	ClassifyError              = classifyError
	IsRetryableError           = isRetryableError
	ParseDiarizeResponse       = parseDiarizeResponse
	ParseTranscriptionResponse = parseTranscriptionResponse
	ParseHTTPError             = parseHTTPError
)
