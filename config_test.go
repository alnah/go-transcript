package main

// NOTE: Do not use t.Parallel() in this file.
// Tests manipulate XDG_CONFIG_HOME environment variable via t.Setenv(),
// which affects process-wide state. Running tests in parallel would cause
// race conditions and unpredictable behavior.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// =============================================================================
// Test Helpers
// =============================================================================

// withXDGConfigHome redirects XDG_CONFIG_HOME to a temp directory for the test.
// Returns the path to the go-transcript config directory (not created yet).
// The caller can create files in this directory as needed.
func withXDGConfigHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// Clear any TRANSCRIPT_OUTPUT_DIR to avoid interference
	t.Setenv("TRANSCRIPT_OUTPUT_DIR", "")
	return filepath.Join(dir, "go-transcript")
}

// createConfigFile creates a config file with the given content.
// Returns the path to the config file.
func createConfigFile(t *testing.T, configDir, content string) string {
	t.Helper()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config directory: %v", err)
	}
	configFile := filepath.Join(configDir, "config")
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
	return configFile
}

// =============================================================================
// TestConfigDir - XDG Support Documentation
// =============================================================================

func TestConfigDir_RespectsXDGConfigHome(t *testing.T) {
	// This test documents that configDir() respects XDG_CONFIG_HOME.
	// This is the primary mechanism used by all other tests for isolation.
	customDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", customDir)

	got, err := configDir()
	assertNoError(t, err)

	want := filepath.Join(customDir, "go-transcript")
	assertEqual(t, got, want)
}

func TestConfigDir_FallbackToHome(t *testing.T) {
	// When XDG_CONFIG_HOME is not set, falls back to ~/.config/go-transcript
	t.Setenv("XDG_CONFIG_HOME", "")

	got, err := configDir()
	assertNoError(t, err)

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "go-transcript")
	assertEqual(t, got, want)
}

// =============================================================================
// TestResolveOutputPath - Pure Function (Table-Driven)
// =============================================================================

func TestResolveOutputPath(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		outputDir   string
		defaultName string
		want        string
	}{
		{
			name:        "absolute_path_ignores_outputDir",
			output:      "/abs/path.md",
			outputDir:   "/should/be/ignored",
			defaultName: "default.md",
			want:        "/abs/path.md",
		},
		{
			name:        "relative_with_outputDir",
			output:      "notes.md",
			outputDir:   "/output",
			defaultName: "default.md",
			want:        filepath.Join("/output", "notes.md"),
		},
		{
			name:        "relative_without_outputDir",
			output:      "notes.md",
			outputDir:   "",
			defaultName: "default.md",
			want:        "notes.md",
		},
		{
			name:        "empty_output_with_outputDir",
			output:      "",
			outputDir:   "/output",
			defaultName: "default.md",
			want:        filepath.Join("/output", "default.md"),
		},
		{
			name:        "empty_output_without_outputDir",
			output:      "",
			outputDir:   "",
			defaultName: "default.md",
			want:        "default.md",
		},
		{
			name:        "nested_relative_path",
			output:      "subdir/notes.md",
			outputDir:   "/output",
			defaultName: "default.md",
			want:        filepath.Join("/output", "subdir", "notes.md"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveOutputPath(tc.output, tc.outputDir, tc.defaultName)
			assertEqual(t, got, tc.want)
		})
	}
}

// =============================================================================
// TestExpandPath - Pure Function (Table-Driven)
// =============================================================================

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home directory: %v", err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "tilde_expansion",
			path: "~/documents/file.txt",
			want: filepath.Join(home, "documents", "file.txt"),
		},
		{
			name: "tilde_only_no_expansion",
			// Only ~/... triggers expansion, not ~ alone
			// This documents current behavior (not a bug)
			path: "~",
			want: "~",
		},
		{
			name: "absolute_path_unchanged",
			path: "/absolute/path",
			want: "/absolute/path",
		},
		{
			name: "relative_path_unchanged",
			path: "relative/path",
			want: "relative/path",
		},
		{
			name: "empty_path",
			path: "",
			want: "",
		},
		{
			name: "tilde_in_middle_unchanged",
			path: "/path/~/file",
			want: "/path/~/file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ExpandPath(tc.path)
			assertEqual(t, got, tc.want)
		})
	}
}

// =============================================================================
// TestParseConfigFile - Parsing Logic
// =============================================================================

func TestParseConfigFile_ValidKeyValue(t *testing.T) {
	configDir := withXDGConfigHome(t)
	configFile := createConfigFile(t, configDir, "output-dir=/path/to/output\n")

	data, err := parseConfigFile(configFile)
	assertNoError(t, err)

	assertEqual(t, data["output-dir"], "/path/to/output")
}

func TestParseConfigFile_MultipleEntries(t *testing.T) {
	configDir := withXDGConfigHome(t)
	content := "key1=value1\nkey2=value2\nkey3=value3\n"
	configFile := createConfigFile(t, configDir, content)

	data, err := parseConfigFile(configFile)
	assertNoError(t, err)

	assertEqual(t, len(data), 3)
	assertEqual(t, data["key1"], "value1")
	assertEqual(t, data["key2"], "value2")
	assertEqual(t, data["key3"], "value3")
}

func TestParseConfigFile_Comments(t *testing.T) {
	configDir := withXDGConfigHome(t)
	content := "# This is a comment\nkey=value\n# Another comment\n"
	configFile := createConfigFile(t, configDir, content)

	data, err := parseConfigFile(configFile)
	assertNoError(t, err)

	assertEqual(t, len(data), 1)
	assertEqual(t, data["key"], "value")
}

func TestParseConfigFile_EmptyLines(t *testing.T) {
	configDir := withXDGConfigHome(t)
	content := "\n\nkey=value\n\n\n"
	configFile := createConfigFile(t, configDir, content)

	data, err := parseConfigFile(configFile)
	assertNoError(t, err)

	assertEqual(t, len(data), 1)
	assertEqual(t, data["key"], "value")
}

func TestParseConfigFile_WhitespaceAroundKeyValue(t *testing.T) {
	configDir := withXDGConfigHome(t)
	content := "  key  =  value  \n"
	configFile := createConfigFile(t, configDir, content)

	data, err := parseConfigFile(configFile)
	assertNoError(t, err)

	assertEqual(t, data["key"], "value")
}

func TestParseConfigFile_ValueWithEquals(t *testing.T) {
	// SplitN with n=2 should preserve equals signs in value
	configDir := withXDGConfigHome(t)
	content := "key=value=with=equals\n"
	configFile := createConfigFile(t, configDir, content)

	data, err := parseConfigFile(configFile)
	assertNoError(t, err)

	assertEqual(t, data["key"], "value=with=equals")
}

func TestParseConfigFile_EmptyValue(t *testing.T) {
	configDir := withXDGConfigHome(t)
	content := "key=\n"
	configFile := createConfigFile(t, configDir, content)

	data, err := parseConfigFile(configFile)
	assertNoError(t, err)

	assertEqual(t, data["key"], "")
}

func TestParseConfigFile_EmptyKey(t *testing.T) {
	// Documents current behavior: empty key is accepted
	// This is not a bug since validConfigKeys won't contain ""
	configDir := withXDGConfigHome(t)
	content := "=value\n"
	configFile := createConfigFile(t, configDir, content)

	data, err := parseConfigFile(configFile)
	assertNoError(t, err)

	assertEqual(t, data[""], "value")
}

func TestParseConfigFile_InvalidSyntax(t *testing.T) {
	configDir := withXDGConfigHome(t)
	content := "valid=line\nno_equals_here\nanother=valid\n"
	configFile := createConfigFile(t, configDir, content)

	_, err := parseConfigFile(configFile)
	if err == nil {
		t.Fatal("expected error for invalid syntax")
	}

	// Error should mention line number
	assertContains(t, err.Error(), "line 2")
	assertContains(t, err.Error(), "no_equals_here")
}

func TestParseConfigFile_FileNotFound(t *testing.T) {
	_, err := parseConfigFile("/nonexistent/path/config")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}

	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got: %v", err)
	}
}

// =============================================================================
// TestLoadConfig - File + Environment Fallback
// =============================================================================

func TestLoadConfig_FileNotExists(t *testing.T) {
	_ = withXDGConfigHome(t) // Sets up empty config dir

	cfg, err := LoadConfig()
	assertNoError(t, err)

	// Empty config, no error
	assertEqual(t, cfg.OutputDir, "")
}

func TestLoadConfig_ValidFile(t *testing.T) {
	configDir := withXDGConfigHome(t)
	createConfigFile(t, configDir, "output-dir=/custom/output\n")

	cfg, err := LoadConfig()
	assertNoError(t, err)

	assertEqual(t, cfg.OutputDir, "/custom/output")
}

func TestLoadConfig_EnvFallback(t *testing.T) {
	_ = withXDGConfigHome(t) // Empty config dir
	t.Setenv("TRANSCRIPT_OUTPUT_DIR", "/env/output")

	cfg, err := LoadConfig()
	assertNoError(t, err)

	assertEqual(t, cfg.OutputDir, "/env/output")
}

func TestLoadConfig_FilePrecedenceOverEnv(t *testing.T) {
	configDir := withXDGConfigHome(t)
	createConfigFile(t, configDir, "output-dir=/file/output\n")
	t.Setenv("TRANSCRIPT_OUTPUT_DIR", "/env/output")

	cfg, err := LoadConfig()
	assertNoError(t, err)

	// File value wins over env
	assertEqual(t, cfg.OutputDir, "/file/output")
}

func TestLoadConfig_InvalidSyntax(t *testing.T) {
	configDir := withXDGConfigHome(t)
	createConfigFile(t, configDir, "invalid_line_no_equals\n")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid syntax")
	}

	assertContains(t, err.Error(), "failed to read config")
}

// =============================================================================
// TestSaveConfigValue - Write with Directory Creation
// =============================================================================

func TestSaveConfigValue_CreatesDirectoryAndFile(t *testing.T) {
	configDir := withXDGConfigHome(t)
	// Directory doesn't exist yet

	err := SaveConfigValue("output-dir", "/test/path")
	assertNoError(t, err)

	// Verify directory was created
	assertFileExists(t, configDir)

	// Verify file was created with correct content
	configFile := filepath.Join(configDir, "config")
	assertFileExists(t, configFile)
	assertFileContains(t, configFile, "output-dir=/test/path")
}

func TestSaveConfigValue_UpdatesExistingValue(t *testing.T) {
	configDir := withXDGConfigHome(t)
	createConfigFile(t, configDir, "output-dir=/old/path\n")

	err := SaveConfigValue("output-dir", "/new/path")
	assertNoError(t, err)

	configFile := filepath.Join(configDir, "config")
	assertFileContains(t, configFile, "output-dir=/new/path")
	assertFileNotContains(t, configFile, "/old/path")
}

func TestSaveConfigValue_PreservesOtherKeys(t *testing.T) {
	configDir := withXDGConfigHome(t)
	createConfigFile(t, configDir, "other-key=other-value\n")

	err := SaveConfigValue("output-dir", "/new/path")
	assertNoError(t, err)

	configFile := filepath.Join(configDir, "config")
	assertFileContains(t, configFile, "output-dir=/new/path")
	assertFileContains(t, configFile, "other-key=other-value")
}

func TestSaveConfigValue_LosesComments(t *testing.T) {
	// Documents that SaveConfigValue does NOT preserve comments,
	// despite what the docstring says. This is the actual behavior.
	// TODO: Either fix the implementation or update the docstring.
	configDir := withXDGConfigHome(t)
	createConfigFile(t, configDir, "# Important comment\noutput-dir=/path\n")

	err := SaveConfigValue("output-dir", "/new/path")
	assertNoError(t, err)

	configFile := filepath.Join(configDir, "config")
	content, _ := os.ReadFile(configFile)
	if strings.Contains(string(content), "# Important comment") {
		t.Error("expected comments to be lost (documenting current behavior)")
	}
}

// =============================================================================
// TestGetConfigValue - Single Key Retrieval
// =============================================================================

func TestGetConfigValue_Exists(t *testing.T) {
	configDir := withXDGConfigHome(t)
	createConfigFile(t, configDir, "output-dir=/test/path\n")

	value, err := GetConfigValue("output-dir")
	assertNoError(t, err)

	assertEqual(t, value, "/test/path")
}

func TestGetConfigValue_NotExists(t *testing.T) {
	configDir := withXDGConfigHome(t)
	createConfigFile(t, configDir, "other-key=value\n")

	value, err := GetConfigValue("output-dir")
	assertNoError(t, err)

	assertEqual(t, value, "")
}

func TestGetConfigValue_FileNotExists(t *testing.T) {
	_ = withXDGConfigHome(t) // Empty dir, no file

	value, err := GetConfigValue("output-dir")
	assertNoError(t, err)

	assertEqual(t, value, "")
}

// =============================================================================
// TestListConfig - All Keys Retrieval
// =============================================================================

func TestListConfig_Empty(t *testing.T) {
	_ = withXDGConfigHome(t) // Empty dir, no file

	data, err := ListConfig()
	assertNoError(t, err)

	assertEqual(t, len(data), 0)
}

func TestListConfig_MultipleKeys(t *testing.T) {
	configDir := withXDGConfigHome(t)
	createConfigFile(t, configDir, "key1=value1\nkey2=value2\n")

	data, err := ListConfig()
	assertNoError(t, err)

	assertEqual(t, len(data), 2)
	assertEqual(t, data["key1"], "value1")
	assertEqual(t, data["key2"], "value2")
}

// =============================================================================
// TestValidOutputDir - Directory Validation with Side Effects
// =============================================================================

func TestValidOutputDir_ExistingDirectory(t *testing.T) {
	dir := t.TempDir()

	err := ValidOutputDir(dir)
	assertNoError(t, err)
}

func TestValidOutputDir_CreatesNonExistent(t *testing.T) {
	parent := t.TempDir()
	newDir := filepath.Join(parent, "new", "nested", "dir")

	err := ValidOutputDir(newDir)
	assertNoError(t, err)

	// Directory should now exist
	info, err := os.Stat(newDir)
	assertNoError(t, err)
	if !info.IsDir() {
		t.Error("expected directory to be created")
	}
}

func TestValidOutputDir_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	// Create a temp dir inside home to test tilde expansion
	testDir := filepath.Join(home, ".go-transcript-test-"+randomSuffix())
	t.Cleanup(func() { os.RemoveAll(testDir) })

	// Use tilde path
	tildePath := "~/" + filepath.Base(testDir)
	err = ValidOutputDir(tildePath)
	assertNoError(t, err)

	// Directory should exist at expanded path
	assertFileExists(t, testDir)
}

func TestValidOutputDir_NotADirectory(t *testing.T) {
	// Create a regular file
	file := tempFile(t, "content")

	err := ValidOutputDir(file)
	if err == nil {
		t.Fatal("expected error for file path")
	}

	assertContains(t, err.Error(), "not a directory")
}

func TestValidOutputDir_EmptyPath(t *testing.T) {
	err := ValidOutputDir("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}

	assertContains(t, err.Error(), "cannot be empty")
}

func TestValidOutputDir_NotWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission test not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("cannot test permissions as root")
	}

	// Create a directory without write permission
	dir := t.TempDir()
	readOnlyDir := filepath.Join(dir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0o555); err != nil {
		t.Fatalf("failed to create readonly dir: %v", err)
	}
	// Ensure we can clean up
	t.Cleanup(func() { os.Chmod(readOnlyDir, 0o755) })

	err := ValidOutputDir(readOnlyDir)
	if err == nil {
		t.Fatal("expected error for non-writable directory")
	}

	assertContains(t, err.Error(), "not writable")
}

// =============================================================================
// Additional Assertion Helpers (not in testhelpers_test.go)
// =============================================================================

// assertFileNotContains checks that the file at path does not contain content.
func assertFileNotContains(t *testing.T, path, content string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("failed to read file %s: %v", path, err)
		return
	}
	if strings.Contains(string(data), content) {
		t.Errorf("file %s should not contain %q", path, content)
	}
}

// randomSuffix generates a simple suffix for test directories.
func randomSuffix() string {
	// Use process ID and a counter for uniqueness without crypto/rand
	return string([]byte{
		byte('a' + (os.Getpid() % 26)),
		byte('a' + ((os.Getpid() / 26) % 26)),
		byte('a' + ((os.Getpid() / 676) % 26)),
	})
}
