//go:build integration

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Test Helpers
// =============================================================================

// skipIfNoFFprobe skips the test if ffprobe is not available.
// Returns the path to ffprobe if found.
func skipIfNoFFprobe(t *testing.T) string {
	t.Helper()

	path, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skipf("ffprobe not available: %v", err)
	}

	return path
}

// getAudioDuration returns the duration of an audio file in seconds using ffprobe.
// Returns -1 if duration cannot be determined.
func getAudioDuration(t *testing.T, ffprobePath, audioPath string) float64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		audioPath,
	)

	output, err := cmd.Output()
	if err != nil {
		t.Logf("ffprobe failed: %v", err)
		return -1
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		t.Logf("failed to parse duration %q: %v", string(output), err)
		return -1
	}

	return duration
}

// audioInfo holds audio stream properties.
type audioInfo struct {
	codec      string
	sampleRate int
	channels   int
}

// getAudioInfo returns codec, sample rate, and channels using ffprobe.
func getAudioInfo(t *testing.T, ffprobePath, audioPath string) *audioInfo {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_name,sample_rate,channels",
		"-of", "default=noprint_wrappers=1",
		audioPath,
	)

	output, err := cmd.Output()
	if err != nil {
		t.Logf("ffprobe failed: %v", err)
		return nil
	}

	info := &audioInfo{}
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := parts[0], parts[1]
		switch key {
		case "codec_name":
			info.codec = value
		case "sample_rate":
			info.sampleRate, _ = strconv.Atoi(value)
		case "channels":
			info.channels, _ = strconv.Atoi(value)
		}
	}

	return info
}

// =============================================================================
// Integration Tests
// =============================================================================

// TestFFmpegRecorder_RecordsFromLavfi verifies that FFmpeg can produce a valid
// OGG file with the expected format (codec, sample rate, channels).
// Uses lavfi virtual device to avoid hardware dependency.
func TestFFmpegRecorder_RecordsFromLavfi(t *testing.T) {
	t.Parallel()

	ffmpegPath := skipIfNoFFmpeg(t)
	ffprobePath := skipIfNoFFprobe(t)

	output := filepath.Join(t.TempDir(), "test_record.ogg")
	const targetDuration = 2 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Record synthetic audio
	err := recordWithLavfi(ctx, ffmpegPath, targetDuration, output)
	assertNoError(t, err)

	// Verify file exists and has OGG magic bytes
	// Expected size: ~50kbps * 2s = ~12.5KB, allow 5KB-50KB range
	assertOggFile(t, output, 5*1024, 50*1024)

	// Verify audio format via ffprobe
	info := getAudioInfo(t, ffprobePath, output)
	if info == nil {
		t.Fatal("failed to get audio info")
	}

	if info.codec != "vorbis" {
		t.Errorf("expected codec vorbis, got %s", info.codec)
	}
	if info.sampleRate != 16000 {
		t.Errorf("expected sample rate 16000, got %d", info.sampleRate)
	}
	if info.channels != 1 {
		t.Errorf("expected 1 channel (mono), got %d", info.channels)
	}

	// Verify duration is approximately correct (±0.5s tolerance)
	duration := getAudioDuration(t, ffprobePath, output)
	if duration < 0 {
		t.Fatal("failed to get audio duration")
	}
	targetSec := targetDuration.Seconds()
	if duration < targetSec-0.5 || duration > targetSec+0.5 {
		t.Errorf("expected duration ~%.1fs, got %.2fs", targetSec, duration)
	}

	t.Logf("Recorded %.2fs of audio to %s (%d bytes)", duration, output, testFileSize(t, output))
}

// TestFFmpegRecorder_StopsOnContextCancel verifies that recording stops gracefully
// when the context is canceled, producing a valid (truncated) audio file.
// This tests the 'q' stdin protocol for graceful FFmpeg shutdown.
func TestFFmpegRecorder_StopsOnContextCancel(t *testing.T) {
	// NOT parallel - timing-sensitive test (SWOT decision)

	ffmpegPath := skipIfNoFFmpeg(t)
	ffprobePath := skipIfNoFFprobe(t)

	output := filepath.Join(t.TempDir(), "test_interrupt.ogg")
	const originalDuration = 10 * time.Second // Request 10s recording
	const cancelAfter = 2 * time.Second       // Cancel after 2s

	ctx, cancel := context.WithCancel(context.Background())

	// Start recording in background
	// Use recordWithLavfiRealtime (anullsrc) instead of recordWithLavfi (sine)
	// because sine=duration=N generates data instantly, while anullsrc runs in real-time
	errCh := make(chan error, 1)
	go func() {
		errCh <- recordWithLavfiRealtime(ctx, ffmpegPath, originalDuration, output)
	}()

	// Wait for FFmpeg to start and begin recording
	time.Sleep(500 * time.Millisecond)

	// Cancel context after target duration
	time.Sleep(cancelAfter - 500*time.Millisecond)
	cancel()

	// Wait for recording to finish
	err := <-errCh

	// Error is acceptable (FFmpeg returns non-zero on interrupt)
	// but file should still be valid
	if err != nil {
		t.Logf("Recording returned error (expected on interrupt): %v", err)
	}

	// File should exist and be valid OGG
	if _, statErr := os.Stat(output); os.IsNotExist(statErr) {
		t.Fatalf("output file was not created: %v", statErr)
	}

	// Verify OGG magic bytes (file should be finalized)
	assertOggFile(t, output, 1*1024, 100*1024) // At least 1KB, max 100KB

	// Verify duration is truncated (not 10s)
	// SWOT decision: accept cancelAfter-0.5s to cancelAfter+2s
	duration := getAudioDuration(t, ffprobePath, output)
	if duration < 0 {
		// File may not be fully parseable after interrupt - that's acceptable
		// as long as OGG magic bytes are present
		t.Logf("Could not parse duration (file may be truncated), but OGG header is valid")
		return
	}

	cancelSec := cancelAfter.Seconds()
	originalSec := originalDuration.Seconds()
	minExpected := cancelSec - 0.5
	maxExpected := cancelSec + 2.0

	if duration < minExpected {
		t.Errorf("duration %.2fs is less than minimum expected %.2fs", duration, minExpected)
	}
	if duration >= originalSec-0.5 {
		t.Errorf("duration %.2fs suggests cancel was ignored (expected < %.2fs)", duration, originalSec)
	}
	if duration > maxExpected {
		t.Logf("Warning: duration %.2fs is higher than expected %.2fs (FFmpeg finalization delay)", duration, maxExpected)
	}

	t.Logf("Interrupted recording: requested %.1fs, canceled after %.1fs, got %.2fs", originalSec, cancelSec, duration)
}

// TestFFmpegRecorder_ListDevices verifies that ListDevices does not crash
// and returns without error. We cannot verify the content as it depends
// on the system's audio devices.
func TestFFmpegRecorder_ListDevices(t *testing.T) {
	t.Parallel()

	ffmpegPath := skipIfNoFFmpeg(t)

	recorder, err := NewFFmpegRecorder(ffmpegPath, "")
	assertNoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	devices, err := recorder.ListDevices(ctx)
	// Error is acceptable on some platforms (e.g., no audio subsystem in CI)
	if err != nil {
		t.Logf("ListDevices returned error (may be expected in CI): %v", err)
		return
	}

	t.Logf("Found %d audio devices: %v", len(devices), devices)
}

// TestFFmpegRecorder_NewRecorderValidation verifies constructor validation.
func TestFFmpegRecorder_NewRecorderValidation(t *testing.T) {
	t.Parallel()

	// Empty path should fail
	_, err := NewFFmpegRecorder("", "")
	assertError(t, err, ErrFFmpegNotFound)

	// Valid path should succeed
	ffmpegPath := skipIfNoFFmpeg(t)
	recorder, err := NewFFmpegRecorder(ffmpegPath, "")
	assertNoError(t, err)

	if recorder == nil {
		t.Error("expected non-nil recorder")
	}
}

// =============================================================================
// Helper utilities
// =============================================================================

// testFileSize returns the size of a file in bytes, or 0 on error.
// Named differently from fileSize in cmd_record.go to avoid conflict.
func testFileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
