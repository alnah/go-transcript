package main

import (
	"errors"
	"strings"
	"testing"
)

func TestExtractDShowDeviceName(t *testing.T) {
	tests := []struct {
		name        string
		stderr      string
		partialName string
		want        string
	}{
		{
			name: "stereo mix with realtek",
			stderr: `[dshow @ 0x7ff] DirectShow video devices
[dshow @ 0x7ff]  "Integrated Camera"
[dshow @ 0x7ff] DirectShow audio devices
[dshow @ 0x7ff]  "Microphone (Realtek High Definition Audio)"
[dshow @ 0x7ff]  "Stereo Mix (Realtek High Definition Audio)"`,
			partialName: "Stereo Mix",
			want:        "Stereo Mix (Realtek High Definition Audio)",
		},
		{
			name: "virtual audio capturer",
			stderr: `[dshow @ 0x7ff] DirectShow audio devices
[dshow @ 0x7ff]  "virtual-audio-capturer"`,
			partialName: "virtual-audio-capturer",
			want:        "virtual-audio-capturer",
		},
		{
			name:        "not found",
			stderr:      `[dshow @ 0x7ff] DirectShow audio devices`,
			partialName: "Stereo Mix",
			want:        "",
		},
		{
			name: "wave out mix variant",
			stderr: `[dshow @ 0x7ff] DirectShow audio devices
[dshow @ 0x7ff]  "Wave Out Mix (SoundBlaster)"`,
			partialName: "Wave Out Mix",
			want:        "Wave Out Mix (SoundBlaster)",
		},
		{
			name: "vb-audio cable output",
			stderr: `[dshow @ 0x7ff] DirectShow audio devices
[dshow @ 0x7ff]  "CABLE Output (VB-Audio Virtual Cable)"`,
			partialName: "CABLE Output",
			want:        "CABLE Output (VB-Audio Virtual Cable)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDShowDeviceName(tt.stderr, tt.partialName)
			if got != tt.want {
				t.Errorf("extractDShowDeviceName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoopbackInstallInstructions(t *testing.T) {
	t.Run("darwin instructions mention BlackHole", func(t *testing.T) {
		instructions := loopbackInstallInstructionsDarwin()
		if !strings.Contains(instructions, "BlackHole") {
			t.Error("Darwin instructions should mention BlackHole")
		}
		if !strings.Contains(instructions, "brew install") {
			t.Error("Darwin instructions should include brew command")
		}
	})

	t.Run("linux instructions mention PulseAudio", func(t *testing.T) {
		instructions := loopbackInstallInstructionsLinux()
		if !strings.Contains(instructions, "PulseAudio") {
			t.Error("Linux instructions should mention PulseAudio")
		}
		if !strings.Contains(instructions, "pactl") {
			t.Error("Linux instructions should mention pactl")
		}
	})

	t.Run("windows instructions mention Stereo Mix and VB-Audio", func(t *testing.T) {
		instructions := loopbackInstallInstructionsWindows()
		if !strings.Contains(instructions, "Stereo Mix") {
			t.Error("Windows instructions should mention Stereo Mix")
		}
		if !strings.Contains(instructions, "VB-Audio") {
			t.Error("Windows instructions should mention VB-Audio Virtual Cable")
		}
	})
}

func TestLoopbackErrorUnwrap(t *testing.T) {
	err := &loopbackError{
		wrapped: ErrLoopbackNotFound,
		help:    "test instructions",
	}

	if !errors.Is(err, ErrLoopbackNotFound) {
		t.Error("loopbackError should unwrap to ErrLoopbackNotFound")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "loopback device not found") {
		t.Errorf("error message should contain sentinel: %s", errStr)
	}
	if !strings.Contains(errStr, "test instructions") {
		t.Errorf("error message should contain help: %s", errStr)
	}
}

func TestCaptureModeConstants(t *testing.T) {
	// Verify constants have distinct values
	if CaptureMicrophone == CaptureLoopback {
		t.Error("CaptureMicrophone and CaptureLoopback should be different")
	}
	if CaptureLoopback == CaptureMix {
		t.Error("CaptureLoopback and CaptureMix should be different")
	}
	if CaptureMicrophone == CaptureMix {
		t.Error("CaptureMicrophone and CaptureMix should be different")
	}
}
