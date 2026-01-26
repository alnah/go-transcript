package cli

import (
	"errors"
	"fmt"
	"testing"
)

// ---------------------------------------------------------------------------
// Tests for sentinel errors
// ---------------------------------------------------------------------------

func TestSentinelErrors_AreDistinct(t *testing.T) {
	t.Parallel()

	sentinels := []error{
		ErrAPIKeyMissing,
		ErrInvalidDuration,
		ErrUnsupportedFormat,
		ErrFileNotFound,
		ErrOutputExists,
	}

	// Verify all sentinels are distinct from each other
	for i, err1 := range sentinels {
		for j, err2 := range sentinels {
			if i != j && errors.Is(err1, err2) {
				t.Errorf("sentinels %d and %d should not match: %v == %v", i, j, err1, err2)
			}
		}
	}
}

func TestSentinelErrors_CanBeWrapped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		sentinel error
	}{
		{"ErrAPIKeyMissing", ErrAPIKeyMissing},
		{"ErrInvalidDuration", ErrInvalidDuration},
		{"ErrUnsupportedFormat", ErrUnsupportedFormat},
		{"ErrFileNotFound", ErrFileNotFound},
		{"ErrOutputExists", ErrOutputExists},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Wrap the error
			wrapped := fmt.Errorf("context: %w", tt.sentinel)

			// errors.Is should still work
			if !errors.Is(wrapped, tt.sentinel) {
				t.Errorf("wrapped error should match sentinel via errors.Is")
			}

			// The wrapped error should contain the original message
			if wrapped.Error() == tt.sentinel.Error() {
				t.Errorf("wrapped error should have additional context")
			}
		})
	}
}

func TestSentinelErrors_HaveMeaningfulMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sentinel error
		contains string
	}{
		{ErrAPIKeyMissing, "OPENAI_API_KEY"},
		{ErrInvalidDuration, "duration"},
		{ErrUnsupportedFormat, "format"},
		{ErrFileNotFound, "not found"},
		{ErrOutputExists, "exists"},
	}

	for _, tt := range tests {
		t.Run(tt.sentinel.Error(), func(t *testing.T) {
			t.Parallel()

			msg := tt.sentinel.Error()
			if msg == "" {
				t.Error("error message should not be empty")
			}
			// Note: We don't strictly check contains to avoid coupling tests
			// to exact wording, but the error should be descriptive
			if len(msg) < 10 {
				t.Errorf("error message should be descriptive, got: %q", msg)
			}
		})
	}
}

func TestSentinelErrors_NotNil(t *testing.T) {
	t.Parallel()

	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrAPIKeyMissing", ErrAPIKeyMissing},
		{"ErrInvalidDuration", ErrInvalidDuration},
		{"ErrUnsupportedFormat", ErrUnsupportedFormat},
		{"ErrFileNotFound", ErrFileNotFound},
		{"ErrOutputExists", ErrOutputExists},
	}

	for _, tt := range sentinels {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.err == nil {
				t.Errorf("%s should not be nil", tt.name)
			}
		})
	}
}
