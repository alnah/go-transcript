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
	"github.com/alnah/go-transcript/internal/transcribe"
)

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
// Tests for runTranscribe
// ---------------------------------------------------------------------------

// createTranscribeCmd creates a cobra.Command for testing runTranscribe.
// This is needed because runTranscribe expects a *cobra.Command for context.
func createTranscribeCmd(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	return cmd
}

func TestRunTranscribe_FileNotFound(t *testing.T) {
	t.Parallel()

	env, _ := testEnv()
	cmd := createTranscribeCmd(context.Background())

	err := runTranscribe(cmd, env, "/nonexistent/file.ogg", "", "", false, 5, "", "")
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

	err := runTranscribe(cmd, env, inputPath, "", "", false, 5, "", "")
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestRunTranscribe_InvalidTemplate(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")

	env, _ := testEnv()
	cmd := createTranscribeCmd(context.Background())

	err := runTranscribe(cmd, env, inputPath, "", "nonexistent-template", false, 5, "", "")
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
	if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "template") {
		t.Errorf("expected template error, got %v", err)
	}
}

func TestRunTranscribe_InvalidLanguage(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")

	env, _ := testEnv()
	cmd := createTranscribeCmd(context.Background())

	err := runTranscribe(cmd, env, inputPath, "", "", false, 5, "invalid-lang", "")
	if err == nil {
		t.Fatal("expected error for invalid language")
	}
}

func TestRunTranscribe_OutputLangRequiresTemplate(t *testing.T) {
	t.Parallel()

	inputPath := createTestAudioFile(t, "audio.ogg")

	env, _ := testEnv()
	cmd := createTranscribeCmd(context.Background())

	err := runTranscribe(cmd, env, inputPath, "", "", false, 5, "", "en")
	if err == nil {
		t.Fatal("expected error when output-lang without template")
	}
	if !strings.Contains(err.Error(), "output-lang") || !strings.Contains(err.Error(), "template") {
		t.Errorf("expected output-lang/template error, got %v", err)
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

	err := runTranscribe(cmd, env, inputPath, "", "", false, 5, "", "")
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
		Getenv:         staticEnv(map[string]string{"OPENAI_API_KEY": "test-key"}),
		Now:            fixedTime(time.Now()),
		FFmpegResolver: ffmpegResolver,
		ConfigLoader:   &mockConfigLoader{},
	}
	cmd := createTranscribeCmd(context.Background())

	err := runTranscribe(cmd, env, inputPath, "", "", false, 5, "", "")
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
		Getenv:         staticEnv(map[string]string{"OPENAI_API_KEY": "test-key"}),
		Now:            fixedTime(time.Now()),
		FFmpegResolver: &mockFFmpegResolver{},
		ConfigLoader:   &mockConfigLoader{},
		ChunkerFactory: chunkerFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	err := runTranscribe(cmd, env, inputPath, outputPath, "", false, 5, "", "")
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
		Getenv:             staticEnv(map[string]string{"OPENAI_API_KEY": "test-key"}),
		Now:                fixedTime(time.Now()),
		FFmpegResolver:     &mockFFmpegResolver{},
		ConfigLoader:       &mockConfigLoader{},
		ChunkerFactory:     chunkerFactory,
		TranscriberFactory: transcriberFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	err := runTranscribe(cmd, env, inputPath, outputPath, "", false, 5, "", "")
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
		Getenv:             staticEnv(map[string]string{"OPENAI_API_KEY": "test-key"}),
		Now:                fixedTime(time.Now()),
		FFmpegResolver:     &mockFFmpegResolver{},
		ConfigLoader:       &mockConfigLoader{},
		ChunkerFactory:     chunkerFactory,
		TranscriberFactory: transcriberFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	err := runTranscribe(cmd, env, inputPath, outputPath, "", false, 5, "", "")
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
		Getenv:             staticEnv(map[string]string{"OPENAI_API_KEY": "test-key"}),
		Now:                fixedTime(time.Now()),
		FFmpegResolver:     &mockFFmpegResolver{},
		ConfigLoader:       &mockConfigLoader{},
		ChunkerFactory:     chunkerFactory,
		TranscriberFactory: transcriberFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	err := runTranscribe(cmd, env, inputPath, outputPath, "", false, 5, "fr", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if capturedOpts.Language != "fr" {
		t.Errorf("expected language 'fr', got %q", capturedOpts.Language)
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
		Getenv:             staticEnv(map[string]string{"OPENAI_API_KEY": "test-key"}),
		Now:                fixedTime(time.Now()),
		FFmpegResolver:     &mockFFmpegResolver{},
		ConfigLoader:       &mockConfigLoader{},
		ChunkerFactory:     chunkerFactory,
		TranscriberFactory: transcriberFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	err := runTranscribe(cmd, env, inputPath, outputPath, "", true, 5, "", "")
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
		Getenv:             staticEnv(map[string]string{"OPENAI_API_KEY": "test-key"}),
		Now:                fixedTime(time.Now()),
		FFmpegResolver:     &mockFFmpegResolver{},
		ConfigLoader:       configLoader,
		ChunkerFactory:     chunkerFactory,
		TranscriberFactory: transcriberFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	// Empty output path - should use default derived from input
	err := runTranscribe(cmd, env, inputPath, "", "", false, 5, "", "")
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
	var capturedTemplate string
	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			capturedTemplate = templateName
			return "# Restructured Output\n\nKey ideas here.", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              stderr,
		Getenv:              staticEnv(map[string]string{"OPENAI_API_KEY": "test-key"}),
		Now:                 fixedTime(time.Now()),
		FFmpegResolver:      &mockFFmpegResolver{},
		ConfigLoader:        &mockConfigLoader{},
		ChunkerFactory:      chunkerFactory,
		TranscriberFactory:  transcriberFactory,
		RestructurerFactory: restructurerFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	err := runTranscribe(cmd, env, inputPath, outputPath, "brainstorm", false, 5, "", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify restructurer was called with correct template
	if capturedTemplate != "brainstorm" {
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

	var capturedLang string
	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			capturedLang = outputLang
			return "restructured", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              staticEnv(map[string]string{"OPENAI_API_KEY": "test-key"}),
		Now:                 fixedTime(time.Now()),
		FFmpegResolver:      &mockFFmpegResolver{},
		ConfigLoader:        &mockConfigLoader{},
		ChunkerFactory:      chunkerFactory,
		TranscriberFactory:  transcriberFactory,
		RestructurerFactory: restructurerFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	// Test: input language fr, output language en
	err := runTranscribe(cmd, env, inputPath, outputPath, "meeting", false, 5, "fr", "en")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Output language should be "en" (explicitly specified)
	if capturedLang != "en" {
		t.Errorf("expected output lang 'en', got %q", capturedLang)
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

	var capturedLang string
	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			capturedLang = outputLang
			return "restructured", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              staticEnv(map[string]string{"OPENAI_API_KEY": "test-key"}),
		Now:                 fixedTime(time.Now()),
		FFmpegResolver:      &mockFFmpegResolver{},
		ConfigLoader:        &mockConfigLoader{},
		ChunkerFactory:      chunkerFactory,
		TranscriberFactory:  transcriberFactory,
		RestructurerFactory: restructurerFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	// Test: input language fr, no output language -> should inherit fr
	err := runTranscribe(cmd, env, inputPath, outputPath, "meeting", false, 5, "fr", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Output language should be "fr" (inherited from input)
	if capturedLang != "fr" {
		t.Errorf("expected output lang 'fr' (inherited), got %q", capturedLang)
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
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			return "", false, restructureErr
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              staticEnv(map[string]string{"OPENAI_API_KEY": "test-key"}),
		Now:                 fixedTime(time.Now()),
		FFmpegResolver:      &mockFFmpegResolver{},
		ConfigLoader:        &mockConfigLoader{},
		ChunkerFactory:      chunkerFactory,
		TranscriberFactory:  transcriberFactory,
		RestructurerFactory: restructurerFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	err := runTranscribe(cmd, env, inputPath, outputPath, "brainstorm", false, 5, "", "")
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
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			restructureCalled = true
			return "should not be called", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              staticEnv(map[string]string{"OPENAI_API_KEY": "test-key"}),
		Now:                 fixedTime(time.Now()),
		FFmpegResolver:      &mockFFmpegResolver{},
		ConfigLoader:        &mockConfigLoader{},
		ChunkerFactory:      chunkerFactory,
		TranscriberFactory:  transcriberFactory,
		RestructurerFactory: restructurerFactory,
	}
	cmd := createTranscribeCmd(context.Background())

	err := runTranscribe(cmd, env, inputPath, outputPath, "brainstorm", false, 5, "", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Restructure should NOT be called for empty transcript
	if restructureCalled {
		t.Error("restructure should not be called for empty/whitespace transcript")
	}
}

// ---------------------------------------------------------------------------
// Tests for validation order
// ---------------------------------------------------------------------------

func TestRunTranscribe_ValidationOrder(t *testing.T) {
	t.Parallel()

	// Test that validation occurs in the documented order:
	// 1. File exists
	// 2. Format supported
	// 3. Config load (warning only)
	// 4. Output path
	// 5. Template valid
	// 6. Language validation
	// 7. Output language requires template
	// 8. Parallel bounds
	// 9. API key present

	tests := []struct {
		name        string
		setup       func(t *testing.T) (inputPath string, env *Env)
		output      string
		template    string
		language    string
		outputLang  string
		wantErr     error
		wantContain string
	}{
		{
			name: "file_not_found_before_format_check",
			setup: func(t *testing.T) (string, *Env) {
				env, _ := testEnv()
				return "/nonexistent/path.ogg", env
			},
			wantErr: ErrFileNotFound,
		},
		{
			name: "format_check_before_template_check",
			setup: func(t *testing.T) (string, *Env) {
				// Create file with bad extension
				path := createTestAudioFile(t, "audio.xyz")
				env, _ := testEnv()
				return path, env
			},
			template: "brainstorm", // Valid template, but should fail before
			wantErr:  ErrUnsupportedFormat,
		},
		{
			name: "template_check_before_api_key",
			setup: func(t *testing.T) (string, *Env) {
				path := createTestAudioFile(t, "audio.ogg")
				env := &Env{
					Stderr:         &syncBuffer{},
					Getenv:         func(string) string { return "" }, // No API key
					Now:            fixedTime(time.Now()),
					FFmpegResolver: &mockFFmpegResolver{},
					ConfigLoader:   &mockConfigLoader{},
				}
				return path, env
			},
			template:    "invalid-template",
			wantContain: "unknown", // Template error before API key error
		},
		{
			name: "output_lang_requires_template",
			setup: func(t *testing.T) (string, *Env) {
				path := createTestAudioFile(t, "audio.ogg")
				env := &Env{
					Stderr:         &syncBuffer{},
					Getenv:         func(string) string { return "" }, // No API key
					Now:            fixedTime(time.Now()),
					FFmpegResolver: &mockFFmpegResolver{},
					ConfigLoader:   &mockConfigLoader{},
				}
				return path, env
			},
			outputLang:  "en",
			wantContain: "output-lang", // Should fail before API key check
		},
		{
			name: "api_key_check_last",
			setup: func(t *testing.T) (string, *Env) {
				path := createTestAudioFile(t, "audio.ogg")
				env := &Env{
					Stderr:         &syncBuffer{},
					Getenv:         func(string) string { return "" }, // No API key
					Now:            fixedTime(time.Now()),
					FFmpegResolver: &mockFFmpegResolver{},
					ConfigLoader:   &mockConfigLoader{},
				}
				return path, env
			},
			wantErr: ErrAPIKeyMissing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inputPath, env := tt.setup(t)
			cmd := createTranscribeCmd(context.Background())

			err := runTranscribe(cmd, env, inputPath, tt.output, tt.template, false, 5, tt.language, tt.outputLang)
			if err == nil {
				t.Fatal("expected error")
			}

			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
			if tt.wantContain != "" && !strings.Contains(err.Error(), tt.wantContain) {
				t.Errorf("expected error containing %q, got %v", tt.wantContain, err)
			}
		})
	}
}
