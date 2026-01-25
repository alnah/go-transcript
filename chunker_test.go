package main

import (
	"testing"
	"time"
)

func TestParseSilenceOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected []silencePoint
	}{
		{
			name:     "empty output",
			output:   "",
			expected: nil,
		},
		{
			name:     "no silences",
			output:   "some ffmpeg output without silence info",
			expected: nil,
		},
		{
			name: "single silence",
			output: `[silencedetect @ 0x7f8b8c004a00] silence_start: 10.5
[silencedetect @ 0x7f8b8c004a00] silence_end: 11.2 | silence_duration: 0.7`,
			expected: []silencePoint{
				{start: 10500 * time.Millisecond, end: 11200 * time.Millisecond},
			},
		},
		{
			name: "multiple silences",
			output: `frame=  100 fps=0.0 q=-0.0 size=N/A time=00:00:05.00 bitrate=N/A speed=N/A
[silencedetect @ 0x7f8b8c004a00] silence_start: 5.123
[silencedetect @ 0x7f8b8c004a00] silence_end: 6.456 | silence_duration: 1.333
frame=  200 fps=0.0 q=-0.0 size=N/A time=00:00:10.00 bitrate=N/A speed=N/A
[silencedetect @ 0x7f8b8c004a00] silence_start: 42.0
[silencedetect @ 0x7f8b8c004a00] silence_end: 43.5 | silence_duration: 1.5
[silencedetect @ 0x7f8b8c004a00] silence_start: 100.25
[silencedetect @ 0x7f8b8c004a00] silence_end: 101.75 | silence_duration: 1.5`,
			expected: []silencePoint{
				{start: 5123 * time.Millisecond, end: 6456 * time.Millisecond},
				{start: 42 * time.Second, end: 43500 * time.Millisecond},
				{start: 100250 * time.Millisecond, end: 101750 * time.Millisecond},
			},
		},
		{
			name: "silence start without end (incomplete)",
			output: `[silencedetect @ 0x7f8b8c004a00] silence_start: 10.0
[silencedetect @ 0x7f8b8c004a00] silence_start: 20.0`,
			expected: nil,
		},
		{
			name: "real ffmpeg output format",
			output: `ffmpeg version 6.1.1 Copyright (c) 2000-2023 the FFmpeg developers
Input #0, ogg, from 'test.ogg':
  Duration: 00:05:30.00, start: 0.000000, bitrate: 48 kb/s
[silencedetect @ 0x12345] silence_start: 0
[silencedetect @ 0x12345] silence_end: 0.5 | silence_duration: 0.5
[silencedetect @ 0x12345] silence_start: 120.123456
[silencedetect @ 0x12345] silence_end: 121.654321 | silence_duration: 1.530865
size=N/A time=00:05:30.00 bitrate=N/A speed=1000x`,
			expected: []silencePoint{
				{start: 0, end: 500 * time.Millisecond},
				{start: 120123 * time.Millisecond, end: 121654 * time.Millisecond},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSilenceOutput(tt.output)
			if len(got) != len(tt.expected) {
				t.Fatalf("parseSilenceOutput() returned %d silences, want %d", len(got), len(tt.expected))
			}
			for i, s := range got {
				// Allow 1ms tolerance for floating point conversion
				if abs(s.start-tt.expected[i].start) > time.Millisecond {
					t.Errorf("silence[%d].start = %v, want %v", i, s.start, tt.expected[i].start)
				}
				if abs(s.end-tt.expected[i].end) > time.Millisecond {
					t.Errorf("silence[%d].end = %v, want %v", i, s.end, tt.expected[i].end)
				}
			}
		})
	}
}

func TestParseDurationFromFFmpegOutput(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		expected    time.Duration
		expectError bool
	}{
		{
			name:        "empty output",
			output:      "",
			expectError: true,
		},
		{
			name:     "duration format standard",
			output:   "  Duration: 01:23:45.67, start: 0.000000, bitrate: 48 kb/s",
			expected: 1*time.Hour + 23*time.Minute + 45*time.Second + 670*time.Millisecond,
		},
		{
			name:     "duration format short",
			output:   "Duration: 00:05:30.00",
			expected: 5*time.Minute + 30*time.Second,
		},
		{
			name:     "duration with milliseconds",
			output:   "Duration: 00:00:10.123",
			expected: 10*time.Second + 123*time.Millisecond,
		},
		{
			name: "time format from progress (fallback)",
			output: `frame=  100 fps=0.0 q=-0.0 size=N/A time=00:01:30.50 bitrate=N/A
frame=  200 fps=0.0 q=-0.0 size=N/A time=00:03:00.25 bitrate=N/A`,
			expected: 3*time.Minute + 250*time.Millisecond,
		},
		{
			name: "real ffmpeg output with both formats",
			output: `ffmpeg version 6.1.1 Copyright (c) 2000-2023 the FFmpeg developers
Input #0, ogg, from 'test.ogg':
  Duration: 00:02:30.50, start: 0.000000, bitrate: 48 kb/s
size=N/A time=00:02:30.50 bitrate=N/A speed=1000x`,
			expected: 2*time.Minute + 30*time.Second + 500*time.Millisecond,
		},
		{
			name:        "no duration info",
			output:      "some random output without duration",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDurationFromFFmpegOutput(tt.output)
			if tt.expectError {
				if err == nil {
					t.Errorf("parseDurationFromFFmpegOutput() expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDurationFromFFmpegOutput() unexpected error: %v", err)
			}
			// Allow 10ms tolerance for parsing differences
			if abs(got-tt.expected) > 10*time.Millisecond {
				t.Errorf("parseDurationFromFFmpegOutput() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseTimeComponents(t *testing.T) {
	tests := []struct {
		name                                  string
		hours, minutes, seconds, centiseconds string
		expected                              time.Duration
	}{
		{"zero", "00", "00", "00", "00", 0},
		{"one hour", "01", "00", "00", "00", time.Hour},
		{"complex", "02", "30", "45", "50", 2*time.Hour + 30*time.Minute + 45*time.Second + 500*time.Millisecond},
		{"milliseconds 3 digits", "00", "00", "10", "123", 10*time.Second + 123*time.Millisecond},
		{"centiseconds 2 digits", "00", "00", "10", "45", 10*time.Second + 450*time.Millisecond},
		{"deciseconds 1 digit", "00", "00", "10", "5", 10*time.Second + 500*time.Millisecond},
		{"microseconds truncated", "00", "00", "10", "123456", 10*time.Second + 123*time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTimeComponents(tt.hours, tt.minutes, tt.seconds, tt.centiseconds)
			if err != nil {
				t.Fatalf("parseTimeComponents() error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("parseTimeComponents() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFormatFFmpegTime(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"zero", 0, "00:00:00.000"},
		{"seconds only", 30 * time.Second, "00:00:30.000"},
		{"minutes and seconds", 5*time.Minute + 30*time.Second, "00:05:30.000"},
		{"hours", 2*time.Hour + 30*time.Minute + 45*time.Second, "02:30:45.000"},
		{"with milliseconds", 1*time.Minute + 30*time.Second + 500*time.Millisecond, "00:01:30.500"},
		{"precise milliseconds", 10*time.Second + 123*time.Millisecond, "00:00:10.123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFFmpegTime(tt.duration)
			if got != tt.expected {
				t.Errorf("formatFFmpegTime(%v) = %q, want %q", tt.duration, got, tt.expected)
			}
		})
	}
}

func TestSelectCutPoints(t *testing.T) {
	// Create a SilenceChunker with 20MB max chunk size
	sc := &SilenceChunker{
		maxChunkSize: 20 * 1024 * 1024, // 20MB
	}

	tests := []struct {
		name           string
		silences       []silencePoint
		bytesPerSecond float64
		expectedCuts   int
	}{
		{
			name:           "no silences",
			silences:       nil,
			bytesPerSecond: 6000, // ~48kbps
			expectedCuts:   0,
		},
		{
			name: "file fits in one chunk",
			silences: []silencePoint{
				{start: 60 * time.Second, end: 61 * time.Second},
				{start: 120 * time.Second, end: 121 * time.Second},
			},
			bytesPerSecond: 6000, // ~48kbps, 5min = 1.8MB, way under 20MB
			expectedCuts:   0,
		},
		{
			name: "needs one cut",
			silences: []silencePoint{
				{start: 30 * time.Minute, end: 30*time.Minute + time.Second},
				{start: 60 * time.Minute, end: 60*time.Minute + time.Second},
			},
			bytesPerSecond: 12000, // At 12KB/s, 20MB = ~28 minutes
			expectedCuts:   2,     // Both silences exceed 28min from previous cut
		},
		{
			name: "needs multiple cuts",
			silences: []silencePoint{
				{start: 10 * time.Minute, end: 10*time.Minute + time.Second},
				{start: 20 * time.Minute, end: 20*time.Minute + time.Second},
				{start: 30 * time.Minute, end: 30*time.Minute + time.Second},
				{start: 40 * time.Minute, end: 40*time.Minute + time.Second},
				{start: 50 * time.Minute, end: 50*time.Minute + time.Second},
				{start: 60 * time.Minute, end: 60*time.Minute + time.Second},
			},
			bytesPerSecond: 20000, // At 20KB/s, 20MB = ~17 minutes
			expectedCuts:   3,     // Cuts around 17min, 34min, 51min
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sc.selectCutPoints(tt.silences, tt.bytesPerSecond)
			if len(got) != tt.expectedCuts {
				t.Errorf("selectCutPoints() returned %d cuts, want %d", len(got), tt.expectedCuts)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{0, "00:00"},
		{30 * time.Second, "00:30"},
		{5*time.Minute + 30*time.Second, "05:30"},
		{59*time.Minute + 59*time.Second, "59:59"},
		{time.Hour, "01:00:00"},
		{2*time.Hour + 30*time.Minute + 45*time.Second, "02:30:45"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.expected)
			}
		})
	}
}

func TestChunk_Duration(t *testing.T) {
	c := Chunk{
		StartTime: 10 * time.Second,
		EndTime:   70 * time.Second,
	}
	if got := c.Duration(); got != 60*time.Second {
		t.Errorf("Chunk.Duration() = %v, want %v", got, 60*time.Second)
	}
}

func TestChunk_String(t *testing.T) {
	c := Chunk{
		Index:     0,
		StartTime: 0,
		EndTime:   5*time.Minute + 30*time.Second,
	}
	expected := "chunk 0: 00:00-05:30"
	if got := c.String(); got != expected {
		t.Errorf("Chunk.String() = %q, want %q", got, expected)
	}
}

func TestSilencePoint_Midpoint(t *testing.T) {
	s := silencePoint{
		start: 10 * time.Second,
		end:   12 * time.Second,
	}
	expected := 11 * time.Second
	if got := s.midpoint(); got != expected {
		t.Errorf("silencePoint.midpoint() = %v, want %v", got, expected)
	}
}

// abs returns the absolute value of a duration.
func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
