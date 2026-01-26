package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

// Injected at build time via ldflags.
var (
	version = "dev"
	commit  = "unknown"
)

// Exit codes per specification.
const (
	ExitOK            = 0
	ExitGeneral       = 1
	ExitUsage         = 2
	ExitSetup         = 3
	ExitValidation    = 4
	ExitTranscription = 5
	ExitRestructure   = 6
	ExitInterrupt     = 130
)

func main() {
	// Load .env file if present (ignore error if missing).
	_ = godotenv.Load()

	// Context with signal cancellation.
	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Root command.
	rootCmd := &cobra.Command{
		Use:     "transcript",
		Short:   "Record, transcribe, and restructure audio sessions",
		Version: fmt.Sprintf("%s (commit: %s)", version, commit),
		// Silence Cobra's default error/usage printing; we handle it ourselves.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	// Subcommands.
	rootCmd.AddCommand(recordCmd())
	rootCmd.AddCommand(transcribeCmd())
	rootCmd.AddCommand(liveCmd())
	rootCmd.AddCommand(configCmd())

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitCode(err))
	}
}

// exitCode maps errors to spec-defined exit codes.
func exitCode(err error) int {
	if err == nil {
		return ExitOK
	}

	// Check for context cancellation (interrupt).
	if errors.Is(err, context.Canceled) {
		return ExitInterrupt
	}

	// Usage errors (ExitUsage = 2): Cobra flag/arg parsing errors.
	// Cobra doesn't expose typed errors, so we check for known error message patterns.
	// These patterns are stable across Cobra versions (tested with v1.8+).
	if isCobraUsageError(err) {
		return ExitUsage
	}

	// Setup errors (ExitSetup = 3).
	if errors.Is(err, ErrFFmpegNotFound) || errors.Is(err, ErrAPIKeyMissing) ||
		errors.Is(err, ErrNoAudioDevice) || errors.Is(err, ErrLoopbackNotFound) ||
		errors.Is(err, ErrUnsupportedPlatform) || errors.Is(err, ErrChecksumMismatch) ||
		errors.Is(err, ErrDownloadFailed) {
		return ExitSetup
	}

	// Validation errors (ExitValidation = 4).
	if errors.Is(err, ErrInvalidDuration) || errors.Is(err, ErrUnsupportedFormat) ||
		errors.Is(err, ErrFileNotFound) || errors.Is(err, ErrUnknownTemplate) ||
		errors.Is(err, ErrOutputExists) || errors.Is(err, ErrChunkingFailed) ||
		errors.Is(err, ErrChunkTooLarge) || errors.Is(err, ErrInvalidLanguage) {
		return ExitValidation
	}

	// Transcription errors (ExitTranscription = 5).
	if errors.Is(err, ErrRateLimit) || errors.Is(err, ErrQuotaExceeded) ||
		errors.Is(err, ErrTimeout) || errors.Is(err, ErrAuthFailed) {
		return ExitTranscription
	}

	// Restructure errors (ExitRestructure = 6).
	if errors.Is(err, ErrTranscriptTooLong) {
		return ExitRestructure
	}

	return ExitGeneral
}

// cobraUsageErrorPatterns contains error message substrings that indicate Cobra usage errors.
// These patterns are stable across Cobra versions (tested with v1.8+).
// Cobra doesn't expose typed errors, so string matching is the only reliable approach.
var cobraUsageErrorPatterns = []string{
	"required flag",             // Missing required flag
	"unknown flag",              // Flag doesn't exist
	"unknown shorthand",         // Short flag doesn't exist
	"flag needs an argument",    // Flag provided without value
	"invalid argument",          // Invalid flag value type
	"if any flags in the group", // Mutually exclusive flag violation
	"accepts ",                  // Wrong number of arguments (e.g., "accepts 1 arg(s)")
	"requires at least",         // Too few arguments
	"requires at most",          // Too many arguments
}

// isCobraUsageError checks if an error is a Cobra usage/parsing error.
func isCobraUsageError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	for _, pattern := range cobraUsageErrorPatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}
	return false
}
