package main

import (
	"strings"
	"testing"
	"time"
)

func TestFormatDurationHuman(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{2 * time.Hour, "2h"},
		{30 * time.Minute, "30m"},
		{90 * time.Minute, "1h30m"},
		{45 * time.Second, "45s"},
		{2*time.Hour + 15*time.Minute, "2h15m"},
		{1 * time.Minute, "1m"},
		{1 * time.Hour, "1h"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDurationHuman(tt.input)
			if got != tt.want {
				t.Errorf("formatDurationHuman(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{500, "500 bytes"},
		{1024, "1 KB"},
		{1536, "1 KB"},
		{1024 * 1024, "1 MB"},
		{142 * 1024 * 1024, "142 MB"},
		{1024 * 1024 * 1024, "1024 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatSize(tt.input)
			if got != tt.want {
				t.Errorf("formatSize(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDefaultRecordingFilename(t *testing.T) {
	filename := defaultRecordingFilename()

	// Should start with "recording_"
	if !strings.HasPrefix(filename, "recording_") {
		t.Errorf("filename should start with 'recording_', got %q", filename)
	}

	// Should end with ".ogg"
	if !strings.HasSuffix(filename, ".ogg") {
		t.Errorf("filename should end with '.ogg', got %q", filename)
	}

	// Should have reasonable length: recording_YYYYMMDD_HHMMSS.ogg = 29 chars
	// recording_ (10) + YYYYMMDD (8) + _ (1) + HHMMSS (6) + .ogg (4) = 29
	if len(filename) != 29 {
		t.Errorf("filename length should be 29, got %d: %q", len(filename), filename)
	}
}

func TestRecordCmd_Flags(t *testing.T) {
	cmd := recordCmd()

	// Check required flag
	durationFlag := cmd.Flags().Lookup("duration")
	if durationFlag == nil {
		t.Fatal("duration flag not found")
	}

	// Check other flags exist
	flags := []string{"output", "device", "loopback", "mix"}
	for _, name := range flags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q not found", name)
		}
	}

	// Check short flags
	if cmd.Flags().ShorthandLookup("d") == nil {
		t.Error("short flag -d not found")
	}
	if cmd.Flags().ShorthandLookup("o") == nil {
		t.Error("short flag -o not found")
	}
}

func TestRecordCmd_MutuallyExclusiveFlags(t *testing.T) {
	cmd := recordCmd()

	// Set both loopback and mix - should fail
	cmd.SetArgs([]string{"-d", "1m", "--loopback", "--mix"})
	err := cmd.Execute()

	if err == nil {
		t.Error("expected error when both --loopback and --mix are set")
	}

	// Error message should mention the mutual exclusivity
	if err != nil && !strings.Contains(err.Error(), "loopback") {
		t.Errorf("error message should mention 'loopback', got: %v", err)
	}
}

func TestRecordCmd_RequiresDuration(t *testing.T) {
	cmd := recordCmd()

	// No duration flag - should fail
	cmd.SetArgs([]string{"-o", "test.ogg"})
	err := cmd.Execute()

	if err == nil {
		t.Error("expected error when --duration is not set")
	}

	if err != nil && !strings.Contains(err.Error(), "duration") {
		t.Errorf("error message should mention 'duration', got: %v", err)
	}
}

func TestRecordCmd_InvalidDuration(t *testing.T) {
	cmd := recordCmd()

	// Invalid duration format
	cmd.SetArgs([]string{"-d", "2hours"})
	err := cmd.Execute()

	if err == nil {
		t.Error("expected error for invalid duration format")
	}

	if err != nil && !strings.Contains(err.Error(), "invalid duration") {
		t.Errorf("error message should mention 'invalid duration', got: %v", err)
	}
}
