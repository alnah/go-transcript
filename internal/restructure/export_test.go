package restructure

// Exports for testing. These allow black-box tests to inject dependencies
// without modifying the public API.

// OpenAI option exports for dependency injection in tests.
var (
	WithChatCompleter    = withChatCompleter
	WithTemplateResolver = withTemplateResolver
)

// DeepSeek option exports for dependency injection in tests.
var (
	WithDeepSeekHTTPClient       = withDeepSeekHTTPClient
	WithDeepSeekTemplateResolver = withDeepSeekTemplateResolver
)

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
