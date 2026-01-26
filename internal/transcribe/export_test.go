package transcribe

// Exports for testing. These allow black-box tests to inject dependencies
// without modifying the public API.

// NewTestTranscriber creates an OpenAITranscriber with a mock audioTranscriber.
// This allows testing without a real OpenAI client.
func NewTestTranscriber(client audioTranscriber, opts ...TranscriberOption) *OpenAITranscriber {
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

// Function exports for unit testing internal logic.
var (
	ClassifyError    = classifyError
	IsRetryableError = isRetryableError
)
