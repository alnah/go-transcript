package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfigFile(t *testing.T) {
	// Create temp config file.
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config")

	tests := []struct {
		name     string
		content  string
		expected map[string]string
		wantErr  bool
	}{
		{
			name:     "empty file",
			content:  "",
			expected: map[string]string{},
		},
		{
			name:     "single value",
			content:  "output-dir=/home/user/transcripts\n",
			expected: map[string]string{"output-dir": "/home/user/transcripts"},
		},
		{
			name:    "multiple values",
			content: "output-dir=/home/user/transcripts\nother-key=other-value\n",
			expected: map[string]string{
				"output-dir": "/home/user/transcripts",
				"other-key":  "other-value",
			},
		},
		{
			name:     "with comments",
			content:  "# This is a comment\noutput-dir=/tmp\n# Another comment\n",
			expected: map[string]string{"output-dir": "/tmp"},
		},
		{
			name:     "with empty lines",
			content:  "\n\noutput-dir=/tmp\n\n",
			expected: map[string]string{"output-dir": "/tmp"},
		},
		{
			name:     "value with equals sign",
			content:  "key=value=with=equals\n",
			expected: map[string]string{"key": "value=with=equals"},
		},
		{
			name:     "whitespace trimmed",
			content:  "  output-dir  =  /tmp/path  \n",
			expected: map[string]string{"output-dir": "/tmp/path"},
		},
		{
			name:    "invalid syntax",
			content: "invalid-line-without-equals\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write test content.
			if err := os.WriteFile(configFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			got, err := parseConfigFile(configFile)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.expected) {
				t.Errorf("got %d values, want %d", len(got), len(tt.expected))
			}
			for k, v := range tt.expected {
				if got[k] != v {
					t.Errorf("got[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestParseConfigFile_NotExists(t *testing.T) {
	_, err := parseConfigFile("/nonexistent/path/config")
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got %v", err)
	}
}

func TestSaveConfigValue(t *testing.T) {
	// Use temp dir as XDG_CONFIG_HOME.
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Save a value.
	if err := SaveConfigValue("output-dir", "/test/path"); err != nil {
		t.Fatalf("SaveConfigValue failed: %v", err)
	}

	// Verify it was saved.
	got, err := GetConfigValue("output-dir")
	if err != nil {
		t.Fatalf("GetConfigValue failed: %v", err)
	}
	if got != "/test/path" {
		t.Errorf("got %q, want %q", got, "/test/path")
	}

	// Update the value.
	if err := SaveConfigValue("output-dir", "/new/path"); err != nil {
		t.Fatalf("SaveConfigValue (update) failed: %v", err)
	}

	got, err = GetConfigValue("output-dir")
	if err != nil {
		t.Fatalf("GetConfigValue failed: %v", err)
	}
	if got != "/new/path" {
		t.Errorf("got %q, want %q", got, "/new/path")
	}
}

func TestGetConfigValue_NotExists(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// No config file exists.
	got, err := GetConfigValue("output-dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestListConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Empty config.
	got, err := ListConfig()
	if err != nil {
		t.Fatalf("ListConfig failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}

	// Add some values.
	_ = SaveConfigValue("key1", "value1")
	_ = SaveConfigValue("key2", "value2")

	got, err = ListConfig()
	if err != nil {
		t.Fatalf("ListConfig failed: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 values, got %d", len(got))
	}
}

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// No config file - should use env var fallback.
	t.Setenv(envOutputDir, "/env/path")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.OutputDir != "/env/path" {
		t.Errorf("expected env fallback, got %q", cfg.OutputDir)
	}

	// Config file takes precedence over env var.
	_ = SaveConfigValue("output-dir", "/config/path")

	cfg, err = LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.OutputDir != "/config/path" {
		t.Errorf("expected config file value, got %q", cfg.OutputDir)
	}
}

func TestResolveOutputPath(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		outputDir   string
		defaultName string
		expected    string
	}{
		{
			name:        "absolute output ignores outputDir",
			output:      "/absolute/path/file.md",
			outputDir:   "/some/dir",
			defaultName: "default.md",
			expected:    "/absolute/path/file.md",
		},
		{
			name:        "relative output combined with outputDir",
			output:      "file.md",
			outputDir:   "/output/dir",
			defaultName: "default.md",
			expected:    "/output/dir/file.md",
		},
		{
			name:        "relative output without outputDir",
			output:      "file.md",
			outputDir:   "",
			defaultName: "default.md",
			expected:    "file.md",
		},
		{
			name:        "no output uses default in outputDir",
			output:      "",
			outputDir:   "/output/dir",
			defaultName: "default.md",
			expected:    "/output/dir/default.md",
		},
		{
			name:        "no output no outputDir uses default",
			output:      "",
			outputDir:   "",
			defaultName: "default.md",
			expected:    "default.md",
		},
		{
			name:        "relative path with subdirectory",
			output:      "subdir/file.md",
			outputDir:   "/output/dir",
			defaultName: "default.md",
			expected:    "/output/dir/subdir/file.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveOutputPath(tt.output, tt.outputDir, tt.defaultName)
			if got != tt.expected {
				t.Errorf("ResolveOutputPath(%q, %q, %q) = %q, want %q",
					tt.output, tt.outputDir, tt.defaultName, got, tt.expected)
			}
		})
	}
}

func TestValidOutputDir(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		err := ValidOutputDir("")
		if err == nil {
			t.Error("expected error for empty string")
		}
	})

	t.Run("existing writable directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := ValidOutputDir(tmpDir)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("non-existent directory is created", func(t *testing.T) {
		tmpDir := t.TempDir()
		newDir := filepath.Join(tmpDir, "new", "nested", "dir")

		err := ValidOutputDir(newDir)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify directory was created.
		if _, err := os.Stat(newDir); os.IsNotExist(err) {
			t.Error("directory was not created")
		}
	})

	t.Run("path is a file not directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "file.txt")
		if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}

		err := ValidOutputDir(filePath)
		if err == nil {
			t.Error("expected error for file path")
		}
	})
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{"~/test", filepath.Join(home, "test")},
		{"~/", filepath.Join(home, "")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~notexpanded", "~notexpanded"}, // Only ~/... is expanded
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ExpandPath(tt.input)
			if got != tt.expected {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestConfigDir_XDG(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir, err := configDir()
	if err != nil {
		t.Fatalf("configDir failed: %v", err)
	}

	expected := filepath.Join(tmpDir, "go-transcript")
	if dir != expected {
		t.Errorf("configDir() = %q, want %q", dir, expected)
	}
}

func TestIsValidConfigKey(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{ConfigKeyOutputDir, true},
		{"output-dir", true},
		{"unknown-key", false},
		{"", false},
		{"OUTPUT-DIR", false}, // Case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := isValidConfigKey(tt.key)
			if got != tt.expected {
				t.Errorf("isValidConfigKey(%q) = %v, want %v", tt.key, got, tt.expected)
			}
		})
	}
}
