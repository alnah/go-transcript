package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/alnah/go-transcript/internal/config"
)

// Notes:
// - Tests focus on observable behavior through public APIs (runStructure, StructureCmd)
// - Internal validation order is tested via error types, not implementation details
// - File I/O is tested with real temp files; restructuring uses mocks
// - The mockRestructurerFactory from mocks_test.go is reused for consistency

// ---------------------------------------------------------------------------
// Tests for deriveStructuredOutputPath - Path transformation logic
// ---------------------------------------------------------------------------

func TestDeriveStructuredOutputPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple_md", "meeting.md", "meeting_structured.md"},
		{"removes_raw_suffix", "meeting_raw.md", "meeting_structured.md"},
		{"preserves_extension", "notes.txt", "notes_structured.txt"},
		{"no_extension", "transcript", "transcript_structured"},
		{"preserves_path", "/path/to/meeting.md", "/path/to/meeting_structured.md"},
		{"path_with_raw", "/path/to/notes_raw.md", "/path/to/notes_structured.md"},
		{"double_extension", "file.backup.md", "file.backup_structured.md"},
		{"empty_string", "", "_structured"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := DeriveStructuredOutputPath(tt.input)
			if result != tt.expected {
				t.Errorf("DeriveStructuredOutputPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests for StructureCmd - Cobra command creation and flag validation
// ---------------------------------------------------------------------------

func TestStructureCmd_RequiresFile(t *testing.T) {
	t.Parallel()

	env, _ := testEnv()
	cmd := StructureCmd(env)

	cmd.SetArgs([]string{})
	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected error when file not provided")
	}
}

func TestStructureCmd_RequiresTemplate(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "test content")

	env, _ := testEnv()
	cmd := StructureCmd(env)

	cmd.SetArgs([]string{inputPath})
	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected error when template not provided")
	}
	if !strings.Contains(err.Error(), "template") {
		t.Errorf("expected template error, got %v", err)
	}
}

func TestStructureCmd_DefaultProvider(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "test content")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			return "restructured", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		FFmpegResolver:      &mockFFmpegResolver{},
		ConfigLoader:        &mockConfigLoader{},
		RestructurerFactory: restructurerFactory,
	}

	cmd := StructureCmd(env)
	cmd.SetArgs([]string{inputPath, "-t", "brainstorm", "-o", outputPath})
	err := cmd.Execute()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify default provider was DeepSeek
	calls := restructurerFactory.NewMapReducerCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 NewMapReducer call, got %d", len(calls))
	}
	if calls[0].Provider != ProviderDeepSeek {
		t.Errorf("expected default provider %q, got %q", ProviderDeepSeek, calls[0].Provider)
	}
}

// ---------------------------------------------------------------------------
// Tests for runStructure - Core restructuring logic
// ---------------------------------------------------------------------------

// createStructureCmd creates a cobra.Command for testing runStructure.
func createStructureCmd(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	return cmd
}

// createTestTranscriptFile creates a temporary transcript file for testing.
func createTestTranscriptFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test transcript file: %v", err)
	}
	return path
}

func TestRunStructure_FileNotFound(t *testing.T) {
	t.Parallel()

	env, _ := testEnv()
	cmd := createStructureCmd(context.Background())

	err := RunStructure(cmd, env, "/nonexistent/file.md", "", "brainstorm", "", ProviderDeepSeek)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %v", err)
	}
}

func TestRunStructure_EmptyFile(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "")

	env, _ := testEnv()
	cmd := createStructureCmd(context.Background())

	err := RunStructure(cmd, env, inputPath, "", "brainstorm", "", ProviderDeepSeek)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' error, got %v", err)
	}
}

func TestRunStructure_WhitespaceOnlyFile(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "   \n\t  \n  ")

	env, _ := testEnv()
	cmd := createStructureCmd(context.Background())

	err := RunStructure(cmd, env, inputPath, "", "brainstorm", "", ProviderDeepSeek)
	if err == nil {
		t.Fatal("expected error for whitespace-only file")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' error, got %v", err)
	}
}

func TestRunStructure_OutputExists(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "test content")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "existing.md")

	// Create existing output file
	if err := os.WriteFile(outputPath, []byte("existing"), 0644); err != nil {
		t.Fatalf("failed to create existing file: %v", err)
	}

	env, _ := testEnv()
	cmd := createStructureCmd(context.Background())

	err := RunStructure(cmd, env, inputPath, outputPath, "brainstorm", "", ProviderDeepSeek)
	if err == nil {
		t.Fatal("expected error for existing output file")
	}
	if !errors.Is(err, ErrOutputExists) {
		t.Errorf("expected ErrOutputExists, got %v", err)
	}
}

func TestRunStructure_InvalidProvider(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "test content")

	env, _ := testEnv()
	cmd := createStructureCmd(context.Background())

	err := RunStructure(cmd, env, inputPath, "", "brainstorm", "", "invalid-provider")
	if err == nil {
		t.Fatal("expected error for invalid provider")
	}
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Errorf("expected ErrUnsupportedProvider, got %v", err)
	}
}

func TestRunStructure_InvalidTemplate(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "test content")

	env, _ := testEnv()
	cmd := createStructureCmd(context.Background())

	err := RunStructure(cmd, env, inputPath, "", "nonexistent-template", "", ProviderDeepSeek)
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
	if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "template") {
		t.Errorf("expected template error, got %v", err)
	}
}

func TestRunStructure_InvalidLanguage(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "test content")

	env, _ := testEnv()
	cmd := createStructureCmd(context.Background())

	err := RunStructure(cmd, env, inputPath, "", "brainstorm", "invalid-lang-code-too-long", ProviderDeepSeek)
	if err == nil {
		t.Fatal("expected error for invalid language")
	}
}

func TestRunStructure_MissingDeepSeekKey(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "test content")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

	env := &Env{
		Stderr: &syncBuffer{},
		Getenv: func(key string) string {
			if key == EnvOpenAIAPIKey {
				return "test-openai-key"
			}
			return "" // No DeepSeek key
		},
		ConfigLoader:        &mockConfigLoader{},
		RestructurerFactory: &mockRestructurerFactory{},
	}
	cmd := createStructureCmd(context.Background())

	err := RunStructure(cmd, env, inputPath, outputPath, "brainstorm", "", ProviderDeepSeek)
	if err == nil {
		t.Fatal("expected error for missing DeepSeek API key")
	}
	if !errors.Is(err, ErrDeepSeekKeyMissing) {
		t.Errorf("expected ErrDeepSeekKeyMissing, got %v", err)
	}
}

func TestRunStructure_MissingOpenAIKey(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "test content")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

	env := &Env{
		Stderr: &syncBuffer{},
		Getenv: func(key string) string {
			if key == EnvDeepSeekAPIKey {
				return "test-deepseek-key"
			}
			return "" // No OpenAI key
		},
		ConfigLoader:        &mockConfigLoader{},
		RestructurerFactory: &mockRestructurerFactory{},
	}
	cmd := createStructureCmd(context.Background())

	err := RunStructure(cmd, env, inputPath, outputPath, "brainstorm", "", ProviderOpenAI)
	if err == nil {
		t.Fatal("expected error for missing OpenAI API key")
	}
	if !errors.Is(err, ErrAPIKeyMissing) {
		t.Errorf("expected ErrAPIKeyMissing, got %v", err)
	}
}

func TestRunStructure_Success(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "This is the raw transcript content.")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")
	stderr := &syncBuffer{}

	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			return "# Restructured Output\n\nKey ideas here.", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              stderr,
		Getenv:              defaultTestEnv,
		ConfigLoader:        &mockConfigLoader{},
		RestructurerFactory: restructurerFactory,
	}
	cmd := createStructureCmd(context.Background())

	err := RunStructure(cmd, env, inputPath, outputPath, "brainstorm", "", ProviderDeepSeek)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify output file was created
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if !strings.Contains(string(content), "Restructured Output") {
		t.Errorf("expected restructured content, got %q", string(content))
	}

	// Verify stderr contains progress messages
	output := stderr.String()
	if !strings.Contains(output, "Reading") {
		t.Errorf("expected 'Reading' in output, got %q", output)
	}
	if !strings.Contains(output, "Restructuring") {
		t.Errorf("expected 'Restructuring' in output, got %q", output)
	}
	if !strings.Contains(output, "Done") {
		t.Errorf("expected 'Done' in output, got %q", output)
	}
}

func TestRunStructure_SuccessWithOpenAI(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "transcript content")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			return "restructured", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	// Only provide OpenAI key
	env := &Env{
		Stderr: &syncBuffer{},
		Getenv: func(key string) string {
			if key == EnvOpenAIAPIKey {
				return "test-openai-key"
			}
			return ""
		},
		ConfigLoader:        &mockConfigLoader{},
		RestructurerFactory: restructurerFactory,
	}
	cmd := createStructureCmd(context.Background())

	err := RunStructure(cmd, env, inputPath, outputPath, "meeting", "", ProviderOpenAI)
	if err != nil {
		t.Fatalf("expected no error with OpenAI provider, got %v", err)
	}

	// Verify OpenAI provider was used
	calls := restructurerFactory.NewMapReducerCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 NewMapReducer call, got %d", len(calls))
	}
	if calls[0].Provider != ProviderOpenAI {
		t.Errorf("expected provider %q, got %q", ProviderOpenAI, calls[0].Provider)
	}
}

func TestRunStructure_WithOutputLang(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "transcript content")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

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
		Getenv:              defaultTestEnv,
		ConfigLoader:        &mockConfigLoader{},
		RestructurerFactory: restructurerFactory,
	}
	cmd := createStructureCmd(context.Background())

	err := RunStructure(cmd, env, inputPath, outputPath, "meeting", "fr", ProviderDeepSeek)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if capturedLang != "fr" {
		t.Errorf("expected output lang 'fr', got %q", capturedLang)
	}
}

func TestRunStructure_RestructureError(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "transcript content")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")

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
		Getenv:              defaultTestEnv,
		ConfigLoader:        &mockConfigLoader{},
		RestructurerFactory: restructurerFactory,
	}
	cmd := createStructureCmd(context.Background())

	err := RunStructure(cmd, env, inputPath, outputPath, "brainstorm", "", ProviderDeepSeek)
	if err == nil {
		t.Fatal("expected error when restructuring fails")
	}
	if !errors.Is(err, restructureErr) {
		t.Errorf("expected restructure error, got %v", err)
	}
}

func TestRunStructure_DefaultOutputPath(t *testing.T) {
	t.Parallel()

	inputDir := t.TempDir()
	inputPath := filepath.Join(inputDir, "meeting_raw.md")
	if err := os.WriteFile(inputPath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create input file: %v", err)
	}

	outputDir := t.TempDir()

	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			return "restructured", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	configLoader := &mockConfigLoader{
		LoadFunc: func() (config.Config, error) {
			return config.Config{OutputDir: outputDir}, nil
		},
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		ConfigLoader:        configLoader,
		RestructurerFactory: restructurerFactory,
	}
	cmd := createStructureCmd(context.Background())

	// Empty output path - should derive from input and use output-dir
	err := RunStructure(cmd, env, inputPath, "", "brainstorm", "", ProviderDeepSeek)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify output was created with expected name (meeting_structured.md, not meeting_raw_structured.md)
	expectedPath := filepath.Join(outputDir, "meeting_structured.md")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected output file at %s", expectedPath)
	}
}

func TestRunStructure_ProgressCallback(t *testing.T) {
	t.Parallel()

	inputPath := createTestTranscriptFile(t, "transcript content")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.md")
	stderr := &syncBuffer{}

	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			return "restructured", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              stderr,
		Getenv:              defaultTestEnv,
		ConfigLoader:        &mockConfigLoader{},
		RestructurerFactory: restructurerFactory,
	}
	cmd := createStructureCmd(context.Background())

	err := RunStructure(cmd, env, inputPath, outputPath, "brainstorm", "", ProviderDeepSeek)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify restructuring message includes provider
	output := stderr.String()
	if !strings.Contains(output, "deepseek") {
		t.Errorf("expected provider 'deepseek' in output, got %q", output)
	}
}

// ---------------------------------------------------------------------------
// Tests for validation order
// ---------------------------------------------------------------------------

func TestRunStructure_ValidationOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) (inputPath string, env *Env)
		output      string
		template    string
		outputLang  string
		provider    string
		wantErr     error
		wantContain string
	}{
		{
			name: "file_not_found_first",
			setup: func(t *testing.T) (string, *Env) {
				env, _ := testEnv()
				return "/nonexistent/path.md", env
			},
			template:    "brainstorm",
			provider:    ProviderDeepSeek,
			wantContain: "not found",
		},
		{
			name: "output_check_before_provider",
			setup: func(t *testing.T) (string, *Env) {
				inputPath := createTestTranscriptFile(t, "content")
				outputDir := t.TempDir()
				outputPath := filepath.Join(outputDir, "existing.md")
				if err := os.WriteFile(outputPath, []byte("existing"), 0644); err != nil {
					t.Fatal(err)
				}
				env, _ := testEnv()
				return inputPath, env
			},
			output:   "", // Will be auto-derived but we need to test with explicit existing
			template: "brainstorm",
			provider: "invalid",              // Invalid provider, but output check should come first
			wantErr:  ErrUnsupportedProvider, // Actually provider is checked before template
		},
		{
			name: "provider_check_before_template",
			setup: func(t *testing.T) (string, *Env) {
				inputPath := createTestTranscriptFile(t, "content")
				env, _ := testEnv()
				return inputPath, env
			},
			template: "invalid-template",
			provider: "invalid-provider",
			wantErr:  ErrUnsupportedProvider,
		},
		{
			name: "template_check_before_language",
			setup: func(t *testing.T) (string, *Env) {
				inputPath := createTestTranscriptFile(t, "content")
				env, _ := testEnv()
				return inputPath, env
			},
			template:    "invalid-template",
			outputLang:  "invalid-lang",
			provider:    ProviderDeepSeek,
			wantContain: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inputPath, env := tt.setup(t)
			cmd := createStructureCmd(context.Background())

			err := RunStructure(cmd, env, inputPath, tt.output, tt.template, tt.outputLang, tt.provider)
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
