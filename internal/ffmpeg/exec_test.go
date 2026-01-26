package ffmpeg

// Notes:
// - RunGraceful tests use real processes (cat, sleep) to test graceful shutdown behavior
// - RunOutput tests use Executor with injected runOutput function
// - CheckVersion tests use Executor with mock runOutput
// - All tests can run in parallel since there's no global state modification

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Executor.RunOutput - FFmpeg output capture
// ---------------------------------------------------------------------------

func TestExecutor_RunOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mockOutput string
		mockErr    error
		wantOutput string
		wantErr    bool
	}{
		{
			name:       "returns stderr output",
			mockOutput: "ffmpeg version 6.1.1",
			mockErr:    nil,
			wantOutput: "ffmpeg version 6.1.1",
			wantErr:    false,
		},
		{
			name:       "returns empty output",
			mockOutput: "",
			mockErr:    nil,
			wantOutput: "",
			wantErr:    false,
		},
		{
			name:       "returns error",
			mockOutput: "",
			mockErr:    errors.New("command failed"),
			wantOutput: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			executor := NewExecutor(
				WithRunOutput(func(ctx context.Context, path string, args []string) (string, error) {
					return tt.mockOutput, tt.mockErr
				}),
			)

			got, err := executor.RunOutput(context.Background(), "/usr/bin/ffmpeg", []string{"-version"})

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if got != tt.wantOutput {
					t.Errorf("got %q, want %q", got, tt.wantOutput)
				}
			}
		})
	}
}

func TestDefaultRunOutput_RealCommand(t *testing.T) {
	t.Parallel()

	// Use echo command which exists on all platforms
	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "hello"}
	} else {
		cmd = "sh"
		args = []string{"-c", "echo hello >&2"}
	}

	output, err := defaultRunOutput(context.Background(), cmd, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Output should contain "hello" (written to stderr)
	if runtime.GOOS != "windows" && !strings.Contains(output, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", output)
	}
}

func TestDefaultRunOutput_NonexistentCommand(t *testing.T) {
	t.Parallel()

	// Non-existent command returns error but also empty output.
	// Callers can choose to ignore the error and use the output.
	output, err := defaultRunOutput(context.Background(), "/nonexistent/command", []string{})
	if err == nil {
		t.Error("expected error for non-existent command")
	}
	if output != "" {
		t.Errorf("expected empty output for non-existent command, got %q", output)
	}
}

func TestDefaultRunOutput_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should return quickly without hanging
	_, err := defaultRunOutput(ctx, "sleep", []string{"10"})
	// Error is ignored by design, but the function should return quickly
	if err != nil {
		t.Logf("got error (expected for cancelled context): %v", err)
	}
}

// ---------------------------------------------------------------------------
// VersionChecker - FFmpeg version parsing
// ---------------------------------------------------------------------------

func TestVersionChecker_Check(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		versionLine    string
		expectWarning  bool
		wantWarningMsg string
	}{
		{
			name:          "version 6 - no warning",
			versionLine:   "ffmpeg version 6.1.1 Copyright (c) 2000-2023",
			expectWarning: false,
		},
		{
			name:          "version 5 - no warning",
			versionLine:   "ffmpeg version 5.0 Copyright (c) 2000-2022",
			expectWarning: false,
		},
		{
			name:          "version 4 - no warning (minimum)",
			versionLine:   "ffmpeg version 4.4.1 Copyright (c) 2000-2021",
			expectWarning: false,
		},
		{
			name:           "version 3 - warning expected",
			versionLine:    "ffmpeg version 3.4.8 Copyright (c) 2000-2020",
			expectWarning:  true,
			wantWarningMsg: "Warning: ffmpeg version 3 detected, version 4+ recommended",
		},
		{
			name:          "version n6.1.1 format",
			versionLine:   "ffmpeg version n6.1.1 Copyright (c) 2000-2023",
			expectWarning: false,
		},
		{
			name:          "unparseable version",
			versionLine:   "something unexpected",
			expectWarning: false,
		},
		{
			name:          "empty output",
			versionLine:   "",
			expectWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stderrBuf strings.Builder
			executor := NewExecutor(
				WithRunOutput(func(ctx context.Context, path string, args []string) (string, error) {
					return tt.versionLine, nil
				}),
			)
			checker := NewVersionChecker(
				WithVersionExecutor(executor),
				WithVersionStderr(&stderrBuf),
			)

			checker.Check(context.Background(), "/usr/bin/ffmpeg")

			gotWarning := stderrBuf.String()
			if tt.expectWarning {
				if !strings.Contains(gotWarning, tt.wantWarningMsg) {
					t.Errorf("expected warning containing %q, got %q", tt.wantWarningMsg, gotWarning)
				}
			} else {
				if gotWarning != "" {
					t.Errorf("expected no warning, got %q", gotWarning)
				}
			}
		})
	}
}

func TestVersionChecker_Check_RunOutputError(t *testing.T) {
	t.Parallel()

	var stderrBuf strings.Builder
	executor := NewExecutor(
		WithRunOutput(func(ctx context.Context, path string, args []string) (string, error) {
			return "", errors.New("command failed")
		}),
	)
	checker := NewVersionChecker(
		WithVersionExecutor(executor),
		WithVersionStderr(&stderrBuf),
	)

	// Should return false when RunOutput returns error with empty output
	ok := checker.Check(context.Background(), "/usr/bin/ffmpeg")
	if ok {
		t.Error("expected Check to return false on error")
	}

	// And should not produce any output
	if stderrBuf.String() != "" {
		t.Errorf("expected no output on error, got %q", stderrBuf.String())
	}
}

func TestVersionChecker_Check_ReturnValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		versionLine string
		wantOK      bool
	}{
		{
			name:        "valid version returns true",
			versionLine: "ffmpeg version 6.1.1 Copyright",
			wantOK:      true,
		},
		{
			name:        "empty output returns false",
			versionLine: "",
			wantOK:      false,
		},
		{
			name:        "unparseable returns false",
			versionLine: "not a version string",
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			executor := NewExecutor(
				WithRunOutput(func(ctx context.Context, path string, args []string) (string, error) {
					return tt.versionLine, nil
				}),
			)
			checker := NewVersionChecker(
				WithVersionExecutor(executor),
				WithVersionStderr(&strings.Builder{}),
			)

			got := checker.Check(context.Background(), "/usr/bin/ffmpeg")
			if got != tt.wantOK {
				t.Errorf("Check() = %v, want %v", got, tt.wantOK)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RunGraceful - graceful shutdown with real processes
// ---------------------------------------------------------------------------

func TestRunGraceful_NormalCompletion(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows - requires bash")
	}

	// Use a command that completes quickly
	err := RunGraceful(context.Background(), "sh", []string{"-c", "exit 0"}, time.Second)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunGraceful_CommandFails(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows - requires bash")
	}

	// Command that exits with error
	err := RunGraceful(context.Background(), "sh", []string{"-c", "exit 1"}, time.Second)
	if err == nil {
		t.Error("expected error for failed command")
	}
}

func TestRunGraceful_NonexistentCommand(t *testing.T) {
	t.Parallel()

	err := RunGraceful(context.Background(), "/nonexistent/command", []string{}, time.Second)
	if err == nil {
		t.Error("expected error for non-existent command")
	}
}

func TestRunGraceful_ContextCancellation(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows - requires cat")
	}

	// Check if cat exists
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not found in PATH")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start a long-running command that reads from stdin (cat waits for input)
	done := make(chan error, 1)
	go func() {
		done <- RunGraceful(ctx, "cat", []string{}, 5*time.Second)
	}()

	// Give the command time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context - should trigger graceful shutdown
	cancel()

	select {
	case err := <-done:
		// Graceful shutdown should return nil (not an error)
		if err != nil {
			t.Logf("got error after cancellation: %v (may be expected)", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("command did not exit after context cancellation")
	}
}

func TestRunGraceful_Timeout(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows - requires sleep")
	}

	// Check if sleep exists
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not found in PATH")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start sleep which ignores stdin 'q' command
	done := make(chan error, 1)
	go func() {
		done <- RunGraceful(ctx, "sleep", []string{"10"}, 100*time.Millisecond)
	}()

	// Give the command time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	select {
	case err := <-done:
		// Should timeout and return ErrTimeout
		if err == nil {
			t.Error("expected timeout error")
		}
		if !errors.Is(err, ErrTimeout) {
			t.Errorf("expected ErrTimeout, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("command did not exit after timeout")
	}
}
