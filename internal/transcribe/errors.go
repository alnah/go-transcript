package transcribe

import "errors"

// ErrAPIKeyMissing indicates OPENAI_API_KEY environment variable is not set.
var ErrAPIKeyMissing = errors.New("OPENAI_API_KEY environment variable not set")

// ErrRateLimit indicates OpenAI API rate limit was exceeded (temporary, retryable).
var ErrRateLimit = errors.New("rate limit exceeded")

// ErrQuotaExceeded indicates OpenAI API quota was exceeded (billing issue, not retryable).
var ErrQuotaExceeded = errors.New("quota exceeded")

// ErrTimeout indicates a request timed out.
var ErrTimeout = errors.New("request timeout")

// ErrAuthFailed indicates OpenAI API authentication failed (invalid key).
var ErrAuthFailed = errors.New("authentication failed")
