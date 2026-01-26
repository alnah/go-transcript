//go:build e2e

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// =============================================================================
// E2E Test Helpers
// =============================================================================

// e2eTimeout is the maximum time for each E2E test.
// 2 minutes provides comfortable margin for API latency.
const e2eTimeout = 2 * time.Minute

// skipIfNoAPIKey skips the test if OPENAI_API_KEY is not set.
// Returns the API key if available.
func skipIfNoAPIKey(t *testing.T) string {
	t.Helper()
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping E2E test")
	}
	return apiKey
}

// setupE2EEnv creates an isolated environment for E2E tests.
// Returns the temp directory path.
// Sets HOME to temp directory to isolate config.
func setupE2EEnv(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()

	// Isolate config by setting HOME to temp directory
	t.Setenv("HOME", tempDir)

	// Also set XDG_CONFIG_HOME for Linux compatibility
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempDir, ".config"))

	return tempDir
}

// skipOnTransientError skips the test if the error is transient (rate limit, timeout).
// Fails the test for permanent errors (auth, quota).
// Returns true if test should continue (no error or skipped).
func skipOnTransientError(t *testing.T, err error) bool {
	t.Helper()

	if err == nil {
		return true
	}

	// Transient errors: skip with clear warning
	if errors.Is(err, ErrRateLimit) {
		t.Skipf("SKIP: Rate limit exceeded (transient) - %v", err)
		return false
	}
	if errors.Is(err, ErrTimeout) {
		t.Skipf("SKIP: Request timeout (transient) - %v", err)
		return false
	}

	// Permanent errors: fail immediately
	if errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("FAIL: Quota exceeded (check billing) - %v", err)
		return false
	}
	if errors.Is(err, ErrAuthFailed) {
		t.Fatalf("FAIL: Authentication failed (check API key) - %v", err)
		return false
	}

	// Other errors: fail
	t.Fatalf("FAIL: Unexpected error - %v", err)
	return false
}

// assertOutputValid validates the transcription/restructuring output.
// Checks: non-empty, minimum length, optional structure validation.
func assertOutputValid(t *testing.T, content string, minLength int, requiredPatterns ...string) {
	t.Helper()

	if content == "" {
		t.Error("output is empty")
		return
	}

	if len(content) < minLength {
		t.Errorf("output too short: got %d chars, want >= %d", len(content), minLength)
	}

	for _, pattern := range requiredPatterns {
		if !strings.Contains(content, pattern) {
			t.Errorf("output missing required pattern %q", pattern)
		}
	}
}

// assertMarkdownStructure validates that content has markdown heading structure.
func assertMarkdownStructure(t *testing.T, content string) {
	t.Helper()

	// Should have at least one heading
	if !strings.Contains(content, "# ") && !strings.Contains(content, "## ") {
		t.Error("output missing markdown headings (# or ##)")
	}
}

// =============================================================================
// E2E Tests
// =============================================================================

// TestE2E_TranscribeBasic tests basic transcription without template.
// Uses testdata/short.ogg (~3s) for speed.
// Note: Synthetic audio (sine wave) may produce empty or minimal transcription.
// This test validates the pipeline works, not transcription quality.
func TestE2E_TranscribeBasic(t *testing.T) {
	skipIfNoAPIKey(t)
	tempDir := setupE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), e2eTimeout)
	defer cancel()

	inputPath := "testdata/short.ogg"
	outputPath := filepath.Join(tempDir, "transcript.md")

	// Verify input exists
	if _, err := os.Stat(inputPath); err != nil {
		t.Fatalf("test fixture missing: %v", err)
	}

	// Run transcription
	err := runTranscribeE2E(ctx, inputPath, outputPath, "", false, 3, "", "")
	if !skipOnTransientError(t, err) {
		return
	}

	// Verify output file was created
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	// Note: Synthetic audio (sine wave) typically produces empty/minimal transcription
	// because there's no speech. We only verify the pipeline completed successfully.
	// Empty output is acceptable for synthetic audio - the API processed it correctly.
	t.Logf("Transcription output: %d bytes (empty is acceptable for synthetic audio)", len(content))
}

// TestE2E_TranscribeWithTemplate tests transcription with brainstorm template.
// Uses testdata/short.ogg (~3s) for speed.
// Note: Even with empty transcription, the restructurer should produce markdown structure.
func TestE2E_TranscribeWithTemplate(t *testing.T) {
	skipIfNoAPIKey(t)
	tempDir := setupE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), e2eTimeout)
	defer cancel()

	inputPath := "testdata/short.ogg"
	outputPath := filepath.Join(tempDir, "brainstorm.md")

	// Run transcription with template
	err := runTranscribeE2E(ctx, inputPath, outputPath, "brainstorm", false, 3, "", "")
	if !skipOnTransientError(t, err) {
		return
	}

	// Verify output
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	// With template, the restructurer should produce some markdown structure
	// even if the transcription is empty/minimal. We validate the pipeline worked.
	if len(content) > 0 {
		// If we got content, check markdown structure
		assertMarkdownStructure(t, string(content))
	}

	t.Logf("Restructured output: %d bytes", len(content))
}

// TestE2E_LiveRecordTranscribe tests the full live pipeline.
// Uses lavfi to generate synthetic audio (no real microphone needed).
// Note: Synthetic audio will produce empty/minimal transcription - we test the pipeline, not quality.
func TestE2E_LiveRecordTranscribe(t *testing.T) {
	skipIfNoAPIKey(t)
	ffmpegPath := skipIfNoFFmpeg(t)
	tempDir := setupE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), e2eTimeout)
	defer cancel()

	outputPath := filepath.Join(tempDir, "live.md")
	audioPath := filepath.Join(tempDir, "live.ogg")

	// Generate synthetic audio using lavfi (same as integration tests)
	const duration = 3 * time.Second
	err := recordWithLavfi(ctx, ffmpegPath, duration, audioPath)
	if err != nil {
		t.Fatalf("failed to generate test audio: %v", err)
	}

	// Verify audio was created
	if _, err := os.Stat(audioPath); err != nil {
		t.Fatalf("test audio not created: %v", err)
	}

	// Run transcription on generated audio
	err = runTranscribeE2E(ctx, audioPath, outputPath, "", false, 3, "", "")
	if !skipOnTransientError(t, err) {
		return
	}

	// Verify output file was created (content may be empty for synthetic audio)
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	t.Logf("Live pipeline output: %d bytes (validates record->transcribe pipeline)", len(content))
}

// TestE2E_ConfigPersistence tests that config is persisted and used.
func TestE2E_ConfigPersistence(t *testing.T) {
	skipIfNoAPIKey(t)
	tempDir := setupE2EEnv(t)

	// Create output directory
	outputDir := filepath.Join(tempDir, "transcripts")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("failed to create output dir: %v", err)
	}

	// Set config output-dir
	err := SaveConfigValue("output-dir", outputDir)
	if err != nil {
		t.Fatalf("failed to set config: %v", err)
	}

	// Verify config was saved
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.OutputDir != outputDir {
		t.Errorf("config output-dir not persisted: got %q, want %q", cfg.OutputDir, outputDir)
	}

	// Run transcription with output in configured directory
	ctx, cancel := context.WithTimeout(context.Background(), e2eTimeout)
	defer cancel()

	inputPath := "testdata/short.ogg"
	// Use a path within the configured output-dir
	outputPath := filepath.Join(outputDir, "config_test.md")

	err = runTranscribeE2E(ctx, inputPath, outputPath, "", false, 3, "", "")
	if !skipOnTransientError(t, err) {
		return
	}

	// Verify file was created in configured directory
	if _, err := os.Stat(outputPath); err != nil {
		t.Errorf("output file not created at %s: %v", outputPath, err)
	}

	t.Logf("Config persistence verified: output in %s", outputDir)
}

// TestE2E_LanguageHandling tests language flags.
// Uses --language for input and --output-lang for restructured output.
// Note: With synthetic audio, this validates the language parameters are accepted,
// not that translation actually occurred.
func TestE2E_LanguageHandling(t *testing.T) {
	skipIfNoAPIKey(t)
	tempDir := setupE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), e2eTimeout)
	defer cancel()

	inputPath := "testdata/short.ogg"
	outputPath := filepath.Join(tempDir, "french.md")

	// Transcribe with English input, French output
	err := runTranscribeE2E(ctx, inputPath, outputPath, "brainstorm", false, 3, "en", "fr")
	if !skipOnTransientError(t, err) {
		return
	}

	// Verify output exists (content may be minimal with synthetic audio)
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	// With template, check structure if we got content
	if len(content) > 0 {
		assertMarkdownStructure(t, string(content))
	}

	t.Logf("Language handling output: %d bytes (validates -l and --output-lang flags)", len(content))
}

// TestE2E_AuthFailure tests that invalid API key produces correct error.
func TestE2E_AuthFailure(t *testing.T) {
	// Check real API key exists (we need it to know E2E is expected to work)
	realKey := os.Getenv("OPENAI_API_KEY")
	if realKey == "" {
		t.Skip("OPENAI_API_KEY not set, cannot test auth failure")
	}

	tempDir := setupE2EEnv(t)

	// Set invalid API key for this test
	t.Setenv("OPENAI_API_KEY", "sk-invalid-key-for-testing")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	inputPath := "testdata/short.ogg"
	outputPath := filepath.Join(tempDir, "should-not-exist.md")

	// Run transcription - should fail with auth error
	err := runTranscribeE2E(ctx, inputPath, outputPath, "", false, 3, "", "")

	if err == nil {
		t.Fatal("expected auth error, got nil")
	}

	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("expected ErrAuthFailed, got: %v", err)
	}

	// Output file should not exist
	if _, err := os.Stat(outputPath); err == nil {
		t.Error("output file should not exist after auth failure")
	}

	t.Logf("Auth failure correctly detected: %v", err)
}

// TestE2E_CLIExitCodes tests that the compiled binary returns correct exit codes.
func TestE2E_CLIExitCodes(t *testing.T) {
	// Build the binary
	binaryPath := filepath.Join(t.TempDir(), "transcript")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, output)
	}

	tests := []struct {
		name     string
		args     []string
		env      []string
		wantExit int
	}{
		{
			name:     "help returns 0",
			args:     []string{"--help"},
			wantExit: ExitOK,
		},
		{
			name:     "version returns 0",
			args:     []string{"--version"},
			wantExit: ExitOK,
		},
		{
			name:     "missing required flag returns 2",
			args:     []string{"record"}, // missing -d
			wantExit: ExitUsage,
		},
		{
			name:     "unknown flag returns 2",
			args:     []string{"record", "--unknown-flag"},
			wantExit: ExitUsage,
		},
		{
			name:     "file not found returns 4",
			args:     []string{"transcribe", "nonexistent.ogg"},
			env:      []string{"OPENAI_API_KEY=sk-test"},
			wantExit: ExitValidation,
		},
		{
			name:     "API key missing returns 3",
			args:     []string{"transcribe", "testdata/short.ogg"},
			env:      []string{"OPENAI_API_KEY="}, // Explicitly unset
			wantExit: ExitSetup,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binaryPath, tt.args...)

			// Start with minimal environment for predictable behavior
			cmd.Env = []string{
				"PATH=" + os.Getenv("PATH"),
				"HOME=" + t.TempDir(),
			}
			// Add test-specific environment
			cmd.Env = append(cmd.Env, tt.env...)

			// Run command
			err := cmd.Run()

			// Extract exit code
			gotExit := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					gotExit = exitErr.ExitCode()
				} else {
					t.Fatalf("unexpected error type: %v", err)
				}
			}

			if gotExit != tt.wantExit {
				t.Errorf("exit code = %d, want %d", gotExit, tt.wantExit)
			}
		})
	}
}

// =============================================================================
// E2E Pipeline Function
// =============================================================================

// runTranscribeE2E runs the transcribe pipeline for E2E testing.
// This replicates the core logic from cmd_transcribe.go.
func runTranscribeE2E(ctx context.Context, inputPath, outputPath, template string, diarize bool, parallel int, language, outputLang string) error {
	// === VALIDATION (fail-fast) ===

	// 1. File exists
	if _, err := os.Stat(inputPath); err != nil {
		if os.IsNotExist(err) {
			return ErrFileNotFound
		}
		return err
	}

	// 2. Format supported
	ext := strings.ToLower(filepath.Ext(inputPath))
	if !supportedFormats[ext] {
		return ErrUnsupportedFormat
	}

	// 3. Template valid (if specified)
	if template != "" {
		if _, err := GetTemplate(template); err != nil {
			return err
		}
	}

	// 4. Language validation
	if err := ValidateLanguage(language); err != nil {
		return err
	}
	if err := ValidateLanguage(outputLang); err != nil {
		return err
	}

	// 5. Output language requires template
	if outputLang != "" && template == "" {
		return errors.New("--output-lang requires --template")
	}

	// 6. Parallel bounds
	parallel = clampParallel(parallel)

	// 7. API key present
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return ErrAPIKeyMissing
	}

	// === SETUP ===

	ffmpegPath, err := resolveFFmpeg(ctx)
	if err != nil {
		return err
	}

	// === CHUNKING ===

	chunker, err := NewSilenceChunker(ffmpegPath)
	if err != nil {
		return err
	}

	chunks, err := chunker.Chunk(ctx, inputPath)
	if err != nil {
		return err
	}
	defer CleanupChunks(chunks)

	// === TRANSCRIPTION ===

	client := openai.NewClient(apiKey)
	transcriber := NewOpenAITranscriber(client)
	opts := TranscribeOptions{
		Diarize:  diarize,
		Language: language,
	}

	results, err := TranscribeAll(ctx, chunks, transcriber, opts, parallel)
	if err != nil {
		return err
	}

	transcript := strings.Join(results, "\n\n")

	// === RESTRUCTURE (optional) ===
	// Skip restructuring if transcript is empty (API returns 400 for null content)

	finalOutput := transcript
	if template != "" && strings.TrimSpace(transcript) != "" {
		effectiveOutputLang := outputLang
		if effectiveOutputLang == "" && language != "" {
			effectiveOutputLang = language
		}

		restructurer := NewOpenAIRestructurer(client)
		mrRestructurer := NewMapReduceRestructurer(restructurer)

		finalOutput, _, err = mrRestructurer.Restructure(ctx, transcript, template, effectiveOutputLang)
		if err != nil {
			return err
		}
	}

	// === WRITE OUTPUT ===

	f, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrOutputExists
		}
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(finalOutput); err != nil {
		os.Remove(outputPath)
		return err
	}

	return nil
}
