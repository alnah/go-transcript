package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// =============================================================================
// Pure Function Tests (no I/O, no mocks)
// =============================================================================

// TestFormatDurationHuman verifies human-readable duration formatting.
// Table-driven to cover all branches and edge cases.
func TestFormatDurationHuman(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		// Hours only
		{"1_hour", 1 * time.Hour, "1h"},
		{"2_hours", 2 * time.Hour, "2h"},
		{"10_hours", 10 * time.Hour, "10h"},

		// Hours with minutes
		{"1h30m", 1*time.Hour + 30*time.Minute, "1h30m"},
		{"2h15m", 2*time.Hour + 15*time.Minute, "2h15m"},
		{"1h1m", 1*time.Hour + 1*time.Minute, "1h1m"},

		// Minutes only
		{"30_minutes", 30 * time.Minute, "30m"},
		{"1_minute", 1 * time.Minute, "1m"},
		{"59_minutes", 59 * time.Minute, "59m"},

		// Seconds only (below 1 minute)
		{"45_seconds", 45 * time.Second, "45s"},
		{"1_second", 1 * time.Second, "1s"},
		{"59_seconds", 59 * time.Second, "59s"},

		// Edge cases
		{"zero", 0, "0s"},
		{"60_minutes_becomes_1h", 60 * time.Minute, "1h"},
		{"61_minutes", 61 * time.Minute, "1h1m"},
		{"90_minutes", 90 * time.Minute, "1h30m"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := formatDurationHuman(tc.duration)
			if got != tc.want {
				t.Errorf("formatDurationHuman(%v) = %q, want %q", tc.duration, got, tc.want)
			}
		})
	}
}

// TestFormatSize verifies human-readable size formatting.
// Table-driven to cover byte/KB/MB thresholds.
func TestFormatSize(t *testing.T) {
	t.Parallel()

	const kb = 1024
	const mb = 1024 * kb

	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		// Bytes (< 1KB)
		{"zero", 0, "0 bytes"},
		{"1_byte", 1, "1 bytes"},
		{"100_bytes", 100, "100 bytes"},
		{"1023_bytes", 1023, "1023 bytes"},

		// KB (>= 1KB, < 1MB)
		{"1_KB", kb, "1 KB"},
		{"1_KB_plus_1", kb + 1, "1 KB"},
		{"10_KB", 10 * kb, "10 KB"},
		{"1023_KB", 1023 * kb, "1023 KB"},

		// MB (>= 1MB)
		{"1_MB", mb, "1 MB"},
		{"1_MB_plus_1", mb + 1, "1 MB"},
		{"10_MB", 10 * mb, "10 MB"},
		{"100_MB", 100 * mb, "100 MB"},
		{"1_GB", 1024 * mb, "1024 MB"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := formatSize(tc.bytes)
			if got != tc.want {
				t.Errorf("formatSize(%d) = %q, want %q", tc.bytes, got, tc.want)
			}
		})
	}
}

// TestDefaultRecordingFilename_Format verifies the default filename format.
// Uses regex to validate format without mocking time.Now().
// Pattern: recording_YYYYMMDD_HHMMSS.ogg
func TestDefaultRecordingFilename_Format(t *testing.T) {
	t.Parallel()

	// Pattern: recording_20260125_143052.ogg
	pattern := regexp.MustCompile(`^recording_\d{8}_\d{6}\.ogg$`)

	filename := defaultRecordingFilename()

	if !pattern.MatchString(filename) {
		t.Errorf("defaultRecordingFilename() = %q, want format recording_YYYYMMDD_HHMMSS.ogg", filename)
	}

	// Additional checks for sanity
	if len(filename) != len("recording_20060102_150405.ogg") {
		t.Errorf("defaultRecordingFilename() length = %d, want %d", len(filename), len("recording_20060102_150405.ogg"))
	}
}

// =============================================================================
// Cobra Flag Validation Tests
// =============================================================================

// TestRecordCmd_DurationRequired verifies that --duration is a required flag.
// Uses real Cobra command to generate authentic error messages.
func TestRecordCmd_DurationRequired(t *testing.T) {
	t.Parallel()

	cmd := recordCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	// Override RunE to prevent actual execution
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return nil
	}

	cmd.SetArgs([]string{}) // No arguments

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing required --duration flag")
	}

	// Verify it's detected as a usage error
	got := exitCode(err)
	if got != ExitUsage {
		t.Errorf("exitCode(missing duration) = %d, want %d (ExitUsage)\nerror: %s", got, ExitUsage, err)
	}
}

// TestRecordCmd_DurationInvalid verifies invalid duration formats are rejected.
// Table-driven to cover various invalid formats.
func TestRecordCmd_DurationInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration string
	}{
		{"alphabetic", "abc"},
		{"full_word", "2hours"},
		{"negative", "-1h"},
		{"invalid_unit", "30x"},
		{"spaces", "1 h"},
		{"empty_string", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd := recordCmd()
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetArgs([]string{"-d", tc.duration})

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error for invalid duration %q", tc.duration)
			}

			// Should be ErrInvalidDuration (ExitValidation)
			assertError(t, err, ErrInvalidDuration)

			got := exitCode(err)
			if got != ExitValidation {
				t.Errorf("exitCode(invalid duration %q) = %d, want %d (ExitValidation)\nerror: %s",
					tc.duration, got, ExitValidation, err)
			}
		})
	}
}

// TestRecordCmd_DurationZero verifies that zero duration is rejected.
func TestRecordCmd_DurationZero(t *testing.T) {
	t.Parallel()

	cmd := recordCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"-d", "0s"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for zero duration")
	}

	assertError(t, err, ErrInvalidDuration)
	assertContains(t, err.Error(), "positive")
}

// TestRecordCmd_LoopbackMixMutuallyExclusive verifies --loopback and --mix cannot be used together.
func TestRecordCmd_LoopbackMixMutuallyExclusive(t *testing.T) {
	t.Parallel()

	cmd := recordCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	// Override RunE to prevent actual execution (we only test flag parsing)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return nil
	}

	cmd.SetArgs([]string{"-d", "1h", "--loopback", "--mix"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for mutually exclusive --loopback and --mix flags")
	}

	got := exitCode(err)
	if got != ExitUsage {
		t.Errorf("exitCode(loopback+mix) = %d, want %d (ExitUsage)\nerror: %s", got, ExitUsage, err)
	}
}

// =============================================================================
// Filesystem Tests
// =============================================================================

// TestRunRecord_OutputExists verifies that runRecord fails if output file already exists.
// Uses tempFile helper to create existing file.
func TestRunRecord_OutputExists(t *testing.T) {
	// Note: Not parallel because runRecord may access global state (config, FFmpeg)
	// However, we test early exit before those are reached

	// Create a temporary file that "already exists"
	dir := t.TempDir()
	existingFile := filepath.Join(dir, "existing.ogg")
	if err := os.WriteFile(existingFile, []byte("existing content"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	opts := recordOptions{
		duration: 1 * time.Hour,
		output:   existingFile,
	}

	// runRecord should fail because output exists
	// Note: This will try to LoadConfig and resolveFFmpeg, but should fail fast
	// before those due to output file check... actually no, the check is AFTER config load.
	// Let's verify the error handling is correct anyway.

	ctx := context.Background()
	err := runRecord(ctx, opts)

	// Should fail with ErrOutputExists
	if err == nil {
		t.Fatal("expected error for existing output file")
	}

	assertError(t, err, ErrOutputExists)
	assertContains(t, err.Error(), existingFile)
}

// TestRunRecord_OutputExistsBeforeFFmpeg verifies the check order:
// output existence is checked AFTER config load but BEFORE FFmpeg resolution.
// This is important because FFmpeg resolution can take time (download).
func TestRunRecord_OutputExistsBeforeFFmpeg(t *testing.T) {
	// This test documents the current behavior order in runRecord():
	// 1. LoadConfig()
	// 2. ResolveOutputPath()
	// 3. Warning for non-.ogg extension
	// 4. Check if output exists <-- ErrOutputExists returned here
	// 5. resolveFFmpeg() <-- NOT reached if output exists

	dir := t.TempDir()
	existingFile := filepath.Join(dir, "test.ogg")
	if err := os.WriteFile(existingFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	opts := recordOptions{
		duration: 1 * time.Hour,
		output:   existingFile,
	}

	err := runRecord(context.Background(), opts)

	// Must be ErrOutputExists, not ErrFFmpegNotFound
	// (proves the check happens before FFmpeg resolution)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errorIs(err, ErrOutputExists) {
		t.Errorf("expected ErrOutputExists, got: %v", err)
	}
}

// =============================================================================
// createRecorder Tests
// =============================================================================

// TestCreateRecorder_ModeSelection verifies correct recorder type is created for each mode.
// Note: This test requires FFmpeg to be available, so it may fail in minimal CI environments.
// The actual recorder behavior is tested in integration tests (Phase G).
func TestCreateRecorder_ModeSelection(t *testing.T) {
	// Skip if FFmpeg is not available (this is a unit test, integration tests will cover this)
	if testing.Short() {
		t.Skip("skipping in short mode (requires FFmpeg)")
	}

	ctx := context.Background()

	// Try to resolve FFmpeg - if not available, skip
	ffmpegPath, err := resolveFFmpeg(ctx)
	if err != nil {
		t.Skipf("FFmpeg not available: %v", err)
	}

	t.Run("microphone_mode", func(t *testing.T) {
		recorder, err := createRecorder(ctx, ffmpegPath, "", false, false)
		if err != nil {
			t.Fatalf("createRecorder(mic) failed: %v", err)
		}
		if recorder == nil {
			t.Error("createRecorder(mic) returned nil")
		}
		// Type assertion to verify it's FFmpegRecorder
		if _, ok := recorder.(*FFmpegRecorder); !ok {
			t.Errorf("createRecorder(mic) returned %T, want *FFmpegRecorder", recorder)
		}
	})

	// Loopback and Mix modes require loopback device detection
	// which may not be available on all systems - skip if detection fails
	t.Run("loopback_mode", func(t *testing.T) {
		recorder, err := createRecorder(ctx, ffmpegPath, "", true, false)
		if err != nil {
			// Expected to fail if no loopback device
			if errorIs(err, ErrLoopbackNotFound) {
				t.Skipf("loopback device not available: %v", err)
			}
			t.Fatalf("createRecorder(loopback) failed: %v", err)
		}
		if recorder == nil {
			t.Error("createRecorder(loopback) returned nil")
		}
	})

	t.Run("mix_mode", func(t *testing.T) {
		recorder, err := createRecorder(ctx, ffmpegPath, "", false, true)
		if err != nil {
			if errorIs(err, ErrLoopbackNotFound) {
				t.Skipf("loopback device not available: %v", err)
			}
			t.Fatalf("createRecorder(mix) failed: %v", err)
		}
		if recorder == nil {
			t.Error("createRecorder(mix) returned nil")
		}
	})
}
