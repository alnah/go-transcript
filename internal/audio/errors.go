package audio

import "errors"

// ErrNoAudioDevice indicates no audio input device was found or detected.
var ErrNoAudioDevice = errors.New("no audio input device found")

// ErrLoopbackNotFound indicates no loopback device was detected.
var ErrLoopbackNotFound = errors.New("loopback device not found")

// ErrChunkingFailed indicates FFmpeg failed during audio chunking.
var ErrChunkingFailed = errors.New("audio chunking failed")

// ErrChunkTooLarge indicates a chunk exceeds the OpenAI API limit (25MB).
var ErrChunkTooLarge = errors.New("chunk exceeds 25MB limit")

// ErrFileNotFound indicates the specified input file does not exist.
var ErrFileNotFound = errors.New("file not found")
