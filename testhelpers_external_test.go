//go:build integration || e2e

package main

import (
	"context"
	"strconv"
	"testing"
	"time"
)

// =============================================================================
// Shared Test Helpers for Integration and E2E Tests
// =============================================================================

// skipIfNoFFmpeg skips the test if ffmpeg is not available.
// Returns the path to ffmpeg if found.
func skipIfNoFFmpeg(t *testing.T) string {
	t.Helper()

	// Try to find ffmpeg via resolveFFmpeg (checks FFMPEG_PATH, installed, PATH)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	path, err := resolveFFmpeg(ctx)
	if err != nil {
		t.Skipf("ffmpeg not available: %v", err)
	}

	return path
}

// recordWithLavfi records synthetic audio using FFmpeg's lavfi (virtual device).
// This allows testing FFmpeg recording without requiring real audio hardware.
// Uses the same encoding parameters as FFmpegRecorder (via buildRecordArgs).
// Note: sine source generates data at real-time pace, suitable for duration tests.
func recordWithLavfi(ctx context.Context, ffmpegPath string, duration time.Duration, output string) error {
	// Use lavfi to generate a sine wave - no hardware needed
	// buildRecordArgs ensures encoding matches production code
	args := buildRecordArgs("lavfi", "sine=frequency=440:duration="+strconv.Itoa(int(duration.Seconds())), duration, output)
	return runFFmpegGraceful(ctx, ffmpegPath, args, gracefulShutdownTimeout)
}

// recordWithLavfiRealtime records using anullsrc with -re flag for real-time pacing.
// This is suitable for testing context cancellation because FFmpeg processes input
// at native frame rate (1x speed), unlike default which generates data instantly.
func recordWithLavfiRealtime(ctx context.Context, ffmpegPath string, duration time.Duration, output string) error {
	// Build args manually to insert -re flag before input
	// -re forces FFmpeg to read input at native frame rate (real-time)
	args := []string{
		"-y",          // Overwrite output
		"-re",         // Read input at native frame rate (real-time)
		"-f", "lavfi", // Input format
		"-i", "anullsrc=r=16000:cl=mono", // Silent audio source at 16kHz mono
		"-t", strconv.Itoa(int(duration.Seconds())), // Duration limit
	}
	args = append(args, encodingArgs()...)
	args = append(args, output)
	return runFFmpegGraceful(ctx, ffmpegPath, args, gracefulShutdownTimeout)
}
