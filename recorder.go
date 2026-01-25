package main

import (
	"context"
	"fmt"
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
	device      string      // Empty string means auto-detect default device.
	captureMode CaptureMode // Microphone, loopback, or mix.
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
	}, nil
}

// NewFFmpegMixRecorder creates a recorder that captures both microphone and system audio.
// This is useful for recording video calls where you want both your voice and the remote audio.
// Returns ErrLoopbackNotFound if the loopback device is not available.
func NewFFmpegMixRecorder(ctx context.Context, ffmpegPath, micDevice string) (*FFmpegRecorder, error) {
	if ffmpegPath == "" {
		return nil, fmt.Errorf("ffmpegPath cannot be empty: %w", ErrFFmpegNotFound)
	}

	_, err := detectLoopbackDevice(ctx, ffmpegPath)
	if err != nil {
		return nil, err
	}

	return &FFmpegRecorder{
		ffmpegPath:  ffmpegPath,
		device:      micDevice, // Will be resolved in Record()
		captureMode: CaptureMix,
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
	// Loopback device is already resolved in NewFFmpegLoopbackRecorder
	// and stored in r.device with the appropriate format.
	loopback, err := detectLoopbackDevice(ctx, r.ffmpegPath)
	if err != nil {
		return err
	}

	args := []string{
		"-y",                                        // Overwrite output without asking.
		"-f", loopback.format,                       // Input format (avfoundation/pulse/dshow).
		"-i", loopback.name,                         // Loopback device.
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

	// Get loopback device
	loopback, err := detectLoopbackDevice(ctx, r.ffmpegPath)
	if err != nil {
		return err
	}

	micFormat := inputFormat()
	micInputArg := formatInputArg(micFormat, micDevice)

	// Build FFmpeg command with two inputs and amix filter
	args := []string{
		"-y", // Overwrite output without asking.
		// Input 1: Microphone
		"-f", micFormat,
		"-i", micInputArg,
		// Input 2: Loopback
		"-f", loopback.format,
		"-i", loopback.name,
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
func (r *FFmpegRecorder) listDevices(ctx context.Context) ([]string, error) {
	format := inputFormat()
	args := listDevicesArgs(format)

	stderr, err := runFFmpegOutput(ctx, r.ffmpegPath, args)
	if err != nil {
		return nil, err
	}

	return parseDevices(format, stderr), nil
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

// parseAVFoundationDevices parses macOS avfoundation device listing.
// Example output:
//
//	[AVFoundation indev @ 0x...] AVFoundation video devices:
//	[AVFoundation indev @ 0x...] [0] FaceTime HD Camera
//	[AVFoundation indev @ 0x...] AVFoundation audio devices:
//	[AVFoundation indev @ 0x...] [0] MacBook Pro Microphone
//	[AVFoundation indev @ 0x...] [1] External Microphone
func parseAVFoundationDevices(stderr string) []string {
	var devices []string
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
				// Return as ":index" format for FFmpeg -i argument.
				devices = append(devices, ":"+matches[1])
			}
		}
	}
	return devices
}

// parseDShowDevices parses Windows dshow device listing.
// Example output:
//
//	[dshow @ 0x...] DirectShow video devices
//	[dshow @ 0x...]  "Integrated Camera"
//	[dshow @ 0x...] DirectShow audio devices
//	[dshow @ 0x...]  "Microphone (Realtek High Definition Audio)"
//	[dshow @ 0x...]  "Stereo Mix (Realtek High Definition Audio)"
func parseDShowDevices(stderr string) []string {
	var devices []string
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
					devices = append(devices, matches[1])
				}
			}
		}
	}
	return devices
}

// parseALSADevices returns default ALSA devices.
// FFmpeg doesn't provide -list_devices for ALSA, so we return common defaults.
// Users on Linux should use `arecord -l` to list devices and specify via --device.
func parseALSADevices(_ string) []string {
	// Return common ALSA defaults. The user may need to use --device for specific hardware.
	return []string{"default", "hw:0", "plughw:0"}
}
