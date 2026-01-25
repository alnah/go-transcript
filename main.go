package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
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
		Use:     "go-transcript",
		Short:   "Record, transcribe, and restructure audio sessions",
		Version: fmt.Sprintf("%s (commit: %s)", version, commit),
		// Silence Cobra's default error/usage printing; we handle it ourselves.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	// Subcommands will be added in Phase D:
	// rootCmd.AddCommand(recordCmd())
	// rootCmd.AddCommand(transcribeCmd())
	// rootCmd.AddCommand(liveCmd())

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

	// Map sentinel errors to exit codes.
	// These will be defined in errors.go (Phase B).
	//
	// if errors.Is(err, ErrFFmpegNotFound) || errors.Is(err, ErrAPIKeyMissing) {
	//     return ExitSetup
	// }
	// if errors.Is(err, ErrInvalidDuration) || errors.Is(err, ErrUnsupportedFormat) ||
	//    errors.Is(err, ErrFileNotFound) || errors.Is(err, ErrUnknownTemplate) ||
	//    errors.Is(err, ErrOutputExists) {
	//     return ExitValidation
	// }
	// if errors.Is(err, ErrRateLimit) || errors.Is(err, ErrTimeout) || errors.Is(err, ErrAuthFailed) {
	//     return ExitTranscription
	// }
	// if errors.Is(err, ErrTranscriptTooLong) {
	//     return ExitRestructure
	// }

	return ExitGeneral
}
