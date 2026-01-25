package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// recordOptions holds the validated options for the record command.
type recordOptions struct {
	duration time.Duration
	output   string
	device   string
	loopback bool
	mix      bool
}

// recordCmd creates the record command.
func recordCmd() *cobra.Command {
	var (
		durationStr string
		output      string
		device      string
		loopback    bool
		mix         bool
	)

	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record audio from microphone or system audio",
		Long: `Record audio from microphone, system audio (loopback), or both mixed.

The output format is OGG Vorbis optimized for voice (~50kbps, 16kHz mono).
Recording can be interrupted with Ctrl+C to stop early - the file will be properly finalized.`,
		Example: `  transcript record -d 2h -o session.ogg           # Microphone only
  transcript record -d 30m --loopback              # System audio only
  transcript record -d 1h --mix -o meeting.ogg     # Mic + system audio`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse duration.
			duration, err := time.ParseDuration(durationStr)
			if err != nil {
				return fmt.Errorf("invalid duration %q: %w (use format like 2h, 30m, 1h30m)", durationStr, ErrInvalidDuration)
			}
			if duration <= 0 {
				return fmt.Errorf("duration must be positive: %w", ErrInvalidDuration)
			}

			// Note: output path resolution (including output-dir) is done in runRecord.
			opts := recordOptions{
				duration: duration,
				output:   output,
				device:   device,
				loopback: loopback,
				mix:      mix,
			}

			return runRecord(cmd.Context(), opts)
		},
	}

	// Flags.
	cmd.Flags().StringVarP(&durationStr, "duration", "d", "", "Recording duration (e.g., 2h, 30m, 1h30m)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (default: recording_<timestamp>.ogg)")
	cmd.Flags().StringVar(&device, "device", "", "Audio input device (default: system default)")
	cmd.Flags().BoolVar(&loopback, "loopback", false, "Capture system audio instead of microphone")
	cmd.Flags().BoolVar(&mix, "mix", false, "Capture both microphone and system audio")

	// Duration is required.
	_ = cmd.MarkFlagRequired("duration")

	// Loopback and mix are mutually exclusive.
	cmd.MarkFlagsMutuallyExclusive("loopback", "mix")

	return cmd
}

// runRecord executes the recording with the given options.
func runRecord(ctx context.Context, opts recordOptions) error {
	// Load config for output-dir.
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
	}

	// Resolve output path using config output-dir.
	opts.output = ResolveOutputPath(opts.output, cfg.OutputDir, defaultRecordingFilename())

	// Warn if output extension is not .ogg.
	ext := strings.ToLower(filepath.Ext(opts.output))
	if ext != "" && ext != ".ogg" {
		fmt.Fprintf(os.Stderr, "Warning: output will be OGG Vorbis format regardless of %s extension\n", ext)
	}

	// Check output file doesn't already exist.
	if _, err := os.Stat(opts.output); err == nil {
		return fmt.Errorf("output file already exists: %s: %w", opts.output, ErrOutputExists)
	}

	// Resolve FFmpeg.
	ffmpegPath, err := resolveFFmpeg(ctx)
	if err != nil {
		return err
	}

	// Check FFmpeg version (warning only).
	checkFFmpegVersion(ctx, ffmpegPath)

	// Create the appropriate recorder.
	recorder, err := createRecorder(ctx, ffmpegPath, opts.device, opts.loopback, opts.mix)
	if err != nil {
		return err
	}

	// Print start message.
	fmt.Fprintf(os.Stderr, "Recording for %s to %s... (press Ctrl+C to stop)\n", formatDurationHuman(opts.duration), opts.output)

	// Record.
	if err := recorder.Record(ctx, opts.duration, opts.output); err != nil {
		// Check if it was an interrupt - file may still be valid.
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "Interrupted, finalizing...")
		} else {
			return err
		}
	}

	// Print completion message with file size.
	size, err := fileSize(opts.output)
	if err != nil {
		// File might not exist if recording failed early.
		return fmt.Errorf("recording failed: output file not created")
	}

	fmt.Fprintf(os.Stderr, "Recording complete: %s (%s)\n", opts.output, formatSize(size))
	return nil
}

// createRecorder creates the appropriate recorder based on capture mode.
func createRecorder(ctx context.Context, ffmpegPath, device string, loopback, mix bool) (Recorder, error) {
	switch {
	case loopback:
		return NewFFmpegLoopbackRecorder(ctx, ffmpegPath)
	case mix:
		return NewFFmpegMixRecorder(ctx, ffmpegPath, device)
	default:
		return NewFFmpegRecorder(ffmpegPath, device)
	}
}

// defaultRecordingFilename generates a default output filename with timestamp.
// Format: recording_20260125_143052.ogg
func defaultRecordingFilename() string {
	return fmt.Sprintf("recording_%s.ogg", time.Now().Format("20060102_150405"))
}

// formatDurationHuman formats a duration for human display.
// Examples: "2h", "30m", "1h30m", "45s"
func formatDurationHuman(d time.Duration) string {
	if d >= time.Hour {
		hours := d / time.Hour
		minutes := (d % time.Hour) / time.Minute
		if minutes > 0 {
			return fmt.Sprintf("%dh%dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return fmt.Sprintf("%ds", d/time.Second)
}

// fileSize returns the size of a file in bytes.
func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// formatSize formats a size in bytes for human display.
// Uses MB for sizes >= 1MB, KB otherwise.
func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)

	if bytes >= mb {
		return fmt.Sprintf("%d MB", bytes/mb)
	}
	if bytes >= kb {
		return fmt.Sprintf("%d KB", bytes/kb)
	}
	return fmt.Sprintf("%d bytes", bytes)
}
