package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Chunk represents a segment of audio extracted from a larger file.
// The caller is responsible for cleaning up chunk files after use.
type Chunk struct {
	Path      string        // Absolute path to the chunk file.
	Index     int           // Zero-based index for ordering.
	StartTime time.Duration // Start timestamp in the source audio.
	EndTime   time.Duration // End timestamp in the source audio.
}

// Duration returns the length of this chunk.
func (c Chunk) Duration() time.Duration {
	return c.EndTime - c.StartTime
}

// String returns a human-readable representation for logging.
func (c Chunk) String() string {
	return fmt.Sprintf("chunk %d: %s-%s",
		c.Index,
		formatDuration(c.StartTime),
		formatDuration(c.EndTime))
}

// formatDuration formats a duration as HH:MM:SS or MM:SS.
func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// Chunker splits an audio file into smaller chunks suitable for transcription.
type Chunker interface {
	// Chunk splits audioPath into multiple chunk files.
	// Returns chunks ordered by their position in the source audio.
	// The caller is responsible for cleaning up the returned chunk files.
	Chunk(ctx context.Context, audioPath string) ([]Chunk, error)
}

// Default chunking parameters.
const (
	// defaultNoiseDB is the silence detection threshold in dB.
	// -30dB is suitable for voice recordings with typical background noise.
	defaultNoiseDB = -30.0

	// defaultMinSilence is the minimum silence duration to detect.
	// 0.5s catches natural pauses in speech without over-splitting.
	defaultMinSilence = 500 * time.Millisecond

	// defaultMaxChunkSize is the target maximum chunk size in bytes.
	// OpenAI limit is 25MB; we use 20MB for VBR safety margin.
	defaultMaxChunkSize = 20 * 1024 * 1024

	// defaultOverlap is the overlap duration for time-based chunking.
	// 30s ensures words at chunk boundaries are captured in at least one chunk.
	defaultOverlap = 30 * time.Second

	// defaultTargetDuration is the target chunk duration for time-based chunking.
	defaultTargetDuration = 10 * time.Minute
)

// TimeChunker splits audio into fixed-duration chunks with overlap.
// This is the fallback strategy when silence detection fails or finds no silences.
type TimeChunker struct {
	ffmpegPath     string
	targetDuration time.Duration
	overlap        time.Duration
}

// NewTimeChunker creates a TimeChunker with the specified parameters.
func NewTimeChunker(ffmpegPath string, targetDuration, overlap time.Duration) (*TimeChunker, error) {
	if ffmpegPath == "" {
		return nil, fmt.Errorf("ffmpegPath cannot be empty: %w", ErrFFmpegNotFound)
	}
	if targetDuration <= 0 {
		targetDuration = defaultTargetDuration
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= targetDuration {
		return nil, fmt.Errorf("overlap (%v) must be less than targetDuration (%v)", overlap, targetDuration)
	}
	return &TimeChunker{
		ffmpegPath:     ffmpegPath,
		targetDuration: targetDuration,
		overlap:        overlap,
	}, nil
}

// Chunk splits the audio file into fixed-duration segments with overlap.
func (tc *TimeChunker) Chunk(ctx context.Context, audioPath string) ([]Chunk, error) {
	// Get total duration of the audio file.
	totalDuration, err := tc.probeDuration(ctx, audioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to probe audio duration: %w", err)
	}

	// Create temp directory for chunks.
	tempDir, err := os.MkdirTemp("", "go-transcript-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Calculate chunk boundaries.
	var chunks []Chunk
	step := tc.targetDuration - tc.overlap
	for i := 0; ; i++ {
		start := time.Duration(i) * step
		if start >= totalDuration {
			break
		}
		end := min(start+tc.targetDuration, totalDuration)

		chunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_%03d.ogg", i))
		if err := tc.extractChunk(ctx, audioPath, chunkPath, start, end); err != nil {
			// Cleanup on failure.
			_ = os.RemoveAll(tempDir)
			return nil, err
		}

		chunks = append(chunks, Chunk{
			Path:      chunkPath,
			Index:     i,
			StartTime: start,
			EndTime:   end,
		})

		// Last chunk reached the end.
		if end >= totalDuration {
			break
		}
	}

	return chunks, nil
}

// probeDuration returns the duration of an audio file using ffprobe/ffmpeg.
func (tc *TimeChunker) probeDuration(ctx context.Context, audioPath string) (time.Duration, error) {
	// Use ffmpeg to get duration (ffprobe may not be available).
	// The -i flag with no output shows file info including duration.
	args := []string{
		"-i", audioPath,
		"-f", "null", "-",
	}
	output, err := runFFmpegOutput(ctx, tc.ffmpegPath, args)
	if err != nil {
		return 0, err
	}

	return parseDurationFromFFmpegOutput(output)
}

// parseDurationFromFFmpegOutput extracts duration from FFmpeg stderr.
// Looks for: "Duration: HH:MM:SS.ms" or "time=HH:MM:SS.ms"
func parseDurationFromFFmpegOutput(output string) (time.Duration, error) {
	// Pattern: Duration: 00:05:23.45
	durationRe := regexp.MustCompile(`Duration:\s*(\d+):(\d+):(\d+)\.(\d+)`)
	if matches := durationRe.FindStringSubmatch(output); matches != nil {
		return parseTimeComponents(matches[1], matches[2], matches[3], matches[4])
	}

	// Fallback pattern: time=00:05:23.45 (from progress output)
	timeRe := regexp.MustCompile(`time=(\d+):(\d+):(\d+)\.(\d+)`)
	// Find all matches and use the last one (final time).
	allMatches := timeRe.FindAllStringSubmatch(output, -1)
	if len(allMatches) > 0 {
		matches := allMatches[len(allMatches)-1]
		return parseTimeComponents(matches[1], matches[2], matches[3], matches[4])
	}

	return 0, fmt.Errorf("could not parse duration from ffmpeg output")
}

// parseTimeComponents converts HH:MM:SS.ms strings to Duration.
func parseTimeComponents(hours, minutes, seconds, centiseconds string) (time.Duration, error) {
	h, _ := strconv.Atoi(hours)
	m, _ := strconv.Atoi(minutes)
	s, _ := strconv.Atoi(seconds)
	// Centiseconds may be 2 digits (.45) or more (.456789).
	cs, _ := strconv.Atoi(centiseconds)
	// Normalize to milliseconds based on digit count.
	ms := cs
	switch len(centiseconds) {
	case 1:
		ms = cs * 100
	case 2:
		ms = cs * 10
	case 3:
		// Already milliseconds.
	default:
		// More precision than we need, truncate.
		for len(centiseconds) > 3 {
			ms /= 10
			centiseconds = centiseconds[:len(centiseconds)-1]
		}
	}

	return time.Duration(h)*time.Hour +
		time.Duration(m)*time.Minute +
		time.Duration(s)*time.Second +
		time.Duration(ms)*time.Millisecond, nil
}

// extractChunk extracts a segment from audioPath to chunkPath.
func (tc *TimeChunker) extractChunk(ctx context.Context, audioPath, chunkPath string, start, end time.Duration) error {
	args := []string{
		"-y",
		"-i", audioPath,
		"-ss", formatFFmpegTime(start),
		"-to", formatFFmpegTime(end),
		"-c", "copy", // No re-encoding.
		chunkPath,
	}

	// #nosec G204 -- ffmpegPath is resolved internally via resolveFFmpeg, not user input
	cmd := exec.CommandContext(ctx, tc.ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: failed to extract chunk %s: %v\nOutput: %s",
			ErrChunkingFailed, chunkPath, err, string(output))
	}
	return nil
}

// formatFFmpegTime formats a duration for FFmpeg -ss/-to arguments.
func formatFFmpegTime(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := d.Seconds() - float64(h*3600+m*60)
	return fmt.Sprintf("%02d:%02d:%06.3f", h, m, s)
}

// SilenceChunker splits audio at detected silence points.
// Falls back to TimeChunker if no silences are found.
type SilenceChunker struct {
	ffmpegPath   string
	noiseDB      float64
	minSilence   time.Duration
	maxChunkSize int64
	fallback     Chunker
}

// SilenceChunkerOption configures a SilenceChunker.
type SilenceChunkerOption func(*SilenceChunker)

// WithNoiseDB sets the silence detection threshold in dB.
// Lower values (more negative) detect quieter sounds as silence.
// Default: -30dB.
func WithNoiseDB(db float64) SilenceChunkerOption {
	return func(sc *SilenceChunker) {
		sc.noiseDB = db
	}
}

// WithMinSilence sets the minimum silence duration to detect.
// Default: 500ms.
func WithMinSilence(d time.Duration) SilenceChunkerOption {
	return func(sc *SilenceChunker) {
		sc.minSilence = d
	}
}

// WithMaxChunkSize sets the target maximum chunk size in bytes.
// Default: 20MB (with safety margin for OpenAI's 25MB limit).
func WithMaxChunkSize(size int64) SilenceChunkerOption {
	return func(sc *SilenceChunker) {
		sc.maxChunkSize = size
	}
}

// WithFallback sets a custom fallback Chunker.
// Default: TimeChunker with 10min target, 30s overlap.
func WithFallback(c Chunker) SilenceChunkerOption {
	return func(sc *SilenceChunker) {
		sc.fallback = c
	}
}

// NewSilenceChunker creates a SilenceChunker with functional options.
// If no fallback is provided, a default TimeChunker is created.
func NewSilenceChunker(ffmpegPath string, opts ...SilenceChunkerOption) (*SilenceChunker, error) {
	if ffmpegPath == "" {
		return nil, fmt.Errorf("ffmpegPath cannot be empty: %w", ErrFFmpegNotFound)
	}

	sc := &SilenceChunker{
		ffmpegPath:   ffmpegPath,
		noiseDB:      defaultNoiseDB,
		minSilence:   defaultMinSilence,
		maxChunkSize: defaultMaxChunkSize,
	}

	for _, opt := range opts {
		opt(sc)
	}

	// Create default fallback if not provided.
	if sc.fallback == nil {
		fallback, err := NewTimeChunker(ffmpegPath, defaultTargetDuration, defaultOverlap)
		if err != nil {
			return nil, fmt.Errorf("failed to create fallback chunker: %w", err)
		}
		sc.fallback = fallback
	}

	return sc, nil
}

// Chunk splits the audio file at silence points.
// If no silences are found, falls back to time-based chunking.
func (sc *SilenceChunker) Chunk(ctx context.Context, audioPath string) ([]Chunk, error) {
	// Get file info for bitrate estimation.
	fileInfo, err := os.Stat(audioPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFileNotFound, err)
	}
	fileSize := fileInfo.Size()

	// Detect silences.
	silences, totalDuration, err := sc.detectSilences(ctx, audioPath)
	if err != nil {
		// Log warning and fall back to time-based chunking.
		fmt.Fprintf(os.Stderr, "Warning: silence detection failed (%v), using time-based chunking\n", err)
		return sc.fallback.Chunk(ctx, audioPath)
	}

	// No silences found - fall back to time-based chunking.
	if len(silences) == 0 {
		fmt.Fprintln(os.Stderr, "Warning: no silences detected, using time-based chunking (may cut mid-sentence)")
		return sc.fallback.Chunk(ctx, audioPath)
	}

	// Calculate average bitrate for size estimation.
	avgBitrate := float64(fileSize) / totalDuration.Seconds() // bytes per second

	// Select cut points that keep chunks under maxChunkSize.
	cutPoints := sc.selectCutPoints(silences, avgBitrate)

	// Create temp directory for chunks.
	tempDir, err := os.MkdirTemp("", "go-transcript-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Extract chunks.
	chunks, err := sc.extractChunks(ctx, audioPath, tempDir, cutPoints, totalDuration)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, err
	}

	return chunks, nil
}

// silencePoint represents a detected silence in the audio.
type silencePoint struct {
	start time.Duration
	end   time.Duration
}

// midpoint returns the middle of the silence, ideal for cutting.
func (s silencePoint) midpoint() time.Duration {
	return s.start + (s.end-s.start)/2
}

// detectSilences runs FFmpeg silencedetect and parses the output.
// Returns silence points and total audio duration.
func (sc *SilenceChunker) detectSilences(ctx context.Context, audioPath string) ([]silencePoint, time.Duration, error) {
	args := []string{
		"-i", audioPath,
		"-af", fmt.Sprintf("silencedetect=noise=%ddB:d=%.2f",
			int(sc.noiseDB),
			sc.minSilence.Seconds()),
		"-f", "null",
		"-",
	}

	output, err := runFFmpegOutput(ctx, sc.ffmpegPath, args)
	if err != nil {
		return nil, 0, err
	}

	silences := parseSilenceOutput(output)
	duration, err := parseDurationFromFFmpegOutput(output)
	if err != nil {
		return nil, 0, fmt.Errorf("could not determine audio duration: %w", err)
	}

	return silences, duration, nil
}

// parseSilenceOutput extracts silence points from FFmpeg silencedetect output.
// FFmpeg outputs lines like:
//
//	[silencedetect @ 0x...] silence_start: 42.123
//	[silencedetect @ 0x...] silence_end: 43.456 | silence_duration: 1.333
func parseSilenceOutput(output string) []silencePoint {
	var silences []silencePoint
	var currentStart time.Duration
	hasStart := false

	// Regex patterns - tolerant of format variations.
	startRe := regexp.MustCompile(`silence_start:\s*([\d.]+)`)
	endRe := regexp.MustCompile(`silence_end:\s*([\d.]+)`)

	for line := range strings.SplitSeq(output, "\n") {
		if matches := startRe.FindStringSubmatch(line); matches != nil {
			seconds, err := strconv.ParseFloat(matches[1], 64)
			if err == nil {
				currentStart = time.Duration(seconds * float64(time.Second))
				hasStart = true
			}
		}
		if matches := endRe.FindStringSubmatch(line); matches != nil && hasStart {
			seconds, err := strconv.ParseFloat(matches[1], 64)
			if err == nil {
				silences = append(silences, silencePoint{
					start: currentStart,
					end:   time.Duration(seconds * float64(time.Second)),
				})
				hasStart = false
			}
		}
	}

	return silences
}

// selectCutPoints chooses silence midpoints that keep chunks under maxChunkSize.
// Uses a greedy algorithm: accumulate silences as candidates until the next
// silence would exceed maxDuration, then cut at the last valid candidate.
func (sc *SilenceChunker) selectCutPoints(silences []silencePoint, bytesPerSecond float64) []time.Duration {
	if len(silences) == 0 {
		return nil
	}

	// Calculate max duration per chunk based on size limit.
	maxDuration := time.Duration(float64(sc.maxChunkSize) / bytesPerSecond * float64(time.Second))

	var cutPoints []time.Duration
	lastCut := time.Duration(0)
	var candidate *time.Duration // Last valid cut point before exceeding maxDuration

	for _, silence := range silences {
		mid := silence.midpoint()
		durationSinceCut := mid - lastCut

		if durationSinceCut < maxDuration {
			// This silence is a valid candidate (chunk would be under limit).
			candidate = &mid
		} else {
			// We've exceeded max duration.
			if candidate != nil {
				// Cut at the last valid candidate.
				cutPoints = append(cutPoints, *candidate)
				lastCut = *candidate
				candidate = nil
				// Re-evaluate current silence from new lastCut.
				if mid-lastCut < maxDuration {
					candidate = &mid
				}
			} else {
				// No valid candidate available, must cut here even though over limit.
				cutPoints = append(cutPoints, mid)
				lastCut = mid
			}
		}
	}

	return cutPoints
}

// extractChunks creates chunk files at the specified cut points.
// If extraction fails partway through, already-created chunk files are cleaned up.
func (sc *SilenceChunker) extractChunks(ctx context.Context, audioPath, tempDir string, cutPoints []time.Duration, totalDuration time.Duration) ([]Chunk, error) {
	// Build segment boundaries: [0, cut1, cut2, ..., totalDuration].
	boundaries := make([]time.Duration, 0, len(cutPoints)+2)
	boundaries = append(boundaries, 0)
	boundaries = append(boundaries, cutPoints...)
	boundaries = append(boundaries, totalDuration)

	chunks := make([]Chunk, 0, len(boundaries)-1)
	for i := range len(boundaries) - 1 {
		start := boundaries[i]
		end := boundaries[i+1]

		chunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_%03d.ogg", i))
		if err := sc.extractChunk(ctx, audioPath, chunkPath, start, end); err != nil {
			// Cleanup chunks already created before returning error.
			for _, c := range chunks {
				_ = os.Remove(c.Path)
			}
			return nil, err
		}

		chunks = append(chunks, Chunk{
			Path:      chunkPath,
			Index:     i,
			StartTime: start,
			EndTime:   end,
		})
	}

	return chunks, nil
}

// extractChunk extracts a segment from audioPath to chunkPath.
func (sc *SilenceChunker) extractChunk(ctx context.Context, audioPath, chunkPath string, start, end time.Duration) error {
	args := []string{
		"-y",
		"-i", audioPath,
		"-ss", formatFFmpegTime(start),
		"-to", formatFFmpegTime(end),
		"-c", "copy", // No re-encoding.
		chunkPath,
	}

	// #nosec G204 -- ffmpegPath is resolved internally via resolveFFmpeg, not user input
	cmd := exec.CommandContext(ctx, sc.ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: failed to extract chunk %s: %v\nOutput: %s",
			ErrChunkingFailed, chunkPath, err, string(output))
	}
	return nil
}

// CleanupChunks removes all chunk files and their parent directory.
// Call this after transcription is complete.
func CleanupChunks(chunks []Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	// All chunks should be in the same temp directory.
	tempDir := filepath.Dir(chunks[0].Path)

	// Verify it's a temp directory before removing.
	if !strings.Contains(tempDir, "go-transcript-") {
		// Safety check: don't delete arbitrary directories.
		// Fall back to removing individual files.
		for _, chunk := range chunks {
			_ = os.Remove(chunk.Path)
		}
		return nil
	}

	return os.RemoveAll(tempDir)
}
