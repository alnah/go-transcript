package ffmpeg

import "errors"

// ErrNotFound indicates FFmpeg binary is not installed and auto-download failed.
var ErrNotFound = errors.New("ffmpeg not found")

// ErrUnsupportedPlatform indicates the OS/architecture is not supported for auto-download.
var ErrUnsupportedPlatform = errors.New("unsupported platform for FFmpeg auto-download")

// ErrChecksumMismatch indicates a downloaded file's checksum verification failed.
var ErrChecksumMismatch = errors.New("checksum mismatch")

// ErrDownloadFailed indicates a file download could not be completed.
var ErrDownloadFailed = errors.New("download failed")

// ErrTimeout is returned when FFmpeg does not exit within the graceful shutdown timeout.
var ErrTimeout = errors.New("ffmpeg did not exit within timeout")
