package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// =============================================================================
// Fixture Helpers
// =============================================================================

// avfoundationDeviceList generates macOS AVFoundation device list output.
// audioDevices is a list of audio device names (e.g., "MacBook Pro Microphone", "BlackHole 2ch").
func avfoundationDeviceList(audioDevices ...string) string {
	var b strings.Builder
	b.WriteString("[AVFoundation indev @ 0x7f8e4c004a00] AVFoundation video devices:\n")
	b.WriteString("[AVFoundation indev @ 0x7f8e4c004a00] [0] FaceTime HD Camera\n")
	b.WriteString("[AVFoundation indev @ 0x7f8e4c004a00] [1] Capture screen 0\n")
	b.WriteString("[AVFoundation indev @ 0x7f8e4c004a00] AVFoundation audio devices:\n")
	for i, name := range audioDevices {
		b.WriteString("[AVFoundation indev @ 0x7f8e4c004a00] [")
		b.WriteString(itoa(i))
		b.WriteString("] ")
		b.WriteString(name)
		b.WriteString("\n")
	}
	return b.String()
}

// dshowDeviceList generates Windows DirectShow device list output.
// audioDevices is a list of audio device names (e.g., "Stereo Mix (Realtek)", "CABLE Output").
func dshowDeviceList(audioDevices ...string) string {
	var b strings.Builder
	b.WriteString("[dshow @ 0x0000020c] DirectShow video devices (some may be both video and audio devices)\n")
	b.WriteString("[dshow @ 0x0000020c]  \"Integrated Webcam\"\n")
	b.WriteString("[dshow @ 0x0000020c] DirectShow audio devices\n")
	for _, name := range audioDevices {
		b.WriteString("[dshow @ 0x0000020c]  \"")
		b.WriteString(name)
		b.WriteString("\"\n")
	}
	return b.String()
}

// =============================================================================
// CaptureMode Tests
// =============================================================================

func TestCaptureMode_Values(t *testing.T) {
	t.Parallel()

	// Verify enum values are distinct and ordered as expected
	if CaptureMicrophone != 0 {
		t.Errorf("expected CaptureMicrophone to be 0, got %d", CaptureMicrophone)
	}
	if CaptureLoopback != 1 {
		t.Errorf("expected CaptureLoopback to be 1, got %d", CaptureLoopback)
	}
	if CaptureMix != 2 {
		t.Errorf("expected CaptureMix to be 2, got %d", CaptureMix)
	}
}

func TestCaptureMode_Distinct(t *testing.T) {
	t.Parallel()

	modes := []CaptureMode{CaptureMicrophone, CaptureLoopback, CaptureMix}
	seen := make(map[CaptureMode]bool)
	for _, m := range modes {
		if seen[m] {
			t.Errorf("duplicate CaptureMode value: %d", m)
		}
		seen[m] = true
	}
}

// =============================================================================
// loopbackError Tests
// =============================================================================

func TestLoopbackError_Unwrap(t *testing.T) {
	t.Parallel()

	err := &loopbackError{
		wrapped: ErrLoopbackNotFound,
		help:    "Install BlackHole",
	}

	if !errors.Is(err, ErrLoopbackNotFound) {
		t.Error("expected errors.Is(err, ErrLoopbackNotFound) to be true")
	}
}

func TestLoopbackError_ErrorMessage(t *testing.T) {
	t.Parallel()

	err := &loopbackError{
		wrapped: ErrLoopbackNotFound,
		help:    "Install BlackHole with: brew install --cask blackhole-2ch",
	}

	msg := err.Error()

	// Should contain the wrapped error message
	if !strings.Contains(msg, "loopback device not found") {
		t.Errorf("expected message to contain wrapped error, got %q", msg)
	}

	// Should contain the help text
	if !strings.Contains(msg, "Install BlackHole") {
		t.Errorf("expected message to contain help text, got %q", msg)
	}
}

func TestLoopbackError_MessageFormat(t *testing.T) {
	t.Parallel()

	err := &loopbackError{
		wrapped: ErrLoopbackNotFound,
		help:    "Help text here",
	}

	msg := err.Error()

	// Should have double newline separator between error and help
	if !strings.Contains(msg, "\n\n") {
		t.Errorf("expected double newline separator in message, got %q", msg)
	}
}

// =============================================================================
// extractDShowDeviceName Tests (Pure Function)
// =============================================================================

func TestExtractDShowDeviceName_FullMatch(t *testing.T) {
	t.Parallel()

	stderr := `[dshow @ 0x0000020c] DirectShow audio devices
[dshow @ 0x0000020c]  "Stereo Mix (Realtek High Definition Audio)"
[dshow @ 0x0000020c]  "Microphone (Realtek High Definition Audio)"
`
	result := extractDShowDeviceName(stderr, "Stereo Mix")

	expected := "Stereo Mix (Realtek High Definition Audio)"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestExtractDShowDeviceName_MultipleLines(t *testing.T) {
	t.Parallel()

	stderr := `[dshow @ 0x0] "Line 1"
[dshow @ 0x0] "Target Device Name"
[dshow @ 0x0] "Line 3"
`
	result := extractDShowDeviceName(stderr, "Target")

	if result != "Target Device Name" {
		t.Errorf("got %q, want %q", result, "Target Device Name")
	}
}

func TestExtractDShowDeviceName_NoMatch(t *testing.T) {
	t.Parallel()

	stderr := `[dshow @ 0x0] "Device A"
[dshow @ 0x0] "Device B"
`
	result := extractDShowDeviceName(stderr, "Nonexistent")

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestExtractDShowDeviceName_NoQuotes(t *testing.T) {
	t.Parallel()

	stderr := `[dshow @ 0x0] Stereo Mix without quotes
`
	result := extractDShowDeviceName(stderr, "Stereo Mix")

	if result != "" {
		t.Errorf("expected empty string when no quotes, got %q", result)
	}
}

func TestExtractDShowDeviceName_UnclosedQuote(t *testing.T) {
	t.Parallel()

	stderr := `[dshow @ 0x0] "Stereo Mix (unclosed
`
	result := extractDShowDeviceName(stderr, "Stereo Mix")

	if result != "" {
		t.Errorf("expected empty string for unclosed quote, got %q", result)
	}
}

func TestExtractDShowDeviceName_EmptyInput(t *testing.T) {
	t.Parallel()

	result := extractDShowDeviceName("", "anything")

	if result != "" {
		t.Errorf("expected empty string for empty input, got %q", result)
	}
}

func TestExtractDShowDeviceName_PartialNameInMiddle(t *testing.T) {
	t.Parallel()

	stderr := `[dshow @ 0x0] "VB-Audio Virtual Cable Output"
`
	result := extractDShowDeviceName(stderr, "CABLE Output")

	// Should NOT match because "CABLE Output" is not in "VB-Audio Virtual Cable Output"
	// Actually, it should match because "CABLE Output" IS a substring... let me check
	// "VB-Audio Virtual Cable Output" does NOT contain "CABLE Output" (case matters, and space differs)
	if result != "" {
		t.Errorf("expected no match, got %q", result)
	}
}

// =============================================================================
// detectLoopbackDarwin Tests (Mock FFmpeg)
// =============================================================================

func TestDetectLoopbackDarwin_BlackHole2ch(t *testing.T) {
	// Not parallel - modifies global runFFmpegOutputFunc
	mock := withFFmpegOutput(avfoundationDeviceList("MacBook Pro Microphone", "BlackHole 2ch"))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackDarwin(context.Background(), "/usr/bin/ffmpeg")

	assertNoError(t, err)
	assertEqual(t, device.name, ":BlackHole 2ch")
	assertEqual(t, device.format, "avfoundation")
}

func TestDetectLoopbackDarwin_BlackHole16ch(t *testing.T) {
	mock := withFFmpegOutput(avfoundationDeviceList("Microphone", "BlackHole 16ch"))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackDarwin(context.Background(), "/usr/bin/ffmpeg")

	assertNoError(t, err)
	assertEqual(t, device.name, ":BlackHole 16ch")
	assertEqual(t, device.format, "avfoundation")
}

func TestDetectLoopbackDarwin_BlackHole64ch(t *testing.T) {
	mock := withFFmpegOutput(avfoundationDeviceList("Microphone", "BlackHole 64ch"))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackDarwin(context.Background(), "/usr/bin/ffmpeg")

	assertNoError(t, err)
	assertEqual(t, device.name, ":BlackHole 64ch")
	assertEqual(t, device.format, "avfoundation")
}

func TestDetectLoopbackDarwin_Priority_2chFirst(t *testing.T) {
	// When multiple BlackHole versions are installed, 2ch should be preferred
	mock := withFFmpegOutput(avfoundationDeviceList("Microphone", "BlackHole 2ch", "BlackHole 16ch", "BlackHole 64ch"))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackDarwin(context.Background(), "/usr/bin/ffmpeg")

	assertNoError(t, err)
	assertEqual(t, device.name, ":BlackHole 2ch")
}

func TestDetectLoopbackDarwin_NotFound(t *testing.T) {
	mock := withFFmpegOutput(avfoundationDeviceList("MacBook Pro Microphone", "External Microphone"))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackDarwin(context.Background(), "/usr/bin/ffmpeg")

	if device != nil {
		t.Errorf("expected nil device, got %+v", device)
	}
	assertError(t, err, ErrLoopbackNotFound)
}

func TestDetectLoopbackDarwin_EmptyOutput(t *testing.T) {
	mock := withFFmpegOutput("")
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackDarwin(context.Background(), "/usr/bin/ffmpeg")

	if device != nil {
		t.Errorf("expected nil device, got %+v", device)
	}
	assertError(t, err, ErrLoopbackNotFound)
}

func TestDetectLoopbackDarwin_ErrorContainsInstallInstructions(t *testing.T) {
	mock := withFFmpegOutput(avfoundationDeviceList("Microphone"))
	t.Cleanup(installFFmpegMock(t, mock))

	_, err := detectLoopbackDarwin(context.Background(), "/usr/bin/ffmpeg")

	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	assertContains(t, msg, "brew install")
	assertContains(t, msg, "blackhole")
}

// =============================================================================
// detectLoopbackLinux Tests (Mock Exec)
// =============================================================================

func TestDetectLoopbackLinux_PulseAudio_Success(t *testing.T) {
	mock := newMockExecRunner().
		OnCommand("pactl", []byte("alsa_output.pci-0000_00_1f.3.analog-stereo\n"), nil)
	t.Cleanup(installExecMock(t, mock))

	device, err := detectLoopbackLinux(context.Background())

	assertNoError(t, err)
	assertEqual(t, device.name, "alsa_output.pci-0000_00_1f.3.analog-stereo.monitor")
	assertEqual(t, device.format, "pulse")
}

func TestDetectLoopbackLinux_MonitorSuffix(t *testing.T) {
	mock := newMockExecRunner().
		OnCommand("pactl", []byte("my-sink\n"), nil)
	t.Cleanup(installExecMock(t, mock))

	device, err := detectLoopbackLinux(context.Background())

	assertNoError(t, err)
	// Verify .monitor is appended
	if !strings.HasSuffix(device.name, ".monitor") {
		t.Errorf("expected name to end with .monitor, got %q", device.name)
	}
	assertEqual(t, device.name, "my-sink.monitor")
}

func TestDetectLoopbackLinux_PulseFormat(t *testing.T) {
	mock := newMockExecRunner().
		OnCommand("pactl", []byte("sink-name\n"), nil)
	t.Cleanup(installExecMock(t, mock))

	device, err := detectLoopbackLinux(context.Background())

	assertNoError(t, err)
	assertEqual(t, device.format, "pulse")
}

func TestDetectLoopbackLinux_TrimsWhitespace(t *testing.T) {
	mock := newMockExecRunner().
		OnCommand("pactl", []byte("  sink-name  \n\n"), nil)
	t.Cleanup(installExecMock(t, mock))

	device, err := detectLoopbackLinux(context.Background())

	assertNoError(t, err)
	assertEqual(t, device.name, "sink-name.monitor")
}

func TestDetectLoopbackLinux_NoPactl_HasPipeWire(t *testing.T) {
	execErr := errors.New("command not found")
	mock := newMockExecRunner().
		OnCommand("pactl", nil, execErr).
		OnCommand("pw-cli", []byte("pipewire info\n"), nil)
	t.Cleanup(installExecMock(t, mock))

	device, err := detectLoopbackLinux(context.Background())

	if device != nil {
		t.Errorf("expected nil device, got %+v", device)
	}
	assertError(t, err, ErrLoopbackNotFound)
	// Should suggest installing pactl
	assertContains(t, err.Error(), "pactl not found")
	assertContains(t, err.Error(), "pulseaudio-utils")
}

func TestDetectLoopbackLinux_NoPactl_NoPipeWire(t *testing.T) {
	execErr := errors.New("command not found")
	mock := newMockExecRunner().
		OnCommand("pactl", nil, execErr).
		OnCommand("pw-cli", nil, execErr)
	t.Cleanup(installExecMock(t, mock))

	device, err := detectLoopbackLinux(context.Background())

	if device != nil {
		t.Errorf("expected nil device, got %+v", device)
	}
	assertError(t, err, ErrLoopbackNotFound)
}

func TestDetectLoopbackLinux_EmptySinkName(t *testing.T) {
	mock := newMockExecRunner().
		OnCommand("pactl", []byte("   \n"), nil)
	t.Cleanup(installExecMock(t, mock))

	device, err := detectLoopbackLinux(context.Background())

	if device != nil {
		t.Errorf("expected nil device, got %+v", device)
	}
	assertError(t, err, ErrLoopbackNotFound)
}

func TestDetectLoopbackLinux_ErrorContainsInstallInstructions(t *testing.T) {
	execErr := errors.New("command not found")
	mock := newMockExecRunner().
		OnCommand("pactl", nil, execErr).
		OnCommand("pw-cli", nil, execErr)
	t.Cleanup(installExecMock(t, mock))

	_, err := detectLoopbackLinux(context.Background())

	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	assertContains(t, msg, "PulseAudio")
	assertContains(t, msg, "apt install")
}

// =============================================================================
// detectLoopbackWindows Tests (Mock FFmpeg)
// =============================================================================

func TestDetectLoopbackWindows_StereoMix(t *testing.T) {
	mock := withFFmpegOutput(dshowDeviceList("Microphone", "Stereo Mix (Realtek High Definition Audio)"))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackWindows(context.Background(), "ffmpeg.exe")

	assertNoError(t, err)
	assertEqual(t, device.name, "audio=Stereo Mix (Realtek High Definition Audio)")
	assertEqual(t, device.format, "dshow")
}

func TestDetectLoopbackWindows_WaveOutMix(t *testing.T) {
	mock := withFFmpegOutput(dshowDeviceList("Microphone", "Wave Out Mix (Some Driver)"))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackWindows(context.Background(), "ffmpeg.exe")

	assertNoError(t, err)
	assertEqual(t, device.name, "audio=Wave Out Mix (Some Driver)")
}

func TestDetectLoopbackWindows_WhatUHear(t *testing.T) {
	mock := withFFmpegOutput(dshowDeviceList("Microphone", "What U Hear (Sound Blaster)"))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackWindows(context.Background(), "ffmpeg.exe")

	assertNoError(t, err)
	assertEqual(t, device.name, "audio=What U Hear (Sound Blaster)")
}

func TestDetectLoopbackWindows_LoQueEscucha(t *testing.T) {
	// Spanish locale variant
	mock := withFFmpegOutput(dshowDeviceList("Microfono", "Lo que escucha (Realtek)"))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackWindows(context.Background(), "ffmpeg.exe")

	assertNoError(t, err)
	assertEqual(t, device.name, "audio=Lo que escucha (Realtek)")
}

func TestDetectLoopbackWindows_VBCable_CABLEOutput(t *testing.T) {
	mock := withFFmpegOutput(dshowDeviceList("Microphone", "CABLE Output (VB-Audio Virtual Cable)"))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackWindows(context.Background(), "ffmpeg.exe")

	assertNoError(t, err)
	assertEqual(t, device.name, "audio=CABLE Output (VB-Audio Virtual Cable)")
}

func TestDetectLoopbackWindows_VBCable_VBAudioVirtualCable(t *testing.T) {
	mock := withFFmpegOutput(dshowDeviceList("Microphone", "VB-Audio Virtual Cable"))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackWindows(context.Background(), "ffmpeg.exe")

	assertNoError(t, err)
	assertEqual(t, device.name, "audio=VB-Audio Virtual Cable")
}

func TestDetectLoopbackWindows_VirtualAudioCapturer(t *testing.T) {
	mock := withFFmpegOutput(dshowDeviceList("Microphone", "virtual-audio-capturer"))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackWindows(context.Background(), "ffmpeg.exe")

	assertNoError(t, err)
	assertEqual(t, device.name, "audio=virtual-audio-capturer")
}

func TestDetectLoopbackWindows_Priority_StereoMixOverVBCable(t *testing.T) {
	// Stereo Mix (priority 1) should be preferred over VB-Cable (priority 2)
	mock := withFFmpegOutput(dshowDeviceList(
		"Microphone",
		"CABLE Output (VB-Audio Virtual Cable)",
		"Stereo Mix (Realtek High Definition Audio)",
	))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackWindows(context.Background(), "ffmpeg.exe")

	assertNoError(t, err)
	// Should pick Stereo Mix, not CABLE Output
	assertContains(t, device.name, "Stereo Mix")
}

func TestDetectLoopbackWindows_Priority_VBCableOverVirtualAudioCapturer(t *testing.T) {
	// VB-Cable (priority 2) should be preferred over virtual-audio-capturer (priority 3)
	mock := withFFmpegOutput(dshowDeviceList(
		"Microphone",
		"virtual-audio-capturer",
		"CABLE Output (VB-Audio Virtual Cable)",
	))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackWindows(context.Background(), "ffmpeg.exe")

	assertNoError(t, err)
	assertContains(t, device.name, "CABLE Output")
}

func TestDetectLoopbackWindows_NotFound(t *testing.T) {
	mock := withFFmpegOutput(dshowDeviceList("Microphone", "Webcam Audio"))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackWindows(context.Background(), "ffmpeg.exe")

	if device != nil {
		t.Errorf("expected nil device, got %+v", device)
	}
	assertError(t, err, ErrLoopbackNotFound)
}

func TestDetectLoopbackWindows_EmptyOutput(t *testing.T) {
	mock := withFFmpegOutput("")
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackWindows(context.Background(), "ffmpeg.exe")

	if device != nil {
		t.Errorf("expected nil device, got %+v", device)
	}
	assertError(t, err, ErrLoopbackNotFound)
}

func TestDetectLoopbackWindows_DShowFormat(t *testing.T) {
	mock := withFFmpegOutput(dshowDeviceList("Stereo Mix"))
	t.Cleanup(installFFmpegMock(t, mock))

	device, err := detectLoopbackWindows(context.Background(), "ffmpeg.exe")

	assertNoError(t, err)
	assertEqual(t, device.format, "dshow")
}

func TestDetectLoopbackWindows_ErrorContainsInstallInstructions(t *testing.T) {
	mock := withFFmpegOutput(dshowDeviceList("Microphone"))
	t.Cleanup(installFFmpegMock(t, mock))

	_, err := detectLoopbackWindows(context.Background(), "ffmpeg.exe")

	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	assertContains(t, msg, "Stereo Mix")
	assertContains(t, msg, "VB-Audio")
}

// =============================================================================
// Installation Instructions Tests
// =============================================================================

func TestLoopbackInstallInstructionsDarwin_ContainsBrewCommand(t *testing.T) {
	t.Parallel()

	instructions := loopbackInstallInstructionsDarwin()

	assertContains(t, instructions, "brew install --cask blackhole")
}

func TestLoopbackInstallInstructionsDarwin_ContainsMultiOutputDevice(t *testing.T) {
	t.Parallel()

	instructions := loopbackInstallInstructionsDarwin()

	assertContains(t, instructions, "Multi-Output Device")
}

func TestLoopbackInstallInstructionsDarwin_ContainsGitHubLink(t *testing.T) {
	t.Parallel()

	instructions := loopbackInstallInstructionsDarwin()

	assertContains(t, instructions, "github.com/ExistentialAudio/BlackHole")
}

func TestLoopbackInstallInstructionsLinux_ContainsPulseAudio(t *testing.T) {
	t.Parallel()

	instructions := loopbackInstallInstructionsLinux()

	assertContains(t, instructions, "PulseAudio")
}

func TestLoopbackInstallInstructionsLinux_ContainsPactl(t *testing.T) {
	t.Parallel()

	instructions := loopbackInstallInstructionsLinux()

	assertContains(t, instructions, "pactl")
}

func TestLoopbackInstallInstructionsLinux_ContainsDistroCommands(t *testing.T) {
	t.Parallel()

	instructions := loopbackInstallInstructionsLinux()

	assertContains(t, instructions, "apt install")
	assertContains(t, instructions, "dnf install")
	assertContains(t, instructions, "pacman")
}

func TestLoopbackInstallInstructionsWindows_ContainsStereoMix(t *testing.T) {
	t.Parallel()

	instructions := loopbackInstallInstructionsWindows()

	assertContains(t, instructions, "Stereo Mix")
}

func TestLoopbackInstallInstructionsWindows_ContainsVBCableURL(t *testing.T) {
	t.Parallel()

	instructions := loopbackInstallInstructionsWindows()

	assertContains(t, instructions, "vb-audio.com/Cable")
}

func TestLoopbackInstallInstructionsWindows_ContainsVoiceMeeterWarning(t *testing.T) {
	t.Parallel()

	instructions := loopbackInstallInstructionsWindows()

	// Should warn that VB-Cable doesn't relay audio
	assertContains(t, instructions, "VoiceMeeter")
}

// =============================================================================
// Coverage Note
// =============================================================================
//
// The detectLoopbackDevice function (dispatcher) is NOT unit-tested here.
// It's a trivial switch on runtime.GOOS that delegates to the OS-specific
// functions tested above. Testing the dispatcher would require:
// - Build tags (//go:build darwin) limiting tests to specific OS
// - OS injection refactoring (unnecessary complexity for a 6-line switch)
//
// The dispatcher is covered by:
// 1. Integration tests (Phase G) that run on real systems
// 2. Transitive coverage: if OS-specific functions work, the dispatcher works
//
// Decision documented in SWOT analysis: Question 2, Option B selected.
