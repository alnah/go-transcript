package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alnah/go-transcript/internal/audio"
	"github.com/alnah/go-transcript/internal/config"
	"github.com/alnah/go-transcript/internal/lang"
	"github.com/alnah/go-transcript/internal/restructure"
	"github.com/alnah/go-transcript/internal/template"
	"github.com/alnah/go-transcript/internal/transcribe"
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

// supportedFormatsList returns a sorted, comma-separated list for error messages.
// The list is sorted for deterministic output in tests and user-facing messages.
func supportedFormatsList() string {
	formats := make([]string, 0, len(supportedFormats))
	for ext := range supportedFormats {
		formats = append(formats, strings.TrimPrefix(ext, "."))
	}
	slices.Sort(formats)
	return strings.Join(formats, ", ")
}

// clampParallel constrains parallel request count to valid range [1, MaxRecommendedParallel].
func clampParallel(n int) int {
	if n < 1 {
		return 1
	}
	if n > transcribe.MaxRecommendedParallel {
		return transcribe.MaxRecommendedParallel
	}
	return n
}

// deriveOutputPath converts an audio file path to a markdown output path.
// Example: "session.ogg" -> "session.md"
func deriveOutputPath(inputPath string) string {
	ext := filepath.Ext(inputPath)
	return strings.TrimSuffix(inputPath, ext) + ".md"
}

// TranscribeCmd creates the transcribe command.
// The env parameter provides injectable dependencies for testing.
func TranscribeCmd(env *Env) *cobra.Command {
	var (
		output     string
		tmpl       string
		diarize    bool
		parallel   int
		language   string
		outputLang string
		provider   string
	)

	cmd := &cobra.Command{
		Use:   "transcribe <audio-file>",
		Short: "Transcribe an audio file",
		Long: `Transcribe an audio file using OpenAI's transcription API.

The audio is split into chunks at natural silence points, transcribed in parallel,
and optionally restructured using a template.

Transcription always uses OpenAI. Restructuring (--template) uses OpenAI by default,
or DeepSeek with --provider deepseek.

Supported formats: ogg, mp3, wav, m4a, flac, mp4, mpeg, mpga, webm`,
		Example: `  transcript transcribe session.ogg -o notes.md -t brainstorm
  transcript transcribe meeting.ogg -t meeting --diarize
  transcript transcribe lecture.ogg -t lecture -l en
  transcript transcribe session.ogg -l fr --output-lang en -t meeting
  transcript transcribe session.ogg -t meeting --provider deepseek  # Use DeepSeek for restructuring
  transcript transcribe session.ogg  # Raw transcript, no restructuring`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTranscribe(cmd, env, args[0], output, tmpl, diarize, parallel, language, outputLang, provider)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (default: <input>.md)")
	cmd.Flags().StringVarP(&tmpl, "template", "t", "", "Restructure template: brainstorm, meeting, lecture")
	cmd.Flags().BoolVar(&diarize, "diarize", false, "Enable speaker identification")
	cmd.Flags().IntVarP(&parallel, "parallel", "p", transcribe.MaxRecommendedParallel, "Max concurrent API requests (1-10)")
	cmd.Flags().StringVarP(&language, "language", "l", "", "Audio language (ISO 639-1 code, e.g., en, fr, pt-BR)")
	cmd.Flags().StringVar(&outputLang, "output-lang", "", "Output language for restructured text (requires --template)")
	cmd.Flags().StringVar(&provider, "provider", ProviderOpenAI, "LLM provider for restructuring: openai, deepseek")

	return cmd
}

// runTranscribe executes the transcription pipeline.
// Validation order: file exists -> format -> output -> template -> language -> provider -> parallel -> API keys
func runTranscribe(cmd *cobra.Command, env *Env, inputPath, output, tmpl string, diarize bool, parallel int, language, outputLang, provider string) error {
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

	// 3. Load config for output-dir
	cfg, err := env.ConfigLoader.Load()
	if err != nil {
		fmt.Fprintf(env.Stderr, "Warning: failed to load config: %v\n", err)
	}

	// 4. Output path (resolve with output-dir, derive default from input if needed)
	defaultOutput := deriveOutputPath(filepath.Base(inputPath))
	output = config.ResolveOutputPath(output, cfg.OutputDir, defaultOutput)

	// 5. Template valid (if specified) - fail-fast before expensive operations
	if tmpl != "" {
		if _, err := template.Get(tmpl); err != nil {
			return err
		}
	}

	// 6. Language validation
	if err := lang.Validate(language); err != nil {
		return err
	}
	if err := lang.Validate(outputLang); err != nil {
		return err
	}

	// 7. Output language requires template
	if outputLang != "" && tmpl == "" {
		return fmt.Errorf("--output-lang requires --template (raw transcripts use the audio's language)")
	}

	// 8. Provider validation
	if provider != ProviderDeepSeek && provider != ProviderOpenAI {
		return ErrUnsupportedProvider
	}

	// 9. Parallel bounds (clamp to 1-10)
	parallel = clampParallel(parallel)

	// 10. API keys present (OpenAI always needed for transcription)
	openaiKey := env.Getenv(EnvOpenAIAPIKey)
	if openaiKey == "" {
		return fmt.Errorf("%w (set it with: export %s=sk-...)", ErrAPIKeyMissing, EnvOpenAIAPIKey)
	}

	// 11. Restructuring API key (only if template specified)
	var restructureAPIKey string
	if tmpl != "" {
		switch provider {
		case ProviderDeepSeek:
			restructureAPIKey = env.Getenv(EnvDeepSeekAPIKey)
			if restructureAPIKey == "" {
				return fmt.Errorf("%w (set it with: export %s=sk-...)", ErrDeepSeekKeyMissing, EnvDeepSeekAPIKey)
			}
		case ProviderOpenAI:
			restructureAPIKey = openaiKey // Reuse OpenAI key
		}
	}

	// === SETUP ===

	// Resolve FFmpeg (may auto-download)
	ffmpegPath, err := env.FFmpegResolver.Resolve(ctx)
	if err != nil {
		return err
	}
	env.FFmpegResolver.CheckVersion(ctx, ffmpegPath)

	// === CHUNKING ===

	fmt.Fprintln(env.Stderr, "Detecting silences...")

	chunker, err := env.ChunkerFactory.NewSilenceChunker(ffmpegPath)
	if err != nil {
		return err
	}

	chunks, err := chunker.Chunk(ctx, inputPath)
	if err != nil {
		return err
	}

	// Ensure cleanup even on error or interrupt
	defer func() {
		if cleanupErr := audio.CleanupChunks(chunks); cleanupErr != nil {
			fmt.Fprintf(env.Stderr, "Warning: failed to cleanup chunks: %v\n", cleanupErr)
		}
	}()

	fmt.Fprintf(env.Stderr, "Chunking audio... %d chunks\n", len(chunks))

	// === TRANSCRIPTION ===

	transcriber := env.TranscriberFactory.NewTranscriber(openaiKey)
	opts := transcribe.Options{
		Diarize:  diarize,
		Language: language,
	}

	// Transcribe with progress output
	fmt.Fprintln(env.Stderr, "Transcribing...")
	results, err := transcribe.TranscribeAll(ctx, chunks, transcriber, opts, parallel)
	if err != nil {
		return err
	}

	transcript := strings.Join(results, "\n\n")
	fmt.Fprintln(env.Stderr, "Transcription complete")

	// === RESTRUCTURE (optional) ===

	finalOutput := transcript
	if tmpl != "" && strings.TrimSpace(transcript) != "" {
		fmt.Fprintf(env.Stderr, "Restructuring with template '%s' (provider: %s)...\n", tmpl, provider)

		// Default output language to input language if not specified
		effectiveOutputLang := outputLang
		if effectiveOutputLang == "" && language != "" {
			effectiveOutputLang = language
		}

		mrRestructurer, err := env.RestructurerFactory.NewMapReducer(provider, restructureAPIKey,
			restructure.WithMapReduceProgress(func(phase string, current, total int) {
				if phase == "map" {
					fmt.Fprintf(env.Stderr, "  Processing part %d/%d...\n", current, total)
				} else {
					fmt.Fprintln(env.Stderr, "  Merging parts...")
				}
			}),
		)
		if err != nil {
			return err
		}

		finalOutput, _, err = mrRestructurer.Restructure(ctx, transcript, tmpl, effectiveOutputLang)
		if err != nil {
			return err
		}
	}

	// === WRITE OUTPUT ===

	// Use O_EXCL to atomically check existence and create file (avoids race condition)
	// #nosec G302 G304 -- user-specified output file with standard permissions
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("output file already exists: %s: %w", output, ErrOutputExists)
		}
		return fmt.Errorf("cannot create output file: %w", err)
	}

	// Ensure file is closed even on write error
	writeErr := func() error {
		defer func() { _ = f.Close() }()
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

	fmt.Fprintf(env.Stderr, "Done: %s\n", output)
	return nil
}
