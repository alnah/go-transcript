// Package apierr provides shared error sentinels and retry infrastructure
// for HTTP-based API clients. All provider-specific error types are
// classified into these sentinels at the adapter boundary.
//
// Providers map HTTP status codes to these errors using fmt.Errorf("%s: %w", msg, sentinel).
// Callers check with errors.Is(err, apierr.ErrRateLimit) etc.
package apierr

import "errors"

// Sentinel errors for API interaction failures.
var (
	// ErrRateLimit indicates the API rate limit was exceeded (temporary, retryable).
	ErrRateLimit = errors.New("rate limit exceeded")

	// ErrQuotaExceeded indicates the API quota was exceeded (billing issue, not retryable).
	ErrQuotaExceeded = errors.New("quota exceeded")

	// ErrTimeout indicates a request timed out.
	ErrTimeout = errors.New("request timeout")

	// ErrAuthFailed indicates API authentication failed (invalid key).
	ErrAuthFailed = errors.New("authentication failed")

	// ErrBadRequest indicates a client error (4xx) that is not otherwise classified.
	ErrBadRequest = errors.New("bad request")
)
