package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
)

// allSentinelErrors lists all sentinel errors defined in errors.go with their expected exit codes.
// This ensures exhaustive coverage and serves as documentation.
var allSentinelErrors = []struct {
	err      error
	name     string
	exitCode int
}{
	// Setup errors (ExitSetup = 3)
	{ErrFFmpegNotFound, "ErrFFmpegNotFound", ExitSetup},
	{ErrAPIKeyMissing, "ErrAPIKeyMissing", ExitSetup},
	{ErrUnsupportedPlatform, "ErrUnsupportedPlatform", ExitSetup},
	{ErrChecksumMismatch, "ErrChecksumMismatch", ExitSetup},
	{ErrDownloadFailed, "ErrDownloadFailed", ExitSetup},
	{ErrNoAudioDevice, "ErrNoAudioDevice", ExitSetup},
	{ErrLoopbackNotFound, "ErrLoopbackNotFound", ExitSetup},

	// Validation errors (ExitValidation = 4)
	{ErrInvalidDuration, "ErrInvalidDuration", ExitValidation},
	{ErrUnsupportedFormat, "ErrUnsupportedFormat", ExitValidation},
	{ErrFileNotFound, "ErrFileNotFound", ExitValidation},
	{ErrUnknownTemplate, "ErrUnknownTemplate", ExitValidation},
	{ErrOutputExists, "ErrOutputExists", ExitValidation},
	{ErrChunkingFailed, "ErrChunkingFailed", ExitValidation},
	{ErrChunkTooLarge, "ErrChunkTooLarge", ExitValidation},
	{ErrInvalidLanguage, "ErrInvalidLanguage", ExitValidation},

	// Transcription errors (ExitTranscription = 5)
	{ErrRateLimit, "ErrRateLimit", ExitTranscription},
	{ErrQuotaExceeded, "ErrQuotaExceeded", ExitTranscription},
	{ErrTimeout, "ErrTimeout", ExitTranscription},
	{ErrAuthFailed, "ErrAuthFailed", ExitTranscription},

	// Restructure errors (ExitRestructure = 6)
	{ErrTranscriptTooLong, "ErrTranscriptTooLong", ExitRestructure},
}

// TestSentinelErrors_WrappedWithFmtErrorf verifies that errors.Is() works after
// wrapping sentinel errors with fmt.Errorf and %w, which is the documented usage pattern.
func TestSentinelErrors_WrappedWithFmtErrorf(t *testing.T) {
	for _, tc := range allSentinelErrors {
		t.Run(tc.name, func(t *testing.T) {
			// Single level wrap (most common)
			wrapped := fmt.Errorf("context info: %w", tc.err)
			if !errors.Is(wrapped, tc.err) {
				t.Errorf("errors.Is(wrapped, %s) = false, want true", tc.name)
			}

			// Multi-level wrap (realistic: chunker -> cmd -> main)
			level1 := fmt.Errorf("level1: %w", tc.err)
			level2 := fmt.Errorf("level2: %w", level1)
			level3 := fmt.Errorf("level3: %w", level2)
			if !errors.Is(level3, tc.err) {
				t.Errorf("errors.Is(deep wrapped, %s) = false, want true", tc.name)
			}
		})
	}
}

// TestExitCode_MapsAllErrors verifies that exitCode() correctly maps all sentinel errors
// to their spec-defined exit codes, including wrapped errors.
func TestExitCode_MapsAllErrors(t *testing.T) {
	// Test all sentinel errors (direct and wrapped)
	for _, tc := range allSentinelErrors {
		t.Run(tc.name+"_direct", func(t *testing.T) {
			got := exitCode(tc.err)
			if got != tc.exitCode {
				t.Errorf("exitCode(%s) = %d, want %d", tc.name, got, tc.exitCode)
			}
		})

		t.Run(tc.name+"_wrapped", func(t *testing.T) {
			wrapped := fmt.Errorf("wrapped: %w", tc.err)
			got := exitCode(wrapped)
			if got != tc.exitCode {
				t.Errorf("exitCode(wrapped %s) = %d, want %d", tc.name, got, tc.exitCode)
			}
		})
	}

	// Edge cases
	t.Run("nil_error", func(t *testing.T) {
		got := exitCode(nil)
		if got != ExitOK {
			t.Errorf("exitCode(nil) = %d, want %d (ExitOK)", got, ExitOK)
		}
	})

	t.Run("unknown_error", func(t *testing.T) {
		unknown := errors.New("some unexpected error")
		got := exitCode(unknown)
		if got != ExitGeneral {
			t.Errorf("exitCode(unknown) = %d, want %d (ExitGeneral)", got, ExitGeneral)
		}
	})

	t.Run("context_canceled", func(t *testing.T) {
		got := exitCode(context.Canceled)
		if got != ExitInterrupt {
			t.Errorf("exitCode(context.Canceled) = %d, want %d (ExitInterrupt)", got, ExitInterrupt)
		}
	})

	t.Run("context_canceled_wrapped", func(t *testing.T) {
		wrapped := fmt.Errorf("operation interrupted: %w", context.Canceled)
		got := exitCode(wrapped)
		if got != ExitInterrupt {
			t.Errorf("exitCode(wrapped context.Canceled) = %d, want %d (ExitInterrupt)", got, ExitInterrupt)
		}
	})
}

// TestExitCode_CobraErrors verifies that Cobra flag errors are correctly mapped to ExitUsage.
// Uses real Cobra errors rather than fabricated strings for robustness.
func TestExitCode_CobraErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(cmd *cobra.Command)
		args    []string
		wantErr bool
	}{
		{
			name: "required_flag_missing",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().String("required", "", "a required flag")
				_ = cmd.MarkFlagRequired("required")
			},
			args:    []string{},
			wantErr: true,
		},
		{
			name: "unknown_flag",
			setup: func(cmd *cobra.Command) {
				// No flags defined
			},
			args:    []string{"--nonexistent"},
			wantErr: true,
		},
		{
			name: "unknown_shorthand",
			setup: func(cmd *cobra.Command) {
				// No flags defined
			},
			args:    []string{"-x"},
			wantErr: true,
		},
		{
			name: "flag_needs_argument",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().String("name", "", "a flag requiring value")
			},
			args:    []string{"--name"},
			wantErr: true,
		},
		{
			name: "invalid_argument_type",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Int("count", 0, "an integer flag")
			},
			args:    []string{"--count", "notanumber"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:           "test",
				SilenceErrors: true,
				SilenceUsage:  true,
				RunE: func(cmd *cobra.Command, args []string) error {
					return nil
				},
			}
			tc.setup(cmd)
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Execute() error = %v, wantErr %v", err, tc.wantErr)
			}

			if err != nil {
				got := exitCode(err)
				if got != ExitUsage {
					t.Errorf("exitCode(cobra error %q) = %d, want %d (ExitUsage)\nerror message: %s",
						tc.name, got, ExitUsage, err.Error())
				}
			}
		})
	}
}

// TestExitCode_CobraMutuallyExclusiveFlags verifies handling of mutually exclusive flag violations.
func TestExitCode_CobraMutuallyExclusiveFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use:           "test",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	cmd.Flags().Bool("foo", false, "foo flag")
	cmd.Flags().Bool("bar", false, "bar flag")
	cmd.MarkFlagsMutuallyExclusive("foo", "bar")
	cmd.SetArgs([]string{"--foo", "--bar"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for mutually exclusive flags")
	}

	got := exitCode(err)
	if got != ExitUsage {
		t.Errorf("exitCode(mutually exclusive flags error) = %d, want %d (ExitUsage)\nerror: %s",
			got, ExitUsage, err.Error())
	}
}
