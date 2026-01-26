package main

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// FFmpeg version and download configuration.
// Binaries from github.com/eugeneware/ffmpeg-static release b6.1.1.
const (
	ffmpegVersion = "6.1.1"

	// downloadTimeout is the maximum time allowed for downloading ffmpeg.
	// Binary is ~20-30MB compressed, allowing for slow connections.
	downloadTimeout = 10 * time.Minute

	// versionFileName stores the installed version for upgrade detection.
	versionFileName = ".version"

	// maxDecompressedSize prevents decompression bombs.
	// FFmpeg binary is ~80MB uncompressed; 200MB limit provides safety margin.
	maxDecompressedSize = 200 * 1024 * 1024

	// minFFmpegMajorVersion is the minimum supported ffmpeg version.
	// Versions below this may lack required features (silencedetect improvements, codec support).
	minFFmpegMajorVersion = 4
)

// Environment variable for custom ffmpeg path.
const envFFmpegPath = "FFMPEG_PATH"

// downloadClient is a dedicated HTTP client for FFmpeg downloads with explicit timeouts.
var downloadClient = &http.Client{
	Timeout: downloadTimeout,
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

// binaryInfo contains download metadata for ffmpeg.
type binaryInfo struct {
	URL    string // Download URL (gzipped binary)
	SHA256 string // Expected checksum of the gzipped file
}

// downloadBaseURL is the base URL for eugeneware/ffmpeg-static releases.
const downloadBaseURL = "https://github.com/eugeneware/ffmpeg-static/releases/download/b6.1.1"

// platforms maps GOOS-GOARCH to ffmpeg download information.
// SHA256 checksums are for the .gz compressed files.
//
// To update checksums after a new release:
//  1. Download each .gz file
//  2. Run: shasum -a 256 <filename>
//  3. Update the SHA256 field below
var platforms = map[string]binaryInfo{
	"darwin-arm64": {
		URL:    downloadBaseURL + "/ffmpeg-darwin-arm64.gz",
		SHA256: "8923876afa8db5585022d7860ec7e589af192f441c56793971276d450ed3bbfa",
	},
	"darwin-amd64": {
		URL:    downloadBaseURL + "/ffmpeg-darwin-x64.gz",
		SHA256: "5d8fb6f280c428d0e82cd5ee68215f0734d64f88e37dcc9e082f818c9e5025f0",
	},
	"linux-amd64": {
		URL:    downloadBaseURL + "/ffmpeg-linux-x64.gz",
		SHA256: "bfe8a8fc511530457b528c48d77b5737527b504a3797a9bc4866aeca69c2dffa",
	},
	"windows-amd64": {
		URL:    downloadBaseURL + "/ffmpeg-win32-x64.gz",
		SHA256: "8883a3dffbd0a16cf4ef95206ea05283f78908dbfb118f73c83f4951dcc06d77",
	},
}

// installDir returns the directory where ffmpeg is installed.
// Default: ~/.go-transcript/bin/
func installDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".go-transcript", "bin"), nil
}

// installedPath returns the path where ffmpeg would be installed.
func installedPath() (string, error) {
	dir, err := installDir()
	if err != nil {
		return "", err
	}

	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	return filepath.Join(dir, name), nil
}

// isInstalled checks if ffmpeg is already installed at the expected location.
// Returns true only if the binary exists and the version file matches.
func isInstalled() (bool, error) {
	ffmpegPath, err := installedPath()
	if err != nil {
		return false, err
	}

	if _, err := os.Stat(ffmpegPath); os.IsNotExist(err) {
		return false, nil
	}

	// Check version file matches current version
	dir, _ := installDir()
	versionPath := filepath.Join(dir, versionFileName)
	// #nosec G304 -- path constructed from user home dir, not user input
	data, err := os.ReadFile(versionPath)
	if err != nil {
		return false, nil // Version file missing = needs reinstall
	}
	if string(data) != ffmpegVersion {
		return false, nil // Version mismatch = needs upgrade
	}

	return true, nil
}

// resolveFFmpeg finds ffmpeg using the following precedence:
//  1. FFMPEG_PATH environment variable (error if set but invalid)
//  2. ~/.go-transcript/bin/ffmpeg (installed by us)
//  3. System PATH
//  4. Auto-download if nothing found
//
// Returns the path to the ffmpeg binary.
func resolveFFmpeg(ctx context.Context) (string, error) {
	// 1. Check FFMPEG_PATH environment variable
	if envPath := os.Getenv(envFFmpegPath); envPath != "" {
		if _, err := os.Stat(envPath); err != nil {
			return "", fmt.Errorf("%w: %s is set to %q but binary not found (unset to enable auto-download)",
				ErrFFmpegNotFound, envFFmpegPath, envPath)
		}
		return envPath, nil
	}

	// 2. Check our install directory
	installed, err := isInstalled()
	if err != nil {
		return "", err
	}
	if installed {
		path, _ := installedPath()
		return path, nil
	}

	// 3. Check system PATH
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path, nil
	}

	// 4. Auto-download
	fmt.Fprintln(os.Stderr, "ffmpeg not found, downloading...")
	if err := downloadAndInstall(ctx); err != nil {
		return "", fmt.Errorf("%w: auto-download failed: %v\n\n%s",
			ErrFFmpegNotFound, err, manualInstallInstructions())
	}

	path, _ := installedPath()
	return path, nil
}

// checkFFmpegVersion verifies that ffmpeg meets minimum version requirements.
// Prints a warning to stderr if version is below minimum but doesn't fail.
func checkFFmpegVersion(ctx context.Context, ffmpegPath string) {
	cmd := exec.CommandContext(ctx, ffmpegPath, "-version")
	output, err := cmd.Output()
	if err != nil {
		return // Can't check version, proceed anyway
	}

	// Parse version from output like "ffmpeg version 6.1.1 Copyright..."
	lines := strings.Split(string(output), "\n")
	if len(lines) == 0 {
		return
	}

	var major int
	_, err = fmt.Sscanf(lines[0], "ffmpeg version %d", &major)
	if err != nil {
		// Try alternative format "ffmpeg version n6.1.1..."
		_, err = fmt.Sscanf(lines[0], "ffmpeg version n%d", &major)
		if err != nil {
			return // Can't parse version
		}
	}

	if major < minFFmpegMajorVersion {
		fmt.Fprintf(os.Stderr, "Warning: ffmpeg version %d detected, version %d+ recommended\n",
			major, minFFmpegMajorVersion)
	}
}

// downloadAndInstall downloads and installs ffmpeg.
func downloadAndInstall(ctx context.Context) error {
	platform := runtime.GOOS + "-" + runtime.GOARCH
	info, ok := platforms[platform]
	if !ok {
		return fmt.Errorf("%w: %s (supported: darwin-arm64, darwin-amd64, linux-amd64, windows-amd64)",
			ErrUnsupportedPlatform, platform)
	}

	dir, err := installDir()
	if err != nil {
		return err
	}

	// Create install directory
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create install directory %s: %w", dir, err)
	}

	// Determine binary name
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	destPath := filepath.Join(dir, name)

	// Download binary
	if err := downloadBinary(ctx, info, destPath); err != nil {
		_ = os.Remove(destPath) // Cleanup on failure
		return fmt.Errorf("failed to download ffmpeg: %w", err)
	}

	// Write version file
	versionPath := filepath.Join(dir, versionFileName)
	if err := os.WriteFile(versionPath, []byte(ffmpegVersion), 0644); err != nil {
		return fmt.Errorf("failed to write version file: %w", err)
	}

	return nil
}

// downloadBinary downloads, verifies, and extracts ffmpeg.
// Uses atomic write pattern: download to temp file, verify, then rename.
func downloadBinary(ctx context.Context, info binaryInfo, destPath string) error {
	dir := filepath.Dir(destPath)
	tempFile, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	// Ensure cleanup on any error
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}()

	// Download with timeout
	downloadCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	if err := downloadToFile(downloadCtx, info.URL, tempFile); err != nil {
		return err
	}

	// Close to flush writes before checksum
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Verify checksum
	if err := verifyChecksum(tempPath, info.SHA256); err != nil {
		return err
	}

	// Decompress gzip to final destination
	if err := decompressGzip(tempPath, destPath); err != nil {
		return err
	}

	// Make executable on Unix
	if runtime.GOOS != "windows" {
		if err := os.Chmod(destPath, 0755); err != nil {
			return fmt.Errorf("failed to make binary executable: %w", err)
		}
	}

	return nil
}

// downloadToFile downloads a URL to an open file.
func downloadToFile(ctx context.Context, url string, dest *os.File) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("%w: invalid URL: %v", ErrDownloadFailed, err)
	}

	resp, err := downloadClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDownloadFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d from %s", ErrDownloadFailed, resp.StatusCode, url)
	}

	if _, err = io.Copy(dest, resp.Body); err != nil {
		return fmt.Errorf("%w: %v", ErrDownloadFailed, err)
	}

	return nil
}

// verifyChecksum computes the SHA256 of a file and compares to expected.
func verifyChecksum(filePath, expectedSHA256 string) error {
	f, err := os.Open(filePath) // #nosec G304 -- filePath is internal temp file
	if err != nil {
		return fmt.Errorf("cannot open file for checksum: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("failed to compute checksum: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expectedSHA256 {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expectedSHA256, actual)
	}

	return nil
}

// decompressGzip decompresses a gzip file to a destination path.
// Uses atomic write pattern.
func decompressGzip(gzPath, destPath string) error {
	gzFile, err := os.Open(gzPath) // #nosec G304 -- gzPath is internal temp file
	if err != nil {
		return fmt.Errorf("cannot open gzip file: %w", err)
	}
	defer func() { _ = gzFile.Close() }()

	gzReader, err := gzip.NewReader(gzFile)
	if err != nil {
		return fmt.Errorf("invalid gzip file: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	// Create temp file for atomic write
	dir := filepath.Dir(destPath)
	tempFile, err := os.CreateTemp(dir, ".extract-*")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	// Ensure cleanup on error
	success := false
	defer func() {
		_ = tempFile.Close()
		if !success {
			_ = os.Remove(tempPath)
		}
	}()

	// Decompress with size limit to prevent decompression bombs
	limitedReader := io.LimitReader(gzReader, maxDecompressedSize)
	written, err := io.Copy(tempFile, limitedReader)
	if err != nil {
		return fmt.Errorf("decompression failed: %w", err)
	}
	if written >= maxDecompressedSize {
		return fmt.Errorf("decompression failed: file exceeds %d bytes limit", maxDecompressedSize)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, destPath); err != nil {
		return fmt.Errorf("failed to install binary: %w", err)
	}

	success = true
	return nil
}

// manualInstallInstructions returns platform-specific instructions for manual FFmpeg installation.
func manualInstallInstructions() string {
	switch runtime.GOOS {
	case "darwin":
		return `To install FFmpeg manually:
  brew install ffmpeg

Or download from https://evermeet.cx/ffmpeg/

Or set FFMPEG_PATH environment variable to your ffmpeg binary.`
	case "linux":
		return `To install FFmpeg manually:
  Ubuntu/Debian: sudo apt install ffmpeg
  Fedora:        sudo dnf install ffmpeg
  Arch:          sudo pacman -S ffmpeg

Or set FFMPEG_PATH environment variable to your ffmpeg binary.`
	case "windows":
		return `To install FFmpeg manually:
  winget install ffmpeg

Or download from https://www.gyan.dev/ffmpeg/builds/

Or set FFMPEG_PATH environment variable to your ffmpeg.exe.`
	default:
		return `To install FFmpeg manually, download from https://ffmpeg.org/download.html
Or set FFMPEG_PATH environment variable to your ffmpeg binary.`
	}
}
