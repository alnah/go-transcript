package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/alnah/go-transcript/internal/audio"
	"github.com/alnah/go-transcript/internal/config"
	"github.com/alnah/go-transcript/internal/lang"
	"github.com/alnah/go-transcript/internal/template"
	"github.com/alnah/go-transcript/internal/transcribe"
)

// Notes:
// - Tests focus on observable behavior through public APIs (runTranscribe, TranscribeCmd)
// - File I/O and format validation happen in runTranscribe (runtime checks)
// - The mockRestructurerFactory from mocks_test.go is reused for consistency

// ---------------------------------------------------------------------------
// Unit tests for helper functions
// ---------------------------------------------------------------------------

func TestClampParallel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{"negative", -5, 1},
		{"zero", 0, 1},
		{"one", 1, 1},
		{"middle", 5, 5},
		{"max", transcribe.MaxRecommendedParallel, transcribe.MaxRecommendedParallel},
		{"over_max", 100, transcribe.MaxRecommendedParallel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ClampParallel(tt.input)
			if result != tt.expected {
				t.Errorf("ClampParallel(%d) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDeriveOutputPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"ogg_to_md", "session.ogg", "session.md"},
		{"mp3_to_md", "meeting.mp3", "meeting.md"},
		{"no_extension", "audio", "audio.md"},
		{"double_extension", "file.backup.ogg", "file.backup.md"},
		{"path_with_dir", "/home/user/audio.ogg", "/home/user/audio.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := DeriveOutputPath(tt.input)
			if result != tt.expected {
				t.Errorf("DeriveOutputPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSupportedFormatsList(t *testing.T) {
	t.Parallel()

	result := SupportedFormatsList()

	// Should contain common formats
	for _, format := range []string{"ogg", "mp3", "wav", "m4a", "flac"} {
		if !strings.Contains(result, format) {
			t.Errorf("expected %q in supported formats list, got %q", format, result)
		}
	}

	// Should be comma-separated
	if !strings.Contains(result, ", ") {
		t.Errorf("expected comma-separated list, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// Tests for ParseTranscribeOptions - CLI input parsing and validation
// ---------------------------------------------------------------------------

func TestParseTranscribeOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		inputPath  string
		output     string
		tmpl       string
		diarize    bool
		parallel   int
		language   string
		outputLang string
		provider   string
		wantErr    bool
		errContain string
	}{
		{
			name:      "valid minimal options",
			inputPath: "/path/to/file.ogg",
			parallel:  5,
			provider:  "deepseek",
			wantErr:   false,
		},
		{
			name:       "valid with all options",
			inputPath:  "/path/to/file.ogg",
			output:     "/output/file.md",
			tmpl:       "meeting",
			diarize:    true,
			parallel:   3,
			language:   "fr",
			outputLang: "en",
			provider:   "openai",
			wantErr:    false,
		},
		{
			name:       "invalid template",
			inputPath:  "/path/to/file.ogg",
			tmpl:       "nonexistent-template",
			parallel:   5,
			provider:   "deepseek",
			wantErr:    true,
			errContain: "unknown",
		},
		{
			name:      "invalid language",
			inputPath: "/path/to/file.ogg",
			parallel:  5,
			language:  "invalid-lang-code-too-long",
			provider:  "deepseek",
			wantErr:   true,
		},
		{
			name:       "invalid output language",
			inputPath:  "/path/to/file.ogg",
			tmpl:       "brainstorm",
			parallel:   5,
			outputLang: "invalid-output-lang",
			provider:   "deepseek",
			wantErr:    true,
		},
		{
			name:       "invalid provider",
			inputPath:  "/path/to/file.ogg",
			parallel:   5,
			provider:   "invalid-provider",
			wantErr:    true,
			errContain: "invalid provider",
		},
		{
			name:      "empty provider uses default",
			inputPath: "/path/to/file.ogg",
			parallel:  5,
			provider:  "",
			wantErr:   false, // Empty provider is allowed - defaults to DeepSeek
		},
		{
			name:      "no template is valid",
			inputPath: "/path/to/file.ogg",
			parallel:  5,
			provider:  "deepseek",
			wantErr:   false, // Raw transcript mode
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseTranscribeOptions(tt.inputPath, tt.output, tt.tmpl, tt.diarize, tt.parallel, tt.language, tt.outputLang, tt.provider)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTranscribeOptions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
				t.Errorf("ParseTranscribeOptions() error = %v, want error containing %q", err, tt.errContain)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests for runTranscribe
// ---------------------------------------------------------------------------

// createTranscribeCmd creates a cobra.Command for testing runTranscribe.
// This is needed because runTranscribe expects a *cobra.Command for context.
func createTranscribeCmd(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	return cmd
}

// mustParseTranscribeOptions is a test helper that parses options or fails the test.
func mustParseTranscribeOptions(t *testing.T, inputPath, output, tmpl string, diarize bool, parallel int, language, outputLang, provider string) TranscribeOptions {
	t.Helper()
	opts, err := ParseTranscribeOptions(inputPath, output, tmpl, diarize, parallel, language, outputLang, provider)
	if err != nil {
		t.Fatalf("ParseTranscribeOptions failed: %v", err)
	}
	return opts
}

func TestRunTranscribe_FileNotFound(t *testing.T) {
	t.Parallel()

	env, _ := testEnv()
	cmd := createTranscribeCmd(context.Background())

	opts := mustParseTranscribeOptions(t, "/nonexistent/file.ogg", "", "", false, 5, "", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !errors.Is(err, ErrFileNotFound) {
		t.Errorf("expected ErrFileNotFound, got %v", err)
	}
}

func TestRunTranscribe_UnsupportedFormat(t *testing.T) {
	t.Parallel()

	// Create a file with unsupported extension
	inputPath := createTestAudioFile(t, "audio.txt")

	env, _ := testEnv()
	cmd := createTranscribeCmd(context.Background())

	opts := mustParseTranscribeOptions(t, inputPath, "", "", false, 5, "", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestRunTranscribe_OutputLangRequiresTemplate(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")

	env, _ := testEnv()
	cmd := createTranscribeCmd(context.Background())

	// Parse options with output language but no template
	// Note: ParseTranscribeOptions doesn't validate this - runTranscribe does
	opts := mustParseTranscribeOptions(t, inputPath, "", "", false, 5, "", "en", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err == nil {
		t.Fatal("expected error when --translate without template")
	}
	if !strings.Contains(err.Error(), "translate") || !strings.Contains(err.Error(), "template") {
		t.Errorf("expected translate/template error, got %v", err)
	}
}

func TestRunTranscribe_MissingAPIKey(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")

	stderr := &syncBuffer{}
	env := &Env{
		Stderr:         stderr,
		Getenv:         func(string) string { return "" }, // No API key
		Now:            fixedTime(time.Now()),
		FFmpegResolver: &mockFFmpegResolver{},
		ConfigLoader:   &mockConfigLoader{},
	}
	cmd := createTranscribeCmd(context.Background())

	opts := mustParseTranscribeOptions(t, inputPath, "", "", false, 5, "", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	if !errors.Is(err, ErrAPIKeyMissing) {
		t.Errorf("expected ErrAPIKeyMissing, got %v", err)
	}
}

func TestRunTranscribe_FFmpegResolveFails(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")

	ffmpegErr := errors.New("ffmpeg not found")
	ffmpegResolver := &mockFFmpegResolver{
		ResolveFunc: func(ctx context.Context) (string, error) {
			return "", ffmpegErr
		},
	}

	env := &Env{
		Stderr:         &syncBuffer{},
		Getenv:         defaultTestEnv,
		Now:            fixedTime(time.Now()),
		FFmpegResolver: ffmpegResolver,
		ConfigLoader:   &mockConfigLoader{},
	}
	cmd := createTranscribeCmd(context.Background())

	opts := mustParseTranscribeOptions(t, inputPath, "", "", false, 5, "", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err == nil {
		t.Fatal("expected error when ffmpeg not found")
	}
	if !errors.Is(err, ffmpegErr) {
		t.Errorf("expected ffmpeg error, got %v", err)
	}
}

func TestRunTranscribe_ChunkerFails(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

	chunkerErr := errors.New("chunker failed")
	chunker := &mockChunker{
		ChunkFunc: func(ctx context.Context, audioPath string) ([]audio.Chunk, error) {
			return nil, chunkerErr
		},
	}
	chunkerFactory := &mockChunkerFactory{
		NewSilenceChunkerFunc: func(ffmpegPath string) (audio.Chunker, error) {
			return chunker, nil
		},
	}

	env := &Env{
		Stderr:         &syncBuffer{},
		Getenv:         defaultTestEnv,
		Now:            fixedTime(time.Now()),
		FFmpegResolver: &mockFFmpegResolver{},
		ConfigLoader:   &mockConfigLoader{},
		ChunkerFactory: chunkerFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	opts := mustParseTranscribeOptions(t, inputPath, outputPath, "", false, 5, "", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err == nil {
		t.Fatal("expected error when chunker fails")
	}
	if !errors.Is(err, chunkerErr) {
		t.Errorf("expected chunker error, got %v", err)
	}
}

func TestRunTranscribe_Success(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")
	stderr := &syncBuffer{}

	// Create mock chunker that returns chunk paths
	chunkDir := t.TempDir()
	chunkPath := filepath.Join(chunkDir, "chunk_0.ogg")
	if err := os.WriteFile(chunkPath, []byte("chunk audio"), 0644); err != nil {
		t.Fatalf("failed to create chunk file: %v", err)
	}

	chunker := &mockChunker{
		ChunkFunc: func(ctx context.Context, audioPath string) ([]audio.Chunk, error) {
			return []audio.Chunk{
				{Path: chunkPath, Index: 0, StartTime: 0, EndTime: 5 * time.Minute},
			}, nil
		},
	}
	chunkerFactory := &mockChunkerFactory{
		NewSilenceChunkerFunc: func(ffmpegPath string) (audio.Chunker, error) {
			return chunker, nil
		},
	}

	transcriber := &mockTranscriber{
		TranscribeFunc: func(ctx context.Context, audioPath string, opts transcribe.Options) (string, error) {
			return "This is the transcribed text.", nil
		},
	}
	transcriberFactory := &mockTranscriberFactory{
		NewTranscriberFunc: func(apiKey string) transcribe.Transcriber {
			return transcriber
		},
	}

	env := &Env{
		Stderr:             stderr,
		Getenv:             defaultTestEnv,
		Now:                fixedTime(time.Now()),
		FFmpegResolver:     &mockFFmpegResolver{},
		ConfigLoader:       &mockConfigLoader{},
		ChunkerFactory:     chunkerFactory,
		TranscriberFactory: transcriberFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	opts := mustParseTranscribeOptions(t, inputPath, outputPath, "", false, 5, "", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify output file was created
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if string(content) != "This is the transcribed text." {
		t.Errorf("expected transcribed text, got %q", string(content))
	}

	// Verify stderr contains progress messages
	output := stderr.String()
	if !strings.Contains(output, "Detecting silences") {
		t.Errorf("expected 'Detecting silences' in output, got %q", output)
	}
	if !strings.Contains(output, "Transcribing") {
		t.Errorf("expected 'Transcribing' in output, got %q", output)
	}
	if !strings.Contains(output, "Done") {
		t.Errorf("expected 'Done' in output, got %q", output)
	}
}

func TestRunTranscribe_OutputExists(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "existing.md")

	// Create existing output file
	if err := os.WriteFile(outputPath, []byte("existing content"), 0644); err != nil {
		t.Fatalf("failed to create existing file: %v", err)
	}

	// Setup all mocks to allow reaching the output check
	chunkDir := t.TempDir()
	chunkPath := filepath.Join(chunkDir, "chunk_0.ogg")
	if err := os.WriteFile(chunkPath, []byte("chunk"), 0644); err != nil {
		t.Fatalf("failed to create chunk: %v", err)
	}

	chunker := &mockChunker{
		ChunkFunc: func(ctx context.Context, audioPath string) ([]audio.Chunk, error) {
			return []audio.Chunk{{Path: chunkPath, Index: 0}}, nil
		},
	}
	chunkerFactory := &mockChunkerFactory{
		NewSilenceChunkerFunc: func(ffmpegPath string) (audio.Chunker, error) {
			return chunker, nil
		},
	}

	transcriber := &mockTranscriber{
		TranscribeFunc: func(ctx context.Context, audioPath string, opts transcribe.Options) (string, error) {
			return "text", nil
		},
	}
	transcriberFactory := &mockTranscriberFactory{
		NewTranscriberFunc: func(apiKey string) transcribe.Transcriber {
			return transcriber
		},
	}

	env := &Env{
		Stderr:             &syncBuffer{},
		Getenv:             defaultTestEnv,
		Now:                fixedTime(time.Now()),
		FFmpegResolver:     &mockFFmpegResolver{},
		ConfigLoader:       &mockConfigLoader{},
		ChunkerFactory:     chunkerFactory,
		TranscriberFactory: transcriberFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	opts := mustParseTranscribeOptions(t, inputPath, outputPath, "", false, 5, "", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err == nil {
		t.Fatal("expected error for existing output file")
	}
	if !errors.Is(err, ErrOutputExists) {
		t.Errorf("expected ErrOutputExists, got %v", err)
	}
}

func TestRunTranscribe_WithLanguage(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

	chunkDir := t.TempDir()
	chunkPath := filepath.Join(chunkDir, "chunk_0.ogg")
	if err := os.WriteFile(chunkPath, []byte("chunk"), 0644); err != nil {
		t.Fatalf("failed to create chunk: %v", err)
	}

	chunker := &mockChunker{
		ChunkFunc: func(ctx context.Context, audioPath string) ([]audio.Chunk, error) {
			return []audio.Chunk{{Path: chunkPath, Index: 0}}, nil
		},
	}
	chunkerFactory := &mockChunkerFactory{
		NewSilenceChunkerFunc: func(ffmpegPath string) (audio.Chunker, error) {
			return chunker, nil
		},
	}

	var capturedOpts transcribe.Options
	transcriber := &mockTranscriber{
		TranscribeFunc: func(ctx context.Context, audioPath string, opts transcribe.Options) (string, error) {
			capturedOpts = opts
			return "text", nil
		},
	}
	transcriberFactory := &mockTranscriberFactory{
		NewTranscriberFunc: func(apiKey string) transcribe.Transcriber {
			return transcriber
		},
	}

	env := &Env{
		Stderr:             &syncBuffer{},
		Getenv:             defaultTestEnv,
		Now:                fixedTime(time.Now()),
		FFmpegResolver:     &mockFFmpegResolver{},
		ConfigLoader:       &mockConfigLoader{},
		ChunkerFactory:     chunkerFactory,
		TranscriberFactory: transcriberFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	opts := mustParseTranscribeOptions(t, inputPath, outputPath, "", false, 5, "fr", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if capturedOpts.Language.String() != "fr" {
		t.Errorf("expected language 'fr', got %q", capturedOpts.Language.String())
	}
}

func TestRunTranscribe_WithDiarize(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

	chunkDir := t.TempDir()
	chunkPath := filepath.Join(chunkDir, "chunk_0.ogg")
	if err := os.WriteFile(chunkPath, []byte("chunk"), 0644); err != nil {
		t.Fatalf("failed to create chunk: %v", err)
	}

	chunker := &mockChunker{
		ChunkFunc: func(ctx context.Context, audioPath string) ([]audio.Chunk, error) {
			return []audio.Chunk{{Path: chunkPath, Index: 0}}, nil
		},
	}
	chunkerFactory := &mockChunkerFactory{
		NewSilenceChunkerFunc: func(ffmpegPath string) (audio.Chunker, error) {
			return chunker, nil
		},
	}

	var capturedOpts transcribe.Options
	transcriber := &mockTranscriber{
		TranscribeFunc: func(ctx context.Context, audioPath string, opts transcribe.Options) (string, error) {
			capturedOpts = opts
			return "text", nil
		},
	}
	transcriberFactory := &mockTranscriberFactory{
		NewTranscriberFunc: func(apiKey string) transcribe.Transcriber {
			return transcriber
		},
	}

	env := &Env{
		Stderr:             &syncBuffer{},
		Getenv:             defaultTestEnv,
		Now:                fixedTime(time.Now()),
		FFmpegResolver:     &mockFFmpegResolver{},
		ConfigLoader:       &mockConfigLoader{},
		ChunkerFactory:     chunkerFactory,
		TranscriberFactory: transcriberFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	opts := mustParseTranscribeOptions(t, inputPath, outputPath, "", true, 5, "", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !capturedOpts.Diarize {
		t.Error("expected diarize to be true")
	}
}

func TestRunTranscribe_DefaultOutputPath(t *testing.T) {
	t.Parallel()

	inputDir := t.TempDir()
	inputPath := filepath.Join(inputDir, "meeting.ogg")
	if err := os.WriteFile(inputPath, []byte("audio"), 0644); err != nil {
		t.Fatalf("failed to create input file: %v", err)
	}

	outputDir := t.TempDir()

	chunkPath := filepath.Join(t.TempDir(), "chunk_0.ogg")
	if err := os.WriteFile(chunkPath, []byte("chunk"), 0644); err != nil {
		t.Fatalf("failed to create chunk: %v", err)
	}

	chunker := &mockChunker{
		ChunkFunc: func(ctx context.Context, audioPath string) ([]audio.Chunk, error) {
			return []audio.Chunk{{Path: chunkPath, Index: 0}}, nil
		},
	}
	chunkerFactory := &mockChunkerFactory{
		NewSilenceChunkerFunc: func(ffmpegPath string) (audio.Chunker, error) {
			return chunker, nil
		},
	}

	transcriber := &mockTranscriber{
		TranscribeFunc: func(ctx context.Context, audioPath string, opts transcribe.Options) (string, error) {
			return "text", nil
		},
	}
	transcriberFactory := &mockTranscriberFactory{
		NewTranscriberFunc: func(apiKey string) transcribe.Transcriber {
			return transcriber
		},
	}

	configLoader := &mockConfigLoader{
		LoadFunc: func() (config.Config, error) {
			return config.Config{OutputDir: outputDir}, nil
		},
	}

	env := &Env{
		Stderr:             &syncBuffer{},
		Getenv:             defaultTestEnv,
		Now:                fixedTime(time.Now()),
		FFmpegResolver:     &mockFFmpegResolver{},
		ConfigLoader:       configLoader,
		ChunkerFactory:     chunkerFactory,
		TranscriberFactory: transcriberFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	// Empty output path - should use default derived from input
	opts := mustParseTranscribeOptions(t, inputPath, "", "", false, 5, "", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify output was created with expected name
	expectedPath := filepath.Join(outputDir, "meeting.md")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected output file at %s", expectedPath)
	}
}

// ---------------------------------------------------------------------------
// Tests for TranscribeCmd (Cobra integration)
// ---------------------------------------------------------------------------

func TestTranscribeCmd_RequiresFile(t *testing.T) {
	t.Parallel()

	env, _ := testEnv()
	cmd := TranscribeCmd(env)

	cmd.SetArgs([]string{})
	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected error when file not provided")
	}
}

func TestTranscribeCmd_ValidatesFormat(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.xyz")

	env, _ := testEnv()
	cmd := TranscribeCmd(env)

	cmd.SetArgs([]string{inputPath})
	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("expected ErrUnsupportedFormat, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests for restructuring path
// ---------------------------------------------------------------------------

func TestRunTranscribe_WithTemplate(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")
	stderr := &syncBuffer{}

	// Setup chunks
	chunkDir := t.TempDir()
	chunkPath := filepath.Join(chunkDir, "chunk_0.ogg")
	if err := os.WriteFile(chunkPath, []byte("chunk audio"), 0644); err != nil {
		t.Fatalf("failed to create chunk file: %v", err)
	}

	chunker := &mockChunker{
		ChunkFunc: func(ctx context.Context, audioPath string) ([]audio.Chunk, error) {
			return []audio.Chunk{
				{Path: chunkPath, Index: 0, StartTime: 0, EndTime: 5 * time.Minute},
			}, nil
		},
	}
	chunkerFactory := &mockChunkerFactory{
		NewSilenceChunkerFunc: func(ffmpegPath string) (audio.Chunker, error) {
			return chunker, nil
		},
	}

	transcriber := &mockTranscriber{
		TranscribeFunc: func(ctx context.Context, audioPath string, opts transcribe.Options) (string, error) {
			return "Raw transcript content here.", nil
		},
	}
	transcriberFactory := &mockTranscriberFactory{
		NewTranscriberFunc: func(apiKey string) transcribe.Transcriber {
			return transcriber
		},
	}

	// Track restructurer calls
	var capturedTemplate template.Name
	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript string, tmpl template.Name, outputLang lang.Language) (string, bool, error) {
			capturedTemplate = tmpl
			return "# Restructured Output\n\nKey ideas here.", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              stderr,
		Getenv:              defaultTestEnv,
		Now:                 fixedTime(time.Now()),
		FFmpegResolver:      &mockFFmpegResolver{},
		ConfigLoader:        &mockConfigLoader{},
		ChunkerFactory:      chunkerFactory,
		TranscriberFactory:  transcriberFactory,
		RestructurerFactory: restructurerFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	opts := mustParseTranscribeOptions(t, inputPath, outputPath, "brainstorm", false, 5, "", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify restructurer was called with correct template
	if capturedTemplate.String() != "brainstorm" {
		t.Errorf("expected template 'brainstorm', got %q", capturedTemplate)
	}

	// Verify output file contains restructured content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if !strings.Contains(string(content), "Restructured Output") {
		t.Errorf("expected restructured content, got %q", string(content))
	}

	// Verify stderr contains restructuring message
	output := stderr.String()
	if !strings.Contains(output, "Restructuring") {
		t.Errorf("expected 'Restructuring' in output, got %q", output)
	}
}

func TestRunTranscribe_WithTemplateAndLanguages(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

	chunkPath := filepath.Join(t.TempDir(), "chunk_0.ogg")
	if err := os.WriteFile(chunkPath, []byte("chunk"), 0644); err != nil {
		t.Fatalf("failed to create chunk: %v", err)
	}

	chunker := &mockChunker{
		ChunkFunc: func(ctx context.Context, audioPath string) ([]audio.Chunk, error) {
			return []audio.Chunk{{Path: chunkPath, Index: 0}}, nil
		},
	}
	chunkerFactory := &mockChunkerFactory{
		NewSilenceChunkerFunc: func(ffmpegPath string) (audio.Chunker, error) {
			return chunker, nil
		},
	}

	transcriber := &mockTranscriber{
		TranscribeFunc: func(ctx context.Context, audioPath string, opts transcribe.Options) (string, error) {
			return "transcribed", nil
		},
	}
	transcriberFactory := &mockTranscriberFactory{
		NewTranscriberFunc: func(apiKey string) transcribe.Transcriber {
			return transcriber
		},
	}

	var capturedLang lang.Language
	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript string, tmpl template.Name, outputLang lang.Language) (string, bool, error) {
			capturedLang = outputLang
			return "restructured", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		Now:                 fixedTime(time.Now()),
		FFmpegResolver:      &mockFFmpegResolver{},
		ConfigLoader:        &mockConfigLoader{},
		ChunkerFactory:      chunkerFactory,
		TranscriberFactory:  transcriberFactory,
		RestructurerFactory: restructurerFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	// Test: input language fr, output language en
	opts := mustParseTranscribeOptions(t, inputPath, outputPath, "meeting", false, 5, "fr", "en", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Output language should be "en" (explicitly specified)
	if capturedLang.String() != "en" {
		t.Errorf("expected output lang 'en', got %q", capturedLang.String())
	}
}

func TestRunTranscribe_WithTemplateInheritLanguage(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

	chunkPath := filepath.Join(t.TempDir(), "chunk_0.ogg")
	if err := os.WriteFile(chunkPath, []byte("chunk"), 0644); err != nil {
		t.Fatalf("failed to create chunk: %v", err)
	}

	chunker := &mockChunker{
		ChunkFunc: func(ctx context.Context, audioPath string) ([]audio.Chunk, error) {
			return []audio.Chunk{{Path: chunkPath, Index: 0}}, nil
		},
	}
	chunkerFactory := &mockChunkerFactory{
		NewSilenceChunkerFunc: func(ffmpegPath string) (audio.Chunker, error) {
			return chunker, nil
		},
	}

	transcriber := &mockTranscriber{
		TranscribeFunc: func(ctx context.Context, audioPath string, opts transcribe.Options) (string, error) {
			return "transcribed", nil
		},
	}
	transcriberFactory := &mockTranscriberFactory{
		NewTranscriberFunc: func(apiKey string) transcribe.Transcriber {
			return transcriber
		},
	}

	var capturedLang lang.Language
	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript string, tmpl template.Name, outputLang lang.Language) (string, bool, error) {
			capturedLang = outputLang
			return "restructured", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		Now:                 fixedTime(time.Now()),
		FFmpegResolver:      &mockFFmpegResolver{},
		ConfigLoader:        &mockConfigLoader{},
		ChunkerFactory:      chunkerFactory,
		TranscriberFactory:  transcriberFactory,
		RestructurerFactory: restructurerFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	// Test: input language fr, no output language -> should inherit fr
	opts := mustParseTranscribeOptions(t, inputPath, outputPath, "meeting", false, 5, "fr", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Output language should be "fr" (inherited from input)
	if capturedLang.String() != "fr" {
		t.Errorf("expected output lang 'fr' (inherited), got %q", capturedLang.String())
	}
}

func TestRunTranscribe_RestructureError(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

	chunkPath := filepath.Join(t.TempDir(), "chunk_0.ogg")
	if err := os.WriteFile(chunkPath, []byte("chunk"), 0644); err != nil {
		t.Fatalf("failed to create chunk: %v", err)
	}

	chunker := &mockChunker{
		ChunkFunc: func(ctx context.Context, audioPath string) ([]audio.Chunk, error) {
			return []audio.Chunk{{Path: chunkPath, Index: 0}}, nil
		},
	}
	chunkerFactory := &mockChunkerFactory{
		NewSilenceChunkerFunc: func(ffmpegPath string) (audio.Chunker, error) {
			return chunker, nil
		},
	}

	transcriber := &mockTranscriber{
		TranscribeFunc: func(ctx context.Context, audioPath string, opts transcribe.Options) (string, error) {
			return "transcribed", nil
		},
	}
	transcriberFactory := &mockTranscriberFactory{
		NewTranscriberFunc: func(apiKey string) transcribe.Transcriber {
			return transcriber
		},
	}

	restructureErr := errors.New("API error during restructuring")
	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript string, tmpl template.Name, outputLang lang.Language) (string, bool, error) {
			return "", false, restructureErr
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		Now:                 fixedTime(time.Now()),
		FFmpegResolver:      &mockFFmpegResolver{},
		ConfigLoader:        &mockConfigLoader{},
		ChunkerFactory:      chunkerFactory,
		TranscriberFactory:  transcriberFactory,
		RestructurerFactory: restructurerFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	opts := mustParseTranscribeOptions(t, inputPath, outputPath, "brainstorm", false, 5, "", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err == nil {
		t.Fatal("expected error when restructuring fails")
	}
	if !errors.Is(err, restructureErr) {
		t.Errorf("expected restructure error, got %v", err)
	}
}

func TestRunTranscribe_EmptyTranscriptSkipsRestructure(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

	chunkPath := filepath.Join(t.TempDir(), "chunk_0.ogg")
	if err := os.WriteFile(chunkPath, []byte("chunk"), 0644); err != nil {
		t.Fatalf("failed to create chunk: %v", err)
	}

	chunker := &mockChunker{
		ChunkFunc: func(ctx context.Context, audioPath string) ([]audio.Chunk, error) {
			return []audio.Chunk{{Path: chunkPath, Index: 0}}, nil
		},
	}
	chunkerFactory := &mockChunkerFactory{
		NewSilenceChunkerFunc: func(ffmpegPath string) (audio.Chunker, error) {
			return chunker, nil
		},
	}

	// Transcriber returns empty/whitespace
	transcriber := &mockTranscriber{
		TranscribeFunc: func(ctx context.Context, audioPath string, opts transcribe.Options) (string, error) {
			return "   ", nil // Whitespace only
		},
	}
	transcriberFactory := &mockTranscriberFactory{
		NewTranscriberFunc: func(apiKey string) transcribe.Transcriber {
			return transcriber
		},
	}

	var restructureCalled bool
	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript string, tmpl template.Name, outputLang lang.Language) (string, bool, error) {
			restructureCalled = true
			return "should not be called", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		Now:                 fixedTime(time.Now()),
		FFmpegResolver:      &mockFFmpegResolver{},
		ConfigLoader:        &mockConfigLoader{},
		ChunkerFactory:      chunkerFactory,
		TranscriberFactory:  transcriberFactory,
		RestructurerFactory: restructurerFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	opts := mustParseTranscribeOptions(t, inputPath, outputPath, "brainstorm", false, 5, "", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Restructure should NOT be called for empty transcript
	if restructureCalled {
		t.Error("restructure should not be called for empty/whitespace transcript")
	}
}

// ---------------------------------------------------------------------------
// Tests for validation order in runTranscribe
// ---------------------------------------------------------------------------

func TestRunTranscribe_ValidationOrder(t *testing.T) {
	t.Parallel()

	t.Run("file_not_found_first", func(t *testing.T) {
		t.Parallel()

		env, _ := testEnv()
		cmd := createTranscribeCmd(context.Background())

		opts := mustParseTranscribeOptions(t, "/nonexistent/path.ogg", "", "", false, 5, "", "", "deepseek")
		err := RunTranscribe(cmd, env, opts)
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrFileNotFound) {
			t.Errorf("expected ErrFileNotFound, got %v", err)
		}
	})

	t.Run("format_check_before_api_key", func(t *testing.T) {
		t.Parallel()

		// Create file with bad extension
		path := createTestAudioFile(t, "audio.xyz")
		env := &Env{
			Stderr:         &syncBuffer{},
			Getenv:         func(string) string { return "" }, // No API key
			Now:            fixedTime(time.Now()),
			FFmpegResolver: &mockFFmpegResolver{},
			ConfigLoader:   &mockConfigLoader{},
		}
		cmd := createTranscribeCmd(context.Background())

		opts := mustParseTranscribeOptions(t, path, "", "", false, 5, "", "", "deepseek")
		err := RunTranscribe(cmd, env, opts)
		if err == nil {
			t.Fatal("expected error")
		}
		// Format error should come before API key error
		if !errors.Is(err, ErrUnsupportedFormat) {
			t.Errorf("expected ErrUnsupportedFormat, got %v", err)
		}
	})

	t.Run("output_lang_requires_template", func(t *testing.T) {
		t.Parallel()

		path := createTestAudioFile(t, "audio.ogg")
		env := &Env{
			Stderr:         &syncBuffer{},
			Getenv:         func(string) string { return "" }, // No API key
			Now:            fixedTime(time.Now()),
			FFmpegResolver: &mockFFmpegResolver{},
			ConfigLoader:   &mockConfigLoader{},
		}
		cmd := createTranscribeCmd(context.Background())

		opts := mustParseTranscribeOptions(t, path, "", "", false, 5, "", "en", "deepseek")
		err := RunTranscribe(cmd, env, opts)
		if err == nil {
			t.Fatal("expected error")
		}
		// Should fail with translate requires template error before API key check
		if !strings.Contains(err.Error(), "translate") {
			t.Errorf("expected translate error, got %v", err)
		}
	})

	t.Run("api_key_check_last", func(t *testing.T) {
		t.Parallel()

		path := createTestAudioFile(t, "audio.ogg")
		env := &Env{
			Stderr:         &syncBuffer{},
			Getenv:         func(string) string { return "" }, // No API key
			Now:            fixedTime(time.Now()),
			FFmpegResolver: &mockFFmpegResolver{},
			ConfigLoader:   &mockConfigLoader{},
		}
		cmd := createTranscribeCmd(context.Background())

		opts := mustParseTranscribeOptions(t, path, "", "", false, 5, "", "", "deepseek")
		err := RunTranscribe(cmd, env, opts)
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrAPIKeyMissing) {
			t.Errorf("expected ErrAPIKeyMissing, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for provider flag
// ---------------------------------------------------------------------------

func TestRunTranscribe_DeepSeekProvider_MissingKey(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

	// Only provide OpenAI key, not DeepSeek key
	env := &Env{
		Stderr: &syncBuffer{},
		Getenv: func(key string) string {
			if key == EnvOpenAIAPIKey {
				return "test-openai-key"
			}
			return "" // No DeepSeek key
		},
		Now:            fixedTime(time.Now()),
		FFmpegResolver: &mockFFmpegResolver{},
		ConfigLoader:   &mockConfigLoader{},
	}
	cmd := createTranscribeCmd(context.Background())

	// Use template to trigger restructuring (which requires DeepSeek key)
	opts := mustParseTranscribeOptions(t, inputPath, outputPath, "brainstorm", false, 5, "", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err == nil {
		t.Fatal("expected error for missing DeepSeek API key")
	}
	if !errors.Is(err, ErrDeepSeekKeyMissing) {
		t.Errorf("expected ErrDeepSeekKeyMissing, got %v", err)
	}
}

func TestRunTranscribe_OpenAIProvider_ReusesKey(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

	chunkPath := filepath.Join(t.TempDir(), "chunk_0.ogg")
	if err := os.WriteFile(chunkPath, []byte("chunk"), 0644); err != nil {
		t.Fatalf("failed to create chunk: %v", err)
	}

	chunker := &mockChunker{
		ChunkFunc: func(ctx context.Context, audioPath string) ([]audio.Chunk, error) {
			return []audio.Chunk{{Path: chunkPath, Index: 0}}, nil
		},
	}
	chunkerFactory := &mockChunkerFactory{
		NewSilenceChunkerFunc: func(ffmpegPath string) (audio.Chunker, error) {
			return chunker, nil
		},
	}

	transcriber := &mockTranscriber{
		TranscribeFunc: func(ctx context.Context, audioPath string, opts transcribe.Options) (string, error) {
			return "transcribed", nil
		},
	}
	transcriberFactory := &mockTranscriberFactory{
		NewTranscriberFunc: func(apiKey string) transcribe.Transcriber {
			return transcriber
		},
	}

	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript string, tmpl template.Name, outputLang lang.Language) (string, bool, error) {
			return "restructured", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	// Only provide OpenAI key - should work with --provider openai
	env := &Env{
		Stderr: &syncBuffer{},
		Getenv: func(key string) string {
			if key == EnvOpenAIAPIKey {
				return "test-openai-key"
			}
			return "" // No DeepSeek key
		},
		Now:                 fixedTime(time.Now()),
		FFmpegResolver:      &mockFFmpegResolver{},
		ConfigLoader:        &mockConfigLoader{},
		ChunkerFactory:      chunkerFactory,
		TranscriberFactory:  transcriberFactory,
		RestructurerFactory: restructurerFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	// Use OpenAI provider - should NOT require DeepSeek key
	opts := mustParseTranscribeOptions(t, inputPath, outputPath, "brainstorm", false, 5, "", "", "openai")
	err := RunTranscribe(cmd, env, opts)
	if err != nil {
		t.Fatalf("expected no error with OpenAI provider, got %v", err)
	}
}

func TestRunTranscribe_ProviderPassedToFactory(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

	chunkPath := filepath.Join(t.TempDir(), "chunk_0.ogg")
	if err := os.WriteFile(chunkPath, []byte("chunk"), 0644); err != nil {
		t.Fatalf("failed to create chunk: %v", err)
	}

	chunker := &mockChunker{
		ChunkFunc: func(ctx context.Context, audioPath string) ([]audio.Chunk, error) {
			return []audio.Chunk{{Path: chunkPath, Index: 0}}, nil
		},
	}
	chunkerFactory := &mockChunkerFactory{
		NewSilenceChunkerFunc: func(ffmpegPath string) (audio.Chunker, error) {
			return chunker, nil
		},
	}

	transcriber := &mockTranscriber{
		TranscribeFunc: func(ctx context.Context, audioPath string, opts transcribe.Options) (string, error) {
			return "transcribed", nil
		},
	}
	transcriberFactory := &mockTranscriberFactory{
		NewTranscriberFunc: func(apiKey string) transcribe.Transcriber {
			return transcriber
		},
	}

	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript string, tmpl template.Name, outputLang lang.Language) (string, bool, error) {
			return "restructured", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		Now:                 fixedTime(time.Now()),
		FFmpegResolver:      &mockFFmpegResolver{},
		ConfigLoader:        &mockConfigLoader{},
		ChunkerFactory:      chunkerFactory,
		TranscriberFactory:  transcriberFactory,
		RestructurerFactory: restructurerFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	opts := mustParseTranscribeOptions(t, inputPath, outputPath, "brainstorm", false, 5, "", "", "deepseek")
	err := RunTranscribe(cmd, env, opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify the correct provider was passed to the factory
	calls := restructurerFactory.NewMapReducerCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 NewMapReducer call, got %d", len(calls))
	}
	if calls[0].Provider != DeepSeekProvider {
		t.Errorf("expected provider %q, got %q", DeepSeekProvider, calls[0].Provider)
	}
}
