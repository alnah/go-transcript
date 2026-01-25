package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Groupe A: Tests fonctions de parsing (pures, parallélisables)
// =============================================================================

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"zero", 0, "00:00"},
		{"seconds_only", 45 * time.Second, "00:45"},
		{"minutes_and_seconds", 5*time.Minute + 30*time.Second, "05:30"},
		{"max_minutes", 59*time.Minute + 59*time.Second, "59:59"},
		{"one_hour", 1 * time.Hour, "01:00:00"},
		{"hours_minutes_seconds", 2*time.Hour + 15*time.Minute + 30*time.Second, "02:15:30"},
		{"ten_hours", 10*time.Hour + 5*time.Minute + 3*time.Second, "10:05:03"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatDuration(tt.duration)
			assertEqual(t, got, tt.want)
		})
	}
}

func TestFormatFFmpegTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"zero", 0, "00:00:00.000"},
		{"milliseconds", 500 * time.Millisecond, "00:00:00.500"},
		{"seconds", 30 * time.Second, "00:00:30.000"},
		{"minute_and_half", 1*time.Minute + 30*time.Second + 500*time.Millisecond, "00:01:30.500"},
		{"one_hour", 1 * time.Hour, "01:00:00.000"},
		{"ninety_seconds", 90 * time.Second, "00:01:30.000"},
		{"complex", 2*time.Hour + 30*time.Minute + 45*time.Second + 123*time.Millisecond, "02:30:45.123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatFFmpegTime(tt.duration)
			assertEqual(t, got, tt.want)
		})
	}
}

func TestParseDurationFromFFmpegOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		output    string
		want      time.Duration
		wantError bool
	}{
		{
			name:   "duration_pattern",
			output: "Duration: 00:05:23.45, start: 0.000000, bitrate: 48 kb/s",
			want:   5*time.Minute + 23*time.Second + 450*time.Millisecond,
		},
		{
			name:   "time_pattern",
			output: "frame=0 fps=0.0 q=0.0 size=N/A time=00:01:30.00 bitrate=N/A",
			want:   1*time.Minute + 30*time.Second,
		},
		{
			name: "multiple_time_uses_last",
			output: `time=00:00:30.00
time=00:01:00.00
time=00:01:30.00`,
			want: 1*time.Minute + 30*time.Second,
		},
		{
			name:   "duration_takes_precedence",
			output: "Duration: 00:02:00.00\ntime=00:01:30.00",
			want:   2 * time.Minute,
		},
		{
			name:      "no_pattern",
			output:    "some random output without duration",
			wantError: true,
		},
		{
			name:      "empty_output",
			output:    "",
			wantError: true,
		},
		{
			name:   "centiseconds_2_digits",
			output: "Duration: 00:00:10.45",
			want:   10*time.Second + 450*time.Millisecond,
		},
		{
			name:   "centiseconds_3_digits",
			output: "Duration: 00:00:10.456",
			want:   10*time.Second + 456*time.Millisecond,
		},
		{
			name:   "centiseconds_1_digit",
			output: "Duration: 00:00:10.4",
			want:   10*time.Second + 400*time.Millisecond,
		},
		{
			name:   "centiseconds_more_than_3_digits",
			output: "Duration: 00:00:10.456789",
			want:   10*time.Second + 456*time.Millisecond, // Truncated
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseDurationFromFFmpegOutput(tt.output)
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			assertNoError(t, err)
			assertEqual(t, got, tt.want)
		})
	}
}

func TestParseTimeComponents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		hours        string
		minutes      string
		seconds      string
		centiseconds string
		want         time.Duration
	}{
		{"zero", "00", "00", "00", "00", 0},
		{"simple", "01", "30", "45", "00", 1*time.Hour + 30*time.Minute + 45*time.Second},
		{"one_digit_cs", "00", "00", "10", "4", 10*time.Second + 400*time.Millisecond},
		{"two_digit_cs", "00", "00", "10", "45", 10*time.Second + 450*time.Millisecond},
		{"three_digit_cs", "00", "00", "10", "456", 10*time.Second + 456*time.Millisecond},
		{"more_than_3_digit_cs", "00", "00", "10", "456789", 10*time.Second + 456*time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseTimeComponents(tt.hours, tt.minutes, tt.seconds, tt.centiseconds)
			assertNoError(t, err)
			assertEqual(t, got, tt.want)
		})
	}
}

func TestParseSilenceOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   string
		expected []silencePoint
	}{
		{
			name:     "empty_output",
			output:   "",
			expected: nil,
		},
		{
			name:     "no_silences",
			output:   "frame=0 fps=0.0 q=0.0 size=N/A time=00:01:30.00",
			expected: nil,
		},
		{
			name: "single_silence",
			output: `[silencedetect @ 0x7f8e4c004a00] silence_start: 5.234
[silencedetect @ 0x7f8e4c004a00] silence_end: 6.123 | silence_duration: 0.889`,
			expected: []silencePoint{
				{start: 5234 * time.Millisecond, end: 6123 * time.Millisecond},
			},
		},
		{
			name: "multiple_silences",
			output: `[silencedetect @ 0x7f8e4c004a00] silence_start: 5.234
[silencedetect @ 0x7f8e4c004a00] silence_end: 6.123 | silence_duration: 0.889
[silencedetect @ 0x7f8e4c004a00] silence_start: 42.567
[silencedetect @ 0x7f8e4c004a00] silence_end: 43.891 | silence_duration: 1.324`,
			expected: []silencePoint{
				{start: 5234 * time.Millisecond, end: 6123 * time.Millisecond},
				{start: 42567 * time.Millisecond, end: 43891 * time.Millisecond},
			},
		},
		{
			name: "start_without_end_ignored",
			output: `[silencedetect @ 0x0] silence_start: 10.0
[silencedetect @ 0x0] silence_start: 20.0
[silencedetect @ 0x0] silence_end: 21.0 | silence_duration: 1.0`,
			expected: []silencePoint{
				{start: 20 * time.Second, end: 21 * time.Second},
			},
		},
		{
			name: "end_without_start_ignored",
			output: `[silencedetect @ 0x0] silence_end: 5.0 | silence_duration: 1.0
[silencedetect @ 0x0] silence_start: 10.0
[silencedetect @ 0x0] silence_end: 11.0 | silence_duration: 1.0`,
			expected: []silencePoint{
				{start: 10 * time.Second, end: 11 * time.Second},
			},
		},
		{
			name: "different_address_formats",
			output: `[silencedetect @ 0x0] silence_start: 1.0
[silencedetect @ 0x0] silence_end: 2.0 | silence_duration: 1.0
[silencedetect @ 0x7fffffff] silence_start: 3.0
[silencedetect @ 0x7fffffff] silence_end: 4.0 | silence_duration: 1.0`,
			expected: []silencePoint{
				{start: 1 * time.Second, end: 2 * time.Second},
				{start: 3 * time.Second, end: 4 * time.Second},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseSilenceOutput(tt.output)

			if len(got) != len(tt.expected) {
				t.Fatalf("got %d silences, want %d", len(got), len(tt.expected))
			}

			for i, s := range got {
				if s.start != tt.expected[i].start || s.end != tt.expected[i].end {
					t.Errorf("silence %d: got {%v, %v}, want {%v, %v}",
						i, s.start, s.end, tt.expected[i].start, tt.expected[i].end)
				}
			}
		})
	}
}

func TestParseSilenceOutput_Fixture(t *testing.T) {
	t.Parallel()

	output := loadFixture(t, "silence_detect_output.txt")
	silences := parseSilenceOutput(output)

	// Fixture contains 3 silences per testdata/silence_detect_output.txt
	assertEqual(t, len(silences), 3)

	// Verify first silence
	if silences[0].start != 5234*time.Millisecond {
		t.Errorf("first silence start: got %v, want %v", silences[0].start, 5234*time.Millisecond)
	}
	if silences[0].end != 6123*time.Millisecond {
		t.Errorf("first silence end: got %v, want %v", silences[0].end, 6123*time.Millisecond)
	}

	// Verify last silence
	if silences[2].start != 85123*time.Millisecond {
		t.Errorf("last silence start: got %v, want %v", silences[2].start, 85123*time.Millisecond)
	}
}

// =============================================================================
// Groupe B/C: Tests constructeurs (table-driven unifié)
// =============================================================================

func TestNewTimeChunker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ffmpegPath     string
		targetDuration time.Duration
		overlap        time.Duration
		wantError      error
		checkDefaults  bool
	}{
		{
			name:           "valid_params",
			ffmpegPath:     "/usr/bin/ffmpeg",
			targetDuration: 5 * time.Minute,
			overlap:        30 * time.Second,
		},
		{
			name:       "empty_path",
			ffmpegPath: "",
			wantError:  ErrFFmpegNotFound,
		},
		{
			name:           "overlap_equals_duration",
			ffmpegPath:     "/usr/bin/ffmpeg",
			targetDuration: 5 * time.Minute,
			overlap:        5 * time.Minute,
			wantError:      nil, // Custom error, not sentinel
		},
		{
			name:           "overlap_exceeds_duration",
			ffmpegPath:     "/usr/bin/ffmpeg",
			targetDuration: 5 * time.Minute,
			overlap:        10 * time.Minute,
			wantError:      nil, // Custom error, not sentinel
		},
		{
			name:           "zero_duration_uses_default",
			ffmpegPath:     "/usr/bin/ffmpeg",
			targetDuration: 0,
			overlap:        0,
			checkDefaults:  true,
		},
		{
			name:           "negative_overlap_corrected",
			ffmpegPath:     "/usr/bin/ffmpeg",
			targetDuration: 5 * time.Minute,
			overlap:        -1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tc, err := NewTimeChunker(tt.ffmpegPath, tt.targetDuration, tt.overlap)

			if tt.wantError != nil {
				assertError(t, err, tt.wantError)
				return
			}

			// Special case: overlap >= duration returns non-sentinel error
			if tt.name == "overlap_equals_duration" || tt.name == "overlap_exceeds_duration" {
				if err == nil {
					t.Error("expected error for invalid overlap")
				}
				return
			}

			assertNoError(t, err)

			if tt.checkDefaults {
				assertEqual(t, tc.targetDuration, defaultTargetDuration)
				assertEqual(t, tc.overlap, time.Duration(0))
			}
		})
	}
}

func TestNewSilenceChunker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ffmpegPath string
		opts       []SilenceChunkerOption
		wantError  error
		verify     func(*testing.T, *SilenceChunker)
	}{
		{
			name:       "valid_no_options",
			ffmpegPath: "/usr/bin/ffmpeg",
			verify: func(t *testing.T, sc *SilenceChunker) {
				assertEqual(t, sc.noiseDB, defaultNoiseDB)
				assertEqual(t, sc.minSilence, defaultMinSilence)
				assertEqual(t, sc.maxChunkSize, int64(defaultMaxChunkSize))
				if sc.fallback == nil {
					t.Error("expected fallback to be created")
				}
			},
		},
		{
			name:       "empty_path",
			ffmpegPath: "",
			wantError:  ErrFFmpegNotFound,
		},
		{
			name:       "with_noise_db",
			ffmpegPath: "/usr/bin/ffmpeg",
			opts:       []SilenceChunkerOption{WithNoiseDB(-40)},
			verify: func(t *testing.T, sc *SilenceChunker) {
				assertEqual(t, sc.noiseDB, -40.0)
			},
		},
		{
			name:       "with_min_silence",
			ffmpegPath: "/usr/bin/ffmpeg",
			opts:       []SilenceChunkerOption{WithMinSilence(1 * time.Second)},
			verify: func(t *testing.T, sc *SilenceChunker) {
				assertEqual(t, sc.minSilence, 1*time.Second)
			},
		},
		{
			name:       "with_max_chunk_size",
			ffmpegPath: "/usr/bin/ffmpeg",
			opts:       []SilenceChunkerOption{WithMaxChunkSize(10 * 1024 * 1024)},
			verify: func(t *testing.T, sc *SilenceChunker) {
				assertEqual(t, sc.maxChunkSize, int64(10*1024*1024))
			},
		},
		{
			name:       "with_custom_fallback",
			ffmpegPath: "/usr/bin/ffmpeg",
			opts:       []SilenceChunkerOption{WithFallback(&mockChunker{})},
			verify: func(t *testing.T, sc *SilenceChunker) {
				if _, ok := sc.fallback.(*mockChunker); !ok {
					t.Error("expected custom fallback")
				}
			},
		},
		{
			name:       "multiple_options",
			ffmpegPath: "/usr/bin/ffmpeg",
			opts: []SilenceChunkerOption{
				WithNoiseDB(-35),
				WithMinSilence(750 * time.Millisecond),
				WithMaxChunkSize(15 * 1024 * 1024),
			},
			verify: func(t *testing.T, sc *SilenceChunker) {
				assertEqual(t, sc.noiseDB, -35.0)
				assertEqual(t, sc.minSilence, 750*time.Millisecond)
				assertEqual(t, sc.maxChunkSize, int64(15*1024*1024))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sc, err := NewSilenceChunker(tt.ffmpegPath, tt.opts...)

			if tt.wantError != nil {
				assertError(t, err, tt.wantError)
				return
			}

			assertNoError(t, err)
			if tt.verify != nil {
				tt.verify(t, sc)
			}
		})
	}
}

// mockChunker is a simple mock for testing custom fallback injection.
type mockChunker struct {
	chunks []Chunk
	err    error
}

func (m *mockChunker) Chunk(_ context.Context, _ string) ([]Chunk, error) {
	return m.chunks, m.err
}

// =============================================================================
// Groupe D: Tests selectCutPoints (logique pure)
// =============================================================================

func TestSelectCutPoints(t *testing.T) {
	t.Parallel()

	// Helper to create SilenceChunker with specific maxChunkSize for testing
	newTestChunker := func(maxChunkSize int64) *SilenceChunker {
		return &SilenceChunker{
			ffmpegPath:   "/usr/bin/ffmpeg",
			maxChunkSize: maxChunkSize,
		}
	}

	// Note: The algorithm cuts at the LAST valid silence BEFORE exceeding maxDuration.
	// This ensures all chunks stay under the limit (required for OpenAI's 25MB API limit).
	// The greedy approach picks the latest possible cut point that keeps the chunk valid.

	tests := []struct {
		name           string
		silences       []silencePoint
		bytesPerSecond float64
		maxChunkSize   int64
		wantCuts       int
		verifyCuts     func(*testing.T, []time.Duration)
	}{
		{
			name:           "no_silences",
			silences:       nil,
			bytesPerSecond: 6000,
			maxChunkSize:   600000, // 100s at 6000 bytes/s
			wantCuts:       0,
		},
		{
			name: "file_fits_in_one_chunk",
			silences: []silencePoint{
				{start: 10 * time.Second, end: 11 * time.Second},
				{start: 30 * time.Second, end: 31 * time.Second},
			},
			bytesPerSecond: 6000,
			maxChunkSize:   600000, // 100s - file fits
			wantCuts:       0,
		},
		{
			name: "cuts_at_last_valid_silence_before_limit",
			silences: []silencePoint{
				{start: 30 * time.Second, end: 31 * time.Second},   // 30.5s < 50s, candidate
				{start: 60 * time.Second, end: 61 * time.Second},   // 60.5s >= 50s, CUT at 30.5s, then 60.5-30.5=30s < 50s, candidate
				{start: 90 * time.Second, end: 91 * time.Second},   // 90.5-30.5=60s >= 50s, CUT at 60.5s, then 90.5-60.5=30s < 50s, candidate
				{start: 120 * time.Second, end: 121 * time.Second}, // 120.5-60.5=60s >= 50s, CUT at 90.5s
			},
			bytesPerSecond: 6000,
			maxChunkSize:   300000, // 50s max
			wantCuts:       3,
			verifyCuts: func(t *testing.T, cuts []time.Duration) {
				// Cuts at last valid candidates: 30.5s, 60.5s, 90.5s
				assertEqual(t, cuts[0], 30500*time.Millisecond)
				assertEqual(t, cuts[1], 60500*time.Millisecond)
				assertEqual(t, cuts[2], 90500*time.Millisecond)
			},
		},
		{
			name: "multiple_cuts_evenly_spaced",
			silences: []silencePoint{
				{start: 25 * time.Second, end: 26 * time.Second},   // 25.5s < 30s, candidate
				{start: 50 * time.Second, end: 51 * time.Second},   // 50.5s >= 30s, CUT at 25.5s, then 50.5-25.5=25s < 30s, candidate
				{start: 75 * time.Second, end: 76 * time.Second},   // 75.5-25.5=50s >= 30s, CUT at 50.5s, then 75.5-50.5=25s < 30s, candidate
				{start: 100 * time.Second, end: 101 * time.Second}, // 100.5-50.5=50s >= 30s, CUT at 75.5s
			},
			bytesPerSecond: 6000,
			maxChunkSize:   180000, // 30s max
			wantCuts:       3,
			verifyCuts: func(t *testing.T, cuts []time.Duration) {
				assertEqual(t, cuts[0], 25500*time.Millisecond)
				assertEqual(t, cuts[1], 50500*time.Millisecond)
				assertEqual(t, cuts[2], 75500*time.Millisecond)
			},
		},
		{
			name: "silence_at_exact_boundary_triggers_cut",
			silences: []silencePoint{
				{start: 50 * time.Second, end: 51 * time.Second}, // Midpoint 50.5s >= 50.5s, no candidate, must cut here
			},
			bytesPerSecond: 6000,
			maxChunkSize:   303000, // 50.5s exactly
			wantCuts:       1,
		},
		{
			name: "closely_spaced_silences_uses_last_valid",
			silences: []silencePoint{
				{start: 10 * time.Second, end: 11 * time.Second}, // 10.5s < 50s, candidate
				{start: 12 * time.Second, end: 13 * time.Second}, // 12.5s < 50s, candidate (overwrites)
				{start: 14 * time.Second, end: 15 * time.Second}, // 14.5s < 50s, candidate (overwrites)
				{start: 60 * time.Second, end: 61 * time.Second}, // 60.5s >= 50s, CUT at 14.5s
			},
			bytesPerSecond: 6000,
			maxChunkSize:   300000, // 50s max
			wantCuts:       1,
			verifyCuts: func(t *testing.T, cuts []time.Duration) {
				// Should cut at 14.5s (last valid silence before 60.5s exceeded limit)
				assertEqual(t, cuts[0], 14500*time.Millisecond)
			},
		},
		{
			name: "no_cuts_when_all_silences_under_limit",
			silences: []silencePoint{
				{start: 10 * time.Second, end: 11 * time.Second},
				{start: 20 * time.Second, end: 21 * time.Second},
				{start: 30 * time.Second, end: 31 * time.Second},
			},
			bytesPerSecond: 6000,
			maxChunkSize:   600000, // 100s max - all silences under
			wantCuts:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sc := newTestChunker(tt.maxChunkSize)
			cuts := sc.selectCutPoints(tt.silences, tt.bytesPerSecond)

			assertEqual(t, len(cuts), tt.wantCuts)

			if tt.verifyCuts != nil && len(cuts) > 0 {
				tt.verifyCuts(t, cuts)
			}
		})
	}
}

func TestSilencePoint_Midpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		start time.Duration
		end   time.Duration
		want  time.Duration
	}{
		{"simple", 10 * time.Second, 12 * time.Second, 11 * time.Second},
		{"odd_duration", 10 * time.Second, 11 * time.Second, 10500 * time.Millisecond},
		{"zero_start", 0, 2 * time.Second, 1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sp := silencePoint{start: tt.start, end: tt.end}
			got := sp.midpoint()
			assertEqual(t, got, tt.want)
		})
	}
}

// =============================================================================
// Groupe E: Tests avec mock FFmpeg (nécessite contrôle strict du mock)
// =============================================================================

// requireFFmpegMock installs a mock and ensures cleanup.
// Returns the mock for call verification.
func requireFFmpegMock(t *testing.T, output string, err error) *mockFFmpegRunner {
	t.Helper()
	var mock *mockFFmpegRunner
	if err != nil {
		mock = withFFmpegError(err)
	} else {
		mock = withFFmpegOutput(output)
	}
	t.Cleanup(installFFmpegMock(t, mock))
	return mock
}

func TestTimeChunker_Chunk_ProbesDuration(t *testing.T) {
	// No t.Parallel() - uses global mock
	output := "Duration: 00:05:00.00, start: 0.000000, bitrate: 48 kb/s"
	mock := requireFFmpegMock(t, output, nil)

	tc, err := NewTimeChunker("/usr/bin/ffmpeg", 10*time.Minute, 30*time.Second)
	assertNoError(t, err)

	audioPath := tempAudioFile(t)

	// This will fail at extractChunk (exec.Command) but we're testing probeDuration
	_, _ = tc.Chunk(context.Background(), audioPath)

	// Verify FFmpeg was called for probe
	if mock.CallCount() < 1 {
		t.Error("expected FFmpeg to be called for duration probe")
	}
}

func TestTimeChunker_Chunk_ContextCancellation(t *testing.T) {
	// No t.Parallel() - uses global mock
	output := "Duration: 00:05:00.00"
	requireFFmpegMock(t, output, nil)

	tc, err := NewTimeChunker("/usr/bin/ffmpeg", 1*time.Minute, 10*time.Second)
	assertNoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	audioPath := tempAudioFile(t)
	_, err = tc.Chunk(ctx, audioPath)

	// Should fail due to cancelled context
	if err == nil {
		t.Error("expected error due to cancelled context")
	}
}

func TestSilenceChunker_Chunk_ParsesSilences(t *testing.T) {
	// No t.Parallel() - uses global mock
	output := loadFixture(t, "silence_detect_output.txt")
	mock := requireFFmpegMock(t, output, nil)

	sc, err := NewSilenceChunker("/usr/bin/ffmpeg")
	assertNoError(t, err)

	audioPath := tempAudioFile(t)

	// Will fail at extractChunk but silences should be parsed
	_, _ = sc.Chunk(context.Background(), audioPath)

	// Verify FFmpeg was called for silence detection
	if mock.CallCount() < 1 {
		t.Error("expected FFmpeg to be called for silence detection")
	}

	// Verify args contain silencedetect
	calls := mock.calls
	if len(calls) > 0 {
		args := strings.Join(calls[0], " ")
		if !strings.Contains(args, "silencedetect") {
			t.Errorf("expected silencedetect in args, got: %s", args)
		}
	}
}

func TestSilenceChunker_Chunk_FallbackOnNoSilence(t *testing.T) {
	// No t.Parallel() - uses global mock
	output := "Duration: 00:01:30.00" // No silences
	requireFFmpegMock(t, output, nil)

	fallbackCalled := false
	mockFallback := &mockChunker{
		chunks: []Chunk{{Path: "/tmp/chunk.ogg", Index: 0}},
	}

	sc, err := NewSilenceChunker("/usr/bin/ffmpeg", WithFallback(&trackingChunker{
		inner: mockFallback,
		onChunk: func() {
			fallbackCalled = true
		},
	}))
	assertNoError(t, err)

	audioPath := tempAudioFile(t)
	chunks, err := sc.Chunk(context.Background(), audioPath)

	assertNoError(t, err)
	if !fallbackCalled {
		t.Error("expected fallback to be called when no silences detected")
	}
	assertEqual(t, len(chunks), 1)
}

// trackingChunker wraps a Chunker to track when it's called.
type trackingChunker struct {
	inner   Chunker
	onChunk func()
}

func (tc *trackingChunker) Chunk(ctx context.Context, audioPath string) ([]Chunk, error) {
	if tc.onChunk != nil {
		tc.onChunk()
	}
	return tc.inner.Chunk(ctx, audioPath)
}

func TestSilenceChunker_Chunk_FileNotFound(t *testing.T) {
	t.Parallel()

	sc, err := NewSilenceChunker("/usr/bin/ffmpeg")
	assertNoError(t, err)

	_, err = sc.Chunk(context.Background(), "/nonexistent/file.ogg")
	assertError(t, err, ErrFileNotFound)
}

// =============================================================================
// Groupe F: Tests Chunk struct et CleanupChunks
// =============================================================================

func TestChunk_Duration(t *testing.T) {
	t.Parallel()

	c := Chunk{
		StartTime: 10 * time.Second,
		EndTime:   30 * time.Second,
	}
	assertEqual(t, c.Duration(), 20*time.Second)
}

func TestChunk_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		chunk Chunk
		want  string
	}{
		{
			name:  "minutes_only",
			chunk: Chunk{Index: 0, StartTime: 0, EndTime: 5 * time.Minute},
			want:  "chunk 0: 00:00-05:00",
		},
		{
			name:  "with_hours",
			chunk: Chunk{Index: 1, StartTime: 1 * time.Hour, EndTime: 1*time.Hour + 30*time.Minute},
			want:  "chunk 1: 01:00:00-01:30:00",
		},
		{
			name:  "mixed_format",
			chunk: Chunk{Index: 2, StartTime: 45 * time.Minute, EndTime: 1*time.Hour + 15*time.Minute},
			want:  "chunk 2: 45:00-01:15:00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.chunk.String()
			assertEqual(t, got, tt.want)
		})
	}
}

func TestCleanupChunks_Empty(t *testing.T) {
	t.Parallel()

	err := CleanupChunks(nil)
	assertNoError(t, err)

	err = CleanupChunks([]Chunk{})
	assertNoError(t, err)
}

func TestCleanupChunks_ValidTempDir(t *testing.T) {
	t.Parallel()

	// Create temp dir with expected pattern
	tempDir, err := os.MkdirTemp("", "go-transcript-test-*")
	assertNoError(t, err)

	// Create chunk files
	chunk1 := filepath.Join(tempDir, "chunk_000.ogg")
	chunk2 := filepath.Join(tempDir, "chunk_001.ogg")
	assertNoError(t, os.WriteFile(chunk1, []byte("test"), 0o644))
	assertNoError(t, os.WriteFile(chunk2, []byte("test"), 0o644))

	chunks := []Chunk{
		{Path: chunk1, Index: 0},
		{Path: chunk2, Index: 1},
	}

	err = CleanupChunks(chunks)
	assertNoError(t, err)

	// Verify directory was removed
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Error("expected temp directory to be removed")
		os.RemoveAll(tempDir) // Cleanup for test
	}
}

func TestCleanupChunks_NonTempDir(t *testing.T) {
	t.Parallel()

	// Create dir WITHOUT the expected pattern (safety test)
	tempDir := t.TempDir() // Uses test's temp dir, not "go-transcript-*"

	// Create chunk files
	chunk1 := filepath.Join(tempDir, "chunk_000.ogg")
	assertNoError(t, os.WriteFile(chunk1, []byte("test"), 0o644))

	chunks := []Chunk{
		{Path: chunk1, Index: 0},
	}

	err := CleanupChunks(chunks)
	assertNoError(t, err)

	// Verify: file should be removed but directory should still exist
	// (safety measure - don't delete arbitrary directories)
	if _, err := os.Stat(chunk1); !os.IsNotExist(err) {
		t.Error("expected chunk file to be removed")
	}
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Error("directory should NOT be removed (safety check)")
	}
}
