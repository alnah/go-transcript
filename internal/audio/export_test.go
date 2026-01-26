package audio

import (
	"context"
	"time"
)

// Export internal functions for testing.
// This file is only compiled during tests (suffix _test.go).

// ParseDurationFromFFmpegOutput exports parseDurationFromFFmpegOutput for testing.
var ParseDurationFromFFmpegOutput = parseDurationFromFFmpegOutput

// ParseTimeComponents exports parseTimeComponents for testing.
var ParseTimeComponents = parseTimeComponents

// FormatFFmpegTime exports formatFFmpegTime for testing.
var FormatFFmpegTime = formatFFmpegTime

// ParseSilenceOutput exports parseSilenceOutput for testing.
// Returns test-visible SilencePointTest instead of internal silencePoint.
func ParseSilenceOutput(output string) []SilencePointTest {
	internal := parseSilenceOutput(output)
	result := make([]SilencePointTest, len(internal))
	for i, s := range internal {
		result[i] = SilencePointTest{Start: s.start, End: s.end}
	}
	return result
}

// TrimTrailingSilence exports trimTrailingSilence for testing.
// Note: silencePoint is unexported, so we use a wrapper.
func TrimTrailingSilence(silences []SilencePointTest, totalDuration time.Duration) time.Duration {
	internal := make([]silencePoint, len(silences))
	for i, s := range silences {
		internal[i] = silencePoint{start: s.Start, end: s.End}
	}
	return trimTrailingSilence(internal, totalDuration)
}

// SilencePointTest is a test-visible version of silencePoint.
type SilencePointTest struct {
	Start time.Duration
	End   time.Duration
}

// ExpandBoundariesForDuration exports expandBoundariesForDuration for testing.
var ExpandBoundariesForDuration = expandBoundariesForDuration

// SelectCutPoints exports selectCutPoints for testing.
// Note: requires a SilenceChunker instance, so we create a minimal one.
func SelectCutPoints(silences []SilencePointTest, bytesPerSecond float64, maxChunkSize int64) []time.Duration {
	sc := &SilenceChunker{
		maxChunkSize: maxChunkSize,
	}
	internal := make([]silencePoint, len(silences))
	for i, s := range silences {
		internal[i] = silencePoint{start: s.Start, end: s.End}
	}
	return sc.selectCutPoints(internal, bytesPerSecond)
}

// ChunkEncodingArgs exports chunkEncodingArgs for testing.
var ChunkEncodingArgs = chunkEncodingArgs

// --- Chunker dependency injection exports ---

// CommandRunner exports commandRunner interface for testing.
type CommandRunner = commandRunner

// TempDirCreator exports tempDirCreator interface for testing.
type TempDirCreator = tempDirCreator

// FileRemover exports fileRemover interface for testing.
type FileRemover = fileRemover

// FileStatter exports fileStatter interface for testing.
type FileStatter = fileStatter

// --- Recorder exports ---

// InputFormat exports inputFormat for testing.
var InputFormat = inputFormat

// FormatInputArg exports formatInputArg for testing.
var FormatInputArg = formatInputArg

// ListDevicesArgs exports listDevicesArgs for testing.
var ListDevicesArgs = listDevicesArgs

// BuildRecordArgs exports buildRecordArgs for testing.
// Wraps to convert duration from seconds to time.Duration internally.
func BuildRecordArgs(inputFormat, inputArg string, durationSec int, output string) []string {
	return buildRecordArgs(inputFormat, inputArg, time.Duration(durationSec)*time.Second, output)
}

// EncodingArgs exports encodingArgs for testing.
var EncodingArgs = encodingArgs

// IsVirtualAudioDevice exports isVirtualAudioDevice for testing.
var IsVirtualAudioDevice = isVirtualAudioDevice

// IsMicrophoneDevice exports isMicrophoneDevice for testing.
var IsMicrophoneDevice = isMicrophoneDevice

// ParseAVFoundationDevices exports parseAVFoundationDevices for testing.
var ParseAVFoundationDevices = parseAVFoundationDevices

// ParseDShowDevices exports parseDShowDevices for testing.
var ParseDShowDevices = parseDShowDevices

// ParseALSADevices exports parseALSADevices for testing.
var ParseALSADevices = parseALSADevices

// ParsePulseDevices exports parsePulseDevices for testing.
var ParsePulseDevices = parsePulseDevices

// --- Recorder dependency injection exports ---

// FFmpegRunner exports ffmpegRunner interface for testing.
type FFmpegRunner = ffmpegRunner

// PactlRunner exports pactlRunner interface for testing.
type PactlRunner = pactlRunner

// RecorderOption exports RecorderOption for testing.
type ExportedRecorderOption = RecorderOption

// WithFFmpegRunner exports WithFFmpegRunner for testing.
var ExportedWithFFmpegRunner = WithFFmpegRunner

// WithPactlRunner exports WithPactlRunner for testing.
var ExportedWithPactlRunner = WithPactlRunner

// --- Loopback exports ---

// ExtractDShowDeviceName exports extractDShowDeviceName for testing.
var ExtractDShowDeviceName = extractDShowDeviceName

// NewLoopbackError creates a loopbackError for testing.
func NewLoopbackError(wrapped error, help string) error {
	return &loopbackError{wrapped: wrapped, help: help}
}

// LoopbackInstallInstructionsDarwin exports loopbackInstallInstructionsDarwin for testing.
var LoopbackInstallInstructionsDarwin = loopbackInstallInstructionsDarwin

// LoopbackInstallInstructionsLinux exports loopbackInstallInstructionsLinux for testing.
var LoopbackInstallInstructionsLinux = loopbackInstallInstructionsLinux

// LoopbackInstallInstructionsWindows exports loopbackInstallInstructionsWindows for testing.
var LoopbackInstallInstructionsWindows = loopbackInstallInstructionsWindows

// ShellCommandRunner exports shellCommandRunner interface for testing.
type ShellCommandRunner = shellCommandRunner

// LoopbackDeviceInfo holds exported loopback device info for testing.
type LoopbackDeviceInfo struct {
	Name   string
	Format string
}

// DetectLoopbackLinuxWithRunner exports detectLoopbackLinuxWithRunner for testing.
// Returns device info or error.
func DetectLoopbackLinuxWithRunner(ctx context.Context, runner ShellCommandRunner) (*LoopbackDeviceInfo, error) {
	dev, err := detectLoopbackLinuxWithRunner(ctx, runner)
	if err != nil {
		return nil, err
	}
	return &LoopbackDeviceInfo{Name: dev.name, Format: dev.format}, nil
}

// --- Chunker warning exports ---

// ExportedWarnFunc exports WarnFunc type alias for testing.
type ExportedWarnFunc = WarnFunc

// ExportedWithWarnFunc exports WithWarnFunc for testing.
var ExportedWithWarnFunc = WithWarnFunc
