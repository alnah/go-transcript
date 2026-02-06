package restructure

// Exports for testing. These allow black-box tests to inject dependencies
// without modifying the public API.

// DeepSeek option exports for dependency injection in tests.
var WithDeepSeekHTTPClient = withDeepSeekHTTPClient

// Function exports for unit testing internal logic.
var (
	// OpenAI error handling
	ClassifyRestructureError    = classifyRestructureError
	IsRetryableRestructureError = isRetryableRestructureError

	// DeepSeek error handling
	ClassifyDeepSeekError    = classifyDeepSeekError
	IsRetryableDeepSeekError = isRetryableDeepSeekError

	// Shared functions
	SplitTranscript = splitTranscript
	BuildMapPrompt  = buildMapPrompt
	EstimateTokens  = estimateTokens
)
