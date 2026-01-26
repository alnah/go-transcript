package transcribe

import "errors"

// ErrRateLimit indicates OpenAI API rate limit was exceeded (temporary, retryable).
var ErrRateLimit = errors.New("rate limit exceeded")

// ErrQuotaExceeded indicates OpenAI API quota was exceeded (billing issue, not retryable).
var ErrQuotaExceeded = errors.New("quota exceeded")

// ErrTimeout indicates a request timed out.
var ErrTimeout = errors.New("request timeout")

// ErrAuthFailed indicates OpenAI API authentication failed (invalid key).
var ErrAuthFailed = errors.New("authentication failed")

// ErrBadRequest indicates a client error (4xx) that is not otherwise classified.
var ErrBadRequest = errors.New("bad request")
