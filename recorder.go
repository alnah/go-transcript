package main

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Recorder records audio from an input device to a file.
type Recorder interface {
	Record(ctx context.Context, duration time.Duration, output string) error
}

// deviceError wraps an error with actionable help text.
// Implements error and Unwrap for errors.Is() compatibility.
type deviceError struct {
	wrapped error
	help    string
}

func (e *deviceError) Error() string {
	return fmt.Sprintf("%v: %s", e.wrapped, e.help)
}

func (e *deviceError) Unwrap() error {
	return e.wrapped
}

// FFmpegRecorder records audio using FFmpeg.
// It supports macOS (avfoundation), Linux (alsa/pulse), and Windows (dshow).
type FFmpegRecorder struct {
	ffmpegPath  string
	device      string          // Empty string means auto-detect default device.
	captureMode CaptureMode     // Microphone, loopback, or mix.
	loopback    *loopbackDevice // Cached loopback device (for loopback/mix modes).
}

// NewFFmpegRecorder creates a new FFmpegRecorder for microphone capture.
// ffmpegPath must be a valid path to the FFmpeg binary.
// device can be empty for auto-detection, or a specific device name:
//   - macOS: ":0" or ":DeviceName"
//   - Linux: "default" or "hw:0"
//   - Windows: "Microphone (Realtek High Definition Audio)"
func NewFFmpegRecorder(ffmpegPath, device string) (*FFmpegRecorder, error) {
	if ffmpegPath == "" {
		return nil, fmt.Errorf("ffmpegPath cannot be empty: %w", ErrFFmpegNotFound)
	}
	return &FFmpegRecorder{
		ffmpegPath:  ffmpegPath,
		device:      device,
		captureMode: CaptureMicrophone,
	}, nil
}

// NewFFmpegLoopbackRecorder creates a recorder for system audio (loopback) capture.
// It auto-detects the loopback device (BlackHole on macOS, PulseAudio monitor on Linux,
// Stereo Mix or virtual-audio-capturer on Windows).
// Returns ErrLoopbackNotFound with installation instructions if no device found.
func NewFFmpegLoopbackRecorder(ctx context.Context, ffmpegPath string) (*FFmpegRecorder, error) {
	if ffmpegPath == "" {
		return nil, fmt.Errorf("ffmpegPath cannot be empty: %w", ErrFFmpegNotFound)
	}

	loopback, err := detectLoopbackDevice(ctx, ffmpegPath)
	if err != nil {
		return nil, err
	}

	return &FFmpegRecorder{
		ffmpegPath:  ffmpegPath,
		device:      loopback.name,
		captureMode: CaptureLoopback,
		loopback:    loopback,
	}, nil
}

// NewFFmpegMixRecorder creates a recorder that captures both microphone and system audio.
// This is useful for recording video calls where you want both your voice and the remote audio.
// Returns ErrLoopbackNotFound if the loopback device is not available.
func NewFFmpegMixRecorder(ctx context.Context, ffmpegPath, micDevice string) (*FFmpegRecorder, error) {
	if ffmpegPath == "" {
		return nil, fmt.Errorf("ffmpegPath cannot be empty: %w", ErrFFmpegNotFound)
	}

	loopback, err := detectLoopbackDevice(ctx, ffmpegPath)
	if err != nil {
		return nil, err
	}

	return &FFmpegRecorder{
		ffmpegPath:  ffmpegPath,
		device:      micDevice, // Will be resolved in Record()
		captureMode: CaptureMix,
		loopback:    loopback,
	}, nil
}

// Record records audio for the specified duration and writes to output.
// The output format is OGG Vorbis at 16kHz mono ~50kbps (optimized for voice).
// If device is empty, it auto-detects the default audio input device.
// Recording can be interrupted via context cancellation (Ctrl+C).
func (r *FFmpegRecorder) Record(ctx context.Context, duration time.Duration, output string) error {
	switch r.captureMode {
	case CaptureLoopback:
		return r.recordLoopback(ctx, duration, output)
	case CaptureMix:
		return r.recordMix(ctx, duration, output)
	default:
		return r.recordMicrophone(ctx, duration, output)
	}
}

// recordMicrophone records from the microphone input device.
func (r *FFmpegRecorder) recordMicrophone(ctx context.Context, duration time.Duration, output string) error {
	device := r.device
	if device == "" {
		detected, err := r.detectDefaultDevice(ctx)
		if err != nil {
			return err
		}
		device = detected
	}

	format := inputFormat()
	inputArg := formatInputArg(format, device)

	args := []string{
		"-y",                                        // Overwrite output without asking.
		"-f", format,                                // Input format (avfoundation/alsa/dshow).
		"-i", inputArg,                              // Input device.
		"-t", strconv.Itoa(int(duration.Seconds())), // Duration in seconds.
		"-c:a", "libvorbis",                         // OGG Vorbis codec.
		"-ar", "16000",                              // 16kHz sample rate.
		"-ac", "1",                                  // Mono.
		"-q:a", "2",                                 // Quality ~50kbps.
		output,
	}

	return runFFmpegGraceful(ctx, r.ffmpegPath, args, gracefulShutdownTimeout)
}

// recordLoopback records from the loopback device (system audio).
func (r *FFmpegRecorder) recordLoopback(ctx context.Context, duration time.Duration, output string) error {
	// Loopback device was detected and cached in NewFFmpegLoopbackRecorder.
	args := []string{
		"-y",                                        // Overwrite output without asking.
		"-f", r.loopback.format,                     // Input format (avfoundation/pulse/dshow).
		"-i", r.loopback.name,                       // Loopback device.
		"-t", strconv.Itoa(int(duration.Seconds())), // Duration in seconds.
		"-c:a", "libvorbis",                         // OGG Vorbis codec.
		"-ar", "16000",                              // 16kHz sample rate.
		"-ac", "1",                                  // Mono.
		"-q:a", "2",                                 // Quality ~50kbps.
		output,
	}

	return runFFmpegGraceful(ctx, r.ffmpegPath, args, gracefulShutdownTimeout)
}

// recordMix records both microphone and loopback mixed together.
func (r *FFmpegRecorder) recordMix(ctx context.Context, duration time.Duration, output string) error {
	// Get microphone device
	micDevice := r.device
	if micDevice == "" {
		detected, err := r.detectDefaultDevice(ctx)
		if err != nil {
			return err
		}
		micDevice = detected
	}

	// Loopback device was detected and cached in NewFFmpegMixRecorder.
	micFormat := inputFormat()
	micInputArg := formatInputArg(micFormat, micDevice)

	// Build FFmpeg command with two inputs and amix filter
	args := []string{
		"-y", // Overwrite output without asking.
		// Input 1: Microphone
		"-f", micFormat,
		"-i", micInputArg,
		// Input 2: Loopback
		"-f", r.loopback.format,
		"-i", r.loopback.name,
		// Mix both inputs
		"-filter_complex", "amix=inputs=2:duration=first:dropout_transition=2",
		"-t", strconv.Itoa(int(duration.Seconds())), // Duration in seconds.
		"-c:a", "libvorbis", // OGG Vorbis codec.
		"-ar", "16000",      // 16kHz sample rate.
		"-ac", "1",          // Mono.
		"-q:a", "2",         // Quality ~50kbps.
		output,
	}

	return runFFmpegGraceful(ctx, r.ffmpegPath, args, gracefulShutdownTimeout)
}

// ListDevices returns a list of available audio input devices.
// This can be used to help users select a device via --device flag.
func (r *FFmpegRecorder) ListDevices(ctx context.Context) ([]string, error) {
	return r.listDevices(ctx)
}

// detectDefaultDevice auto-detects the default audio input device for the current OS.
// Returns an error with available devices listed if detection fails.
func (r *FFmpegRecorder) detectDefaultDevice(ctx context.Context) (string, error) {
	format := inputFormat()

	devices, err := r.listDevices(ctx)
	if err != nil {
		// Fallback: return generic help message.
		return "", &deviceError{
			wrapped: ErrNoAudioDevice,
			help:    fmt.Sprintf("run 'ffmpeg -f %s -list_devices true -i dummy' to see available devices, use --device to specify one", format),
		}
	}

	if len(devices) == 0 {
		return "", &deviceError{
			wrapped: ErrNoAudioDevice,
			help:    "no audio input devices detected, check that a microphone is connected and enabled",
		}
	}

	// Return the first detected device.
	return devices[0], nil
}

// listDevices queries FFmpeg for available audio input devices.
// The output format varies by OS, so we parse accordingly.
// On Linux, prefers PulseAudio (pactl) over ALSA for better device discovery.
func (r *FFmpegRecorder) listDevices(ctx context.Context) ([]string, error) {
	format := inputFormat()

	// On Linux, try PulseAudio first for better device discovery.
	if runtime.GOOS == "linux" {
		if devices := listPulseDevices(ctx); len(devices) > 0 {
			return devices, nil
		}
		// Fall back to ALSA defaults.
	}

	args := listDevicesArgs(format)

	stderr, err := runFFmpegOutput(ctx, r.ffmpegPath, args)
	if err != nil {
		return nil, err
	}

	return parseDevices(format, stderr), nil
}

// listPulseDevices uses pactl to list PulseAudio sources (Linux).
// Returns nil if pactl is not available or fails.
func listPulseDevices(ctx context.Context) []string {
	cmd := exec.CommandContext(ctx, "pactl", "list", "sources", "short")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parsePulseDevices(string(output))
}

// inputFormat returns the FFmpeg input format for the current OS.
func inputFormat() string {
	switch runtime.GOOS {
	case "darwin":
		return "avfoundation"
	case "windows":
		return "dshow"
	default:
		// Linux and others default to ALSA.
		return "alsa"
	}
}

// listDevicesArgs returns FFmpeg arguments to list audio devices for the given format.
func listDevicesArgs(format string) []string {
	switch format {
	case "avfoundation":
		// macOS: list_devices outputs to stderr, -i "" triggers the listing.
		return []string{"-f", "avfoundation", "-list_devices", "true", "-i", ""}
	case "dshow":
		// Windows: list_devices outputs to stderr, -i dummy triggers the listing.
		return []string{"-f", "dshow", "-list_devices", "true", "-i", "dummy"}
	default:
		// Linux ALSA: we use arecord-style listing via FFmpeg.
		// Note: FFmpeg doesn't have -list_devices for ALSA, we return common defaults.
		return []string{"-f", "alsa", "-i", "default", "-t", "0", "-f", "null", "-"}
	}
}

// formatInputArg formats the device name for FFmpeg -i argument based on OS.
func formatInputArg(format, device string) string {
	switch format {
	case "avfoundation":
		// macOS: audio-only input uses ":deviceindex" or ":devicename".
		if strings.HasPrefix(device, ":") {
			return device
		}
		return ":" + device
	case "dshow":
		// Windows: format is "audio=DeviceName".
		if strings.HasPrefix(device, "audio=") {
			return device
		}
		return "audio=" + device
	default:
		// Linux ALSA: device name is used directly.
		return device
	}
}

// parseDevices extracts device names from FFmpeg -list_devices output.
// Returns nil if parsing fails (caller should use fallback message).
func parseDevices(format, stderr string) []string {
	switch format {
	case "avfoundation":
		return parseAVFoundationDevices(stderr)
	case "dshow":
		return parseDShowDevices(stderr)
	default:
		return parseALSADevices(stderr)
	}
}

// virtualAudioDevices lists known virtual audio devices that should be deprioritized.
// These are typically used for screen sharing/loopback, not microphone input.
// Cross-platform list covering macOS, Windows, and Linux.
var virtualAudioDevices = []string{
	// macOS
	"AirBeamTV",
	"ZoomAudioDevice",
	"Microsoft Teams Audio",
	"BlackHole",
	"Soundflower",
	"Loopback Audio",
	// Windows
	"Stereo Mix",
	"Wave Out Mix",
	"What U Hear",
	"Lo que escucha", // Spanish locale
	"CABLE Output",
	"VB-Audio Virtual Cable",
	"virtual-audio-capturer",
	"VoiceMeeter",
	// Linux (PulseAudio/PipeWire)
	".monitor", // PulseAudio monitor devices (e.g., "alsa_output.pci-0000_00_1f.3.analog-stereo.monitor")
}

// isVirtualAudioDevice checks if a device name matches a known virtual audio device.
func isVirtualAudioDevice(name string) bool {
	nameLower := strings.ToLower(name)
	for _, virtual := range virtualAudioDevices {
		if strings.Contains(nameLower, strings.ToLower(virtual)) {
			return true
		}
	}
	return false
}

// isMicrophoneDevice checks if a device name looks like a real microphone.
// Cross-platform patterns for macOS, Windows, and Linux.
func isMicrophoneDevice(name string) bool {
	nameLower := strings.ToLower(name)
	return strings.Contains(nameLower, "micro") ||
		strings.Contains(nameLower, "input") ||
		strings.Contains(nameLower, "headset") ||
		strings.Contains(nameLower, "webcam") ||
		strings.Contains(nameLower, "usb audio") ||
		// Linux-specific
		strings.Contains(nameLower, "capture") ||
		strings.Contains(nameLower, "analog-stereo") && !strings.Contains(nameLower, ".monitor") ||
		// Windows-specific
		strings.Contains(nameLower, "realtek") && strings.Contains(nameLower, "microphone")
}

// parseAVFoundationDevices parses macOS avfoundation device listing.
// Returns devices sorted with real microphones first, virtual devices last.
// Example output:
//
//	[AVFoundation indev @ 0x...] AVFoundation video devices:
//	[AVFoundation indev @ 0x...] [0] FaceTime HD Camera
//	[AVFoundation indev @ 0x...] AVFoundation audio devices:
//	[AVFoundation indev @ 0x...] [0] AirBeamTV Audio
//	[AVFoundation indev @ 0x...] [1] MacBook Pro Microphone
func parseAVFoundationDevices(stderr string) []string {
	type deviceInfo struct {
		index string
		name  string
	}
	var allDevices []deviceInfo
	inAudioSection := false
	lines := strings.Split(stderr, "\n")

	// Pattern: [0] Device Name
	devicePattern := regexp.MustCompile(`\[(\d+)\]\s+(.+)$`)

	for _, line := range lines {
		if strings.Contains(line, "AVFoundation audio devices:") {
			inAudioSection = true
			continue
		}
		if strings.Contains(line, "AVFoundation video devices:") {
			inAudioSection = false
			continue
		}
		if inAudioSection {
			if matches := devicePattern.FindStringSubmatch(line); matches != nil {
				allDevices = append(allDevices, deviceInfo{
					index: matches[1],
					name:  matches[2],
				})
			}
		}
	}

	// Sort devices: real microphones first, then unknown, then virtual devices.
	var microphones, unknown, virtual []string
	for _, d := range allDevices {
		deviceID := ":" + d.index
		if isVirtualAudioDevice(d.name) {
			virtual = append(virtual, deviceID)
		} else if isMicrophoneDevice(d.name) {
			microphones = append(microphones, deviceID)
		} else {
			unknown = append(unknown, deviceID)
		}
	}

	// Combine: microphones first, then unknown, then virtual.
	var result []string
	result = append(result, microphones...)
	result = append(result, unknown...)
	result = append(result, virtual...)
	return result
}

// parseDShowDevices parses Windows dshow device listing.
// Returns devices sorted with real microphones first, virtual devices last.
// Example output:
//
//	[dshow @ 0x...] DirectShow video devices
//	[dshow @ 0x...]  "Integrated Camera"
//	[dshow @ 0x...] DirectShow audio devices
//	[dshow @ 0x...]  "Microphone (Realtek High Definition Audio)"
//	[dshow @ 0x...]  "Stereo Mix (Realtek High Definition Audio)"
func parseDShowDevices(stderr string) []string {
	var allDevices []string
	inAudioSection := false
	lines := strings.Split(stderr, "\n")

	// Pattern: "Device Name" (quoted).
	devicePattern := regexp.MustCompile(`"([^"]+)"`)

	for _, line := range lines {
		if strings.Contains(line, "DirectShow audio devices") {
			inAudioSection = true
			continue
		}
		if strings.Contains(line, "DirectShow video devices") {
			inAudioSection = false
			continue
		}
		if inAudioSection {
			if matches := devicePattern.FindStringSubmatch(line); matches != nil {
				// Skip "Alternative name" lines.
				if !strings.Contains(line, "Alternative name") {
					allDevices = append(allDevices, matches[1])
				}
			}
		}
	}

	// Sort devices: real microphones first, then unknown, then virtual devices.
	var microphones, unknown, virtual []string
	for _, name := range allDevices {
		if isVirtualAudioDevice(name) {
			virtual = append(virtual, name)
		} else if isMicrophoneDevice(name) {
			microphones = append(microphones, name)
		} else {
			unknown = append(unknown, name)
		}
	}

	// Combine: microphones first, then unknown, then virtual.
	var result []string
	result = append(result, microphones...)
	result = append(result, unknown...)
	result = append(result, virtual...)
	return result
}

// parseALSADevices returns default ALSA devices.
// FFmpeg doesn't provide -list_devices for ALSA, so we return common defaults.
// Users on Linux should use `arecord -l` to list devices and specify via --device.
func parseALSADevices(_ string) []string {
	// Return common ALSA defaults. The user may need to use --device for specific hardware.
	return []string{"default", "hw:0", "plughw:0"}
}

// parsePulseDevices parses PulseAudio source listing for Linux.
// Uses `pactl list sources short` output format:
//
//	0	alsa_output.pci-0000_00_1f.3.analog-stereo.monitor	module-alsa-card.c	s16le 2ch 44100Hz	IDLE
//	1	alsa_input.pci-0000_00_1f.3.analog-stereo	module-alsa-card.c	s16le 2ch 44100Hz	IDLE
//
// Returns devices sorted with real microphones first, monitor devices last.
func parsePulseDevices(output string) []string {
	var allDevices []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			// Second field is the source name
			allDevices = append(allDevices, fields[1])
		}
	}

	// Sort devices: real microphones first, then unknown, then virtual/monitor devices.
	var microphones, unknown, virtual []string
	for _, name := range allDevices {
		if isVirtualAudioDevice(name) {
			virtual = append(virtual, name)
		} else if isMicrophoneDevice(name) {
			microphones = append(microphones, name)
		} else {
			unknown = append(unknown, name)
		}
	}

	// Combine: microphones first, then unknown, then virtual.
	var result []string
	result = append(result, microphones...)
	result = append(result, unknown...)
	result = append(result, virtual...)
	return result
}
