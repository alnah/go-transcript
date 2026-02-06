package apierr

import (
	"context"
	"fmt"
	"time"
)

// RetryConfig holds retry parameters for exponential backoff.
//
// All fields must be non-negative. Invalid values are normalized:
//   - MaxRetries < 0 becomes 0 (single attempt)
//   - BaseDelay <= 0 becomes 1ms
//   - MaxDelay <= 0 becomes BaseDelay
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// normalize ensures all RetryConfig fields have valid values.
func (c *RetryConfig) normalize() {
	if c.MaxRetries < 0 {
		c.MaxRetries = 0
	}
	if c.BaseDelay <= 0 {
		c.BaseDelay = time.Millisecond
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = c.BaseDelay
	}
}

// RetryWithBackoff executes fn with exponential backoff retry.
// It retries only if shouldRetry returns true for the error.
// Returns the result of the last attempt.
//
// Invalid RetryConfig values are normalized (see RetryConfig documentation).
func RetryWithBackoff[T any](
	ctx context.Context,
	cfg RetryConfig,
	fn func() (T, error),
	shouldRetry func(error) bool,
) (T, error) {
	cfg.normalize()

	var zero T
	var lastErr error
	delay := cfg.BaseDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return zero, ctx.Err()
			case <-timer.C:
			}
			// Exponential backoff with cap.
			delay = min(delay*2, cfg.MaxDelay)
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err
		if !shouldRetry(lastErr) {
			return zero, lastErr
		}
	}

	return zero, fmt.Errorf("max retries (%d) exceeded: %w", cfg.MaxRetries, lastErr)
}
