package main

import "errors"

// Sentinel errors for go-transcript.
//
// Usage pattern: wrap sentinels with context at call site using fmt.Errorf:
//
//	return fmt.Errorf("invalid duration %q: %w", value, ErrInvalidDuration)
//
// This preserves errors.Is() compatibility while adding context.
// The exitCode() function in main.go maps these to spec-defined exit codes.

// --- Setup errors (ExitSetup = 3) ---
// Environment and dependency errors that prevent the tool from running.

var (
	// ErrFFmpegNotFound indicates FFmpeg binary is not installed and auto-download failed.
	ErrFFmpegNotFound = errors.New("ffmpeg not found")

	// ErrAPIKeyMissing indicates OPENAI_API_KEY environment variable is not set.
	ErrAPIKeyMissing = errors.New("OPENAI_API_KEY environment variable not set")

	// ErrUnsupportedPlatform indicates the OS/architecture is not supported for auto-download.
	ErrUnsupportedPlatform = errors.New("unsupported platform for FFmpeg auto-download")

	// ErrChecksumMismatch indicates a downloaded file's checksum verification failed.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrDownloadFailed indicates a file download could not be completed.
	ErrDownloadFailed = errors.New("download failed")
)

// --- Validation errors (ExitValidation = 4) ---
// Input validation errors that indicate incorrect usage.

var (
	// ErrInvalidDuration indicates a duration string could not be parsed.
	// Wrap with the invalid value: fmt.Errorf("invalid duration %q: %w", val, ErrInvalidDuration)
	ErrInvalidDuration = errors.New("invalid duration format")

	// ErrUnsupportedFormat indicates an audio file has an unsupported extension.
	// Wrap with the format: fmt.Errorf("unsupported audio format %s: %w", ext, ErrUnsupportedFormat)
	ErrUnsupportedFormat = errors.New("unsupported audio format")

	// ErrFileNotFound indicates the specified input file does not exist.
	ErrFileNotFound = errors.New("file not found")

	// ErrUnknownTemplate indicates an invalid template name was specified.
	// Wrap with the name: fmt.Errorf("unknown template %q: %w", name, ErrUnknownTemplate)
	ErrUnknownTemplate = errors.New("unknown template")

	// ErrOutputExists indicates the output file already exists.
	// Wrap with the path: fmt.Errorf("output file already exists: %s: %w", path, ErrOutputExists)
	ErrOutputExists = errors.New("output file already exists")
)

// --- Transcription errors (ExitTranscription = 5) ---
// API and network errors during transcription.

var (
	// ErrRateLimit indicates OpenAI API rate limit was exceeded (temporary, retryable).
	ErrRateLimit = errors.New("rate limit exceeded")

	// ErrQuotaExceeded indicates OpenAI API quota was exceeded (billing issue, not retryable).
	// This is different from ErrRateLimit - it requires user action (check billing).
	ErrQuotaExceeded = errors.New("quota exceeded")

	// ErrTimeout indicates a request timed out.
	ErrTimeout = errors.New("request timeout")

	// ErrAuthFailed indicates OpenAI API authentication failed (invalid key).
	ErrAuthFailed = errors.New("authentication failed")
)

// --- Restructure errors (ExitRestructure = 6) ---
// Errors during LLM restructuring phase.

var (
	// ErrTranscriptTooLong indicates the transcript exceeds the 100K token limit.
	ErrTranscriptTooLong = errors.New("transcript exceeds 100K token limit")
)

// --- Recorder errors (ExitSetup = 3) ---
// Audio recording errors.

var (
	// ErrNoAudioDevice indicates no audio input device was found or detected.
	ErrNoAudioDevice = errors.New("no audio input device found")

	// ErrLoopbackNotFound indicates no loopback device was detected.
	// The user needs to install a virtual audio driver (BlackHole, PulseAudio monitor, etc.).
	ErrLoopbackNotFound = errors.New("loopback device not found")
)

// --- Chunking errors (ExitValidation = 4) ---
// Errors during audio chunking phase.

var (
	// ErrChunkingFailed indicates FFmpeg failed during audio chunking.
	// Wrap with context: fmt.Errorf("chunking failed: %w", ErrChunkingFailed)
	ErrChunkingFailed = errors.New("audio chunking failed")

	// ErrChunkTooLarge indicates a chunk exceeds the OpenAI API limit (25MB).
	// This is rare with the 20MB safety margin but can happen with VBR audio.
	// Wrap with size: fmt.Errorf("chunk too large (%d MB, max 25MB): %w", sizeMB, ErrChunkTooLarge)
	ErrChunkTooLarge = errors.New("chunk exceeds 25MB limit")
)
