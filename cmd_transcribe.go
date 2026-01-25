package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	"github.com/spf13/cobra"
)

// supportedFormats lists audio formats accepted by OpenAI's transcription API.
// Source: https://platform.openai.com/docs/guides/speech-to-text
var supportedFormats = map[string]bool{
	".ogg":  true,
	".mp3":  true,
	".wav":  true,
	".m4a":  true,
	".flac": true,
	".mp4":  true,
	".mpeg": true,
	".mpga": true,
	".webm": true,
}

// supportedFormatsList returns a comma-separated list for error messages.
func supportedFormatsList() string {
	formats := make([]string, 0, len(supportedFormats))
	for ext := range supportedFormats {
		formats = append(formats, strings.TrimPrefix(ext, "."))
	}
	return strings.Join(formats, ", ")
}

// clampParallel constrains parallel request count to valid range [1, 10].
func clampParallel(n int) int {
	if n < 1 {
		return 1
	}
	if n > 10 {
		return 10
	}
	return n
}

// deriveOutputPath converts an audio file path to a markdown output path.
// Example: "session.ogg" -> "session.md"
func deriveOutputPath(inputPath string) string {
	ext := filepath.Ext(inputPath)
	return strings.TrimSuffix(inputPath, ext) + ".md"
}

// transcribeCmd creates the transcribe command.
func transcribeCmd() *cobra.Command {
	var (
		output     string
		template   string
		diarize    bool
		parallel   int
		language   string
		outputLang string
	)

	cmd := &cobra.Command{
		Use:   "transcribe <audio-file>",
		Short: "Transcribe an audio file",
		Long: `Transcribe an audio file using OpenAI's transcription API.

The audio is split into chunks at natural silence points, transcribed in parallel,
and optionally restructured using a template.

Supported formats: ogg, mp3, wav, m4a, flac, mp4, mpeg, mpga, webm`,
		Example: `  transcript transcribe session.ogg -o notes.md -t brainstorm
  transcript transcribe meeting.ogg -t meeting --diarize
  transcript transcribe lecture.ogg -t lecture -l en
  transcript transcribe session.ogg -l fr --output-lang en -t meeting
  transcript transcribe session.ogg  # Raw transcript, no restructuring`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTranscribe(cmd, args[0], output, template, diarize, parallel, language, outputLang)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (default: <input>.md)")
	cmd.Flags().StringVarP(&template, "template", "t", "", "Restructure template: brainstorm, meeting, lecture")
	cmd.Flags().BoolVar(&diarize, "diarize", false, "Enable speaker identification")
	cmd.Flags().IntVarP(&parallel, "parallel", "p", 3, "Max concurrent API requests (1-10)")
	cmd.Flags().StringVarP(&language, "language", "l", "", "Audio language (ISO 639-1 code, e.g., en, fr, pt-BR)")
	cmd.Flags().StringVar(&outputLang, "output-lang", "", "Output language for restructured text (requires --template)")

	return cmd
}

// runTranscribe executes the transcription pipeline.
// Validation order: file exists -> format -> output -> template -> language -> parallel -> API key
func runTranscribe(cmd *cobra.Command, inputPath, output, template string, diarize bool, parallel int, language, outputLang string) error {
	ctx := cmd.Context()

	// === VALIDATION (fail-fast) ===

	// 1. File exists
	if _, err := os.Stat(inputPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrFileNotFound, inputPath)
		}
		return fmt.Errorf("cannot access input file: %w", err)
	}

	// 2. Format supported
	ext := strings.ToLower(filepath.Ext(inputPath))
	if !supportedFormats[ext] {
		return fmt.Errorf("unsupported format %q (supported: %s): %w",
			ext, supportedFormatsList(), ErrUnsupportedFormat)
	}

	// 3. Output path (derive if not specified, check deferred to write with O_EXCL)
	if output == "" {
		output = deriveOutputPath(inputPath)
	}

	// 4. Template valid (if specified) - fail-fast before expensive operations
	if template != "" {
		if _, err := GetTemplate(template); err != nil {
			return err
		}
	}

	// 5. Language validation
	if err := ValidateLanguage(language); err != nil {
		return err
	}
	if err := ValidateLanguage(outputLang); err != nil {
		return err
	}

	// 6. Output language requires template
	if outputLang != "" && template == "" {
		return fmt.Errorf("--output-lang requires --template (raw transcripts use the audio's language)")
	}

	// 7. Parallel bounds (clamp to 1-10)
	parallel = clampParallel(parallel)

	// 8. API key present
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("%w (set it with: export OPENAI_API_KEY=sk-...)", ErrAPIKeyMissing)
	}

	// === SETUP ===

	// Resolve FFmpeg (may auto-download)
	ffmpegPath, err := resolveFFmpeg(ctx)
	if err != nil {
		return err
	}
	checkFFmpegVersion(ctx, ffmpegPath)

	// === CHUNKING ===

	fmt.Fprintln(os.Stderr, "Detecting silences...")

	chunker, err := NewSilenceChunker(ffmpegPath)
	if err != nil {
		return err
	}

	chunks, err := chunker.Chunk(ctx, inputPath)
	if err != nil {
		return err
	}

	// Ensure cleanup even on error or interrupt
	defer func() {
		if cleanupErr := CleanupChunks(chunks); cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to cleanup chunks: %v\n", cleanupErr)
		}
	}()

	fmt.Fprintf(os.Stderr, "Chunking audio... %d chunks\n", len(chunks))

	// === TRANSCRIPTION ===

	client := openai.NewClient(apiKey)
	transcriber := NewOpenAITranscriber(client)
	opts := TranscribeOptions{
		Diarize:  diarize,
		Language: language,
	}

	// Transcribe with progress output
	fmt.Fprintln(os.Stderr, "Transcribing...")
	results, err := TranscribeAll(ctx, chunks, transcriber, opts, parallel)
	if err != nil {
		return err
	}

	transcript := strings.Join(results, "\n\n")
	fmt.Fprintln(os.Stderr, "Transcription complete")

	// === RESTRUCTURE (optional) ===

	finalOutput := transcript
	if template != "" {
		fmt.Fprintf(os.Stderr, "Restructuring with template '%s'...\n", template)

		// Default output language to input language if not specified
		effectiveOutputLang := outputLang
		if effectiveOutputLang == "" && language != "" {
			effectiveOutputLang = language
		}

		restructurer := NewOpenAIRestructurer(client)
		finalOutput, err = restructurer.Restructure(ctx, transcript, template, effectiveOutputLang)
		if err != nil {
			return err
		}
	}

	// === WRITE OUTPUT ===

	// Use O_EXCL to atomically check existence and create file (avoids race condition)
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("output file already exists: %s: %w", output, ErrOutputExists)
		}
		return fmt.Errorf("cannot create output file: %w", err)
	}

	// Ensure file is closed even on write error
	writeErr := func() error {
		defer f.Close()
		if _, err := f.WriteString(finalOutput); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
		return nil
	}()

	if writeErr != nil {
		// Attempt to remove partial file on write failure
		_ = os.Remove(output)
		return writeErr
	}

	fmt.Fprintf(os.Stderr, "Done: %s\n", output)
	return nil
}
