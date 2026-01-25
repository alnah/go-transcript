package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// gracefulShutdownTimeout is the time to wait for FFmpeg to finalize the file
// after sending 'q' to stdin before forcefully killing the process.
const gracefulShutdownTimeout = 5 * time.Second

// ErrFFmpegTimeout is returned when FFmpeg does not exit within the graceful shutdown timeout.
var ErrFFmpegTimeout = errors.New("ffmpeg did not exit within timeout")

// runFFmpegGraceful executes FFmpeg with graceful shutdown on context cancellation.
// When ctx is canceled, it sends 'q' to stdin to allow FFmpeg to finalize the file
// properly (write headers, close container), then waits up to timeout before killing.
// This approach works cross-platform (Windows/macOS/Linux) unlike SIGTERM.
func runFFmpegGraceful(ctx context.Context, ffmpegPath string, args []string, timeout time.Duration) error {
	cmd := exec.Command(ffmpegPath, args...)

	// Create stdin pipe for graceful shutdown via 'q' command.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	// Capture stderr for error messages (FFmpeg writes most output to stderr).
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Channel to receive the result of cmd.Wait().
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// FFmpeg completed normally (or with error).
		if err != nil {
			return fmt.Errorf("ffmpeg failed: %w\nOutput: %s", err, stderr.String())
		}
		return nil

	case <-ctx.Done():
		// Context canceled - initiate graceful shutdown.
		// Send 'q' to FFmpeg stdin to request graceful exit.
		_, _ = io.WriteString(stdin, "q")
		_ = stdin.Close()

		// Wait for FFmpeg to exit gracefully or timeout.
		select {
		case err := <-done:
			// FFmpeg exited after receiving 'q'.
			if err != nil {
				// Exit code != 0 is expected when interrupted, check if file was written.
				// FFmpeg returns error on interrupt but file should be valid.
				return nil
			}
			return nil

		case <-time.After(timeout):
			// Timeout reached - force kill.
			_ = cmd.Process.Kill()
			<-done // Wait for process to be reaped.
			return fmt.Errorf("%w: killed after %v", ErrFFmpegTimeout, timeout)
		}
	}
}

// ffmpegOutputFunc is the function signature for running FFmpeg and capturing output.
// This variable can be replaced in tests to mock FFmpeg behavior.
var runFFmpegOutputFunc = runFFmpegOutputImpl

// runFFmpegOutput executes FFmpeg and captures its stderr output.
// FFmpeg writes most diagnostic output (including device lists, probe info) to stderr.
// This is useful for commands like -list_devices, -i with probe, silencedetect filter, etc.
func runFFmpegOutput(ctx context.Context, ffmpegPath string, args []string) (string, error) {
	return runFFmpegOutputFunc(ctx, ffmpegPath, args)
}

// runFFmpegOutputImpl is the real implementation of runFFmpegOutput.
func runFFmpegOutputImpl(ctx context.Context, ffmpegPath string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// FFmpeg -list_devices returns exit code 1, so we ignore the error
	// and just return the stderr output for parsing.
	_ = cmd.Run()

	return stderr.String(), nil
}
