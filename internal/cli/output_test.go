package cli

import (
	"bytes"
	"strings"
	"testing"
)

// Notes:
// - Tests cover the warnNonMarkdownExtension function which centralizes
//   the warning logic for non-.md extensions across all CLI commands.
// - Pure function with io.Writer dependency, easy to test without mocking.

// ---------------------------------------------------------------------------
// TestWarnNonMarkdownExtension - Extension warning logic
// ---------------------------------------------------------------------------

func TestWarnNonMarkdownExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		wantWarning bool
		wantContain string // Substring expected in warning (if any)
	}{
		// No warning cases - .md extension
		{
			name:        "md extension lowercase",
			path:        "output.md",
			wantWarning: false,
		},
		{
			name:        "md extension uppercase",
			path:        "output.MD",
			wantWarning: false,
		},
		{
			name:        "md extension mixed case",
			path:        "output.Md",
			wantWarning: false,
		},
		{
			name:        "md with path",
			path:        "/path/to/output.md",
			wantWarning: false,
		},

		// No warning cases - no extension (EnsureExtension would have added .md)
		{
			name:        "no extension",
			path:        "output",
			wantWarning: false,
		},
		{
			name:        "no extension with path",
			path:        "/path/to/output",
			wantWarning: false,
		},

		// Warning cases - non-.md extension
		{
			name:        "txt extension",
			path:        "output.txt",
			wantWarning: true,
			wantContain: ".txt",
		},
		{
			name:        "json extension",
			path:        "output.json",
			wantWarning: true,
			wantContain: ".json",
		},
		{
			name:        "ogg extension",
			path:        "output.ogg",
			wantWarning: true,
			wantContain: ".ogg",
		},
		{
			name:        "TXT uppercase",
			path:        "output.TXT",
			wantWarning: true,
			wantContain: ".txt", // Normalized to lowercase by strings.ToLower
		},
		{
			name:        "non-md with path",
			path:        "/path/to/output.txt",
			wantWarning: true,
			wantContain: ".txt",
		},

		// Edge cases
		{
			name:        "hidden file no extension",
			path:        ".bashrc",
			wantWarning: true, // filepath.Ext(".bashrc") returns ".bashrc"
			wantContain: ".bashrc",
		},
		{
			name:        "dot in middle",
			path:        "file.backup.txt",
			wantWarning: true,
			wantContain: ".txt",
		},
		{
			name:        "empty path",
			path:        "",
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			warnNonMarkdownExtension(&buf, tt.path)

			output := buf.String()
			if tt.wantWarning {
				if output == "" {
					t.Errorf("warnNonMarkdownExtension(%q) wrote nothing, want warning",
						tt.path)
				}
				if !strings.Contains(output, "Warning") {
					t.Errorf("warnNonMarkdownExtension(%q) output missing 'Warning': %q",
						tt.path, output)
				}
				if tt.wantContain != "" && !strings.Contains(output, tt.wantContain) {
					t.Errorf("warnNonMarkdownExtension(%q) output missing %q: %q",
						tt.path, tt.wantContain, output)
				}
			} else {
				if output != "" {
					t.Errorf("warnNonMarkdownExtension(%q) wrote %q, want nothing",
						tt.path, output)
				}
			}
		})
	}
}

func TestWarnNonMarkdownExtensionCaseNormalization(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	warnNonMarkdownExtension(&buf, "output.TXT")
	output := buf.String()

	// Verify lowercase is present
	if !strings.Contains(output, ".txt") {
		t.Errorf("warnNonMarkdownExtension(%q) output = %q, want containing %q", "output.TXT", output, ".txt")
	}

	// Verify uppercase is NOT present (case normalization works)
	if strings.Contains(output, ".TXT") {
		t.Errorf("warnNonMarkdownExtension(%q) output = %q, should not contain %q (case normalization failed)", "output.TXT", output, ".TXT")
	}
}
