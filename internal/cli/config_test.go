package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alnah/go-transcript/internal/config"
)

// ---------------------------------------------------------------------------
// Unit tests for helper functions
// ---------------------------------------------------------------------------

func TestIsValidConfigKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		key      string
		expected bool
	}{
		{"valid_output_dir", config.KeyOutputDir, true},
		{"invalid_random", "random-key", false},
		{"invalid_empty", "", false},
		{"invalid_similar", "output_dir", false}, // Wrong format (underscore vs dash)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsValidConfigKey(tt.key)
			if result != tt.expected {
				t.Errorf("IsValidConfigKey(%q) = %v, want %v", tt.key, result, tt.expected)
			}
		})
	}
}

func TestValidConfigKeys(t *testing.T) {
	t.Parallel()

	// Verify validConfigKeys contains expected keys
	found := false
	for _, key := range ValidConfigKeys {
		if key == config.KeyOutputDir {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ValidConfigKeys to contain %q", config.KeyOutputDir)
	}
}

// ---------------------------------------------------------------------------
// Tests for runConfigSet
// ---------------------------------------------------------------------------

func TestRunConfigSet_ValidKey(t *testing.T) {
	// Note: This test modifies the real config file
	// We use t.Setenv to redirect config to temp dir
	// Cannot use t.Parallel() with t.Setenv()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	outputDir := t.TempDir()
	stderr := &syncBuffer{}

	env := &Env{
		Stderr: stderr,
		Getenv: os.Getenv,
	}

	err := RunConfigSet(env, config.KeyOutputDir, outputDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify success message
	output := stderr.String()
	if !strings.Contains(output, "Set") || !strings.Contains(output, config.KeyOutputDir) {
		t.Errorf("expected 'Set output-dir' in output, got %q", output)
	}

	// Verify config was saved
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.OutputDir != outputDir {
		t.Errorf("expected output-dir %q, got %q", outputDir, cfg.OutputDir)
	}
}

func TestRunConfigSet_InvalidKey(t *testing.T) {
	t.Parallel()

	env := &Env{
		Stderr: &syncBuffer{},
	}

	err := RunConfigSet(env, "invalid-key", "value")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("expected 'unknown' in error, got %v", err)
	}
}

func TestRunConfigSet_ExpandsPath(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	// Create a test directory that can be expanded
	testDir := t.TempDir()
	stderr := &syncBuffer{}

	env := &Env{
		Stderr: stderr,
		Getenv: os.Getenv,
	}

	err := RunConfigSet(env, config.KeyOutputDir, testDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify path was stored
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	// Path should be absolute
	if !filepath.IsAbs(cfg.OutputDir) {
		t.Errorf("expected absolute path, got %q", cfg.OutputDir)
	}
}

func TestRunConfigSet_InvalidOutputDir(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	// Create a file (not directory) to cause validation failure
	filePath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(filePath, []byte("file"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	env := &Env{
		Stderr: &syncBuffer{},
		Getenv: os.Getenv,
	}

	err := RunConfigSet(env, config.KeyOutputDir, filePath)
	if err == nil {
		t.Fatal("expected error for file as output-dir")
	}
	if !strings.Contains(err.Error(), "invalid output-dir") {
		t.Errorf("expected 'invalid output-dir' error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests for runConfigGet
// ---------------------------------------------------------------------------

func TestRunConfigGet_ValidKey(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	// Set a value first
	outputDir := t.TempDir()
	if err := config.Save(config.KeyOutputDir, outputDir); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	env := &Env{
		Stderr: &syncBuffer{},
		Getenv: os.Getenv,
	}

	// Capture stdout (runConfigGet prints to stdout, not stderr)
	// Since we can't easily capture stdout in unit tests, we just verify no error
	err := RunConfigGet(env, config.KeyOutputDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunConfigGet_InvalidKey(t *testing.T) {
	t.Parallel()

	env := &Env{
		Stderr: &syncBuffer{},
		Getenv: os.Getenv,
	}

	err := RunConfigGet(env, "invalid-key")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("expected 'unknown' in error, got %v", err)
	}
}

func TestRunConfigGet_EnvFallback(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	envOutputDir := t.TempDir()

	env := &Env{
		Stderr: &syncBuffer{},
		Getenv: staticEnv(map[string]string{
			config.EnvOutputDir: envOutputDir,
		}),
	}

	// No config file - should use env fallback
	err := RunConfigGet(env, config.KeyOutputDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// We can't easily verify the output, but the function should succeed
}

// ---------------------------------------------------------------------------
// Tests for runConfigList
// ---------------------------------------------------------------------------

func TestRunConfigList_WithConfig(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	// Set a value first
	outputDir := t.TempDir()
	if err := config.Save(config.KeyOutputDir, outputDir); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	env := &Env{
		Stderr: &syncBuffer{},
		Getenv: os.Getenv,
	}

	err := RunConfigList(env)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunConfigList_EmptyConfig(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	env := &Env{
		Stderr: &syncBuffer{},
		Getenv: func(string) string { return "" }, // No env vars
	}

	err := RunConfigList(env)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunConfigList_WithEnvOverride(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	envOutputDir := t.TempDir()

	env := &Env{
		Stderr: &syncBuffer{},
		Getenv: staticEnv(map[string]string{
			config.EnvOutputDir: envOutputDir,
		}),
	}

	err := RunConfigList(env)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests for ConfigCmd (Cobra integration)
// ---------------------------------------------------------------------------

func TestConfigCmd_HasSubcommands(t *testing.T) {
	t.Parallel()

	env, _ := testEnv()
	cmd := ConfigCmd(env)

	// Verify subcommands exist
	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}

	expected := []string{"set", "get", "list"}
	for _, name := range expected {
		if !subcommands[name] {
			t.Errorf("expected subcommand %q", name)
		}
	}
}

func TestConfigCmd_SetRequiresArgs(t *testing.T) {
	t.Parallel()

	env, _ := testEnv()
	cmd := ConfigCmd(env)

	cmd.SetArgs([]string{"set"}) // Missing key and value
	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected error when args missing")
	}
}

func TestConfigCmd_SetRequiresTwoArgs(t *testing.T) {
	t.Parallel()

	env, _ := testEnv()
	cmd := ConfigCmd(env)

	cmd.SetArgs([]string{"set", "key"}) // Missing value
	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected error when value missing")
	}
}

func TestConfigCmd_GetRequiresArg(t *testing.T) {
	t.Parallel()

	env, _ := testEnv()
	cmd := ConfigCmd(env)

	cmd.SetArgs([]string{"get"}) // Missing key
	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected error when key missing")
	}
}

func TestConfigCmd_ListNoArgs(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	env, _ := testEnv()
	cmd := ConfigCmd(env)

	cmd.SetArgs([]string{"list"})
	err := cmd.Execute()

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
