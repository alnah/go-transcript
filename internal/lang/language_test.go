package lang_test

// Notes:
// - Black-box testing: all tests use the public API only (lang_test package)
// - Empty string behavior is intentionally tested: "" means "auto-detect" for Validate,
//   and "not specified" for other functions (returns false/empty)
// - validLanguages map coverage: we test a representative sample (common + uncommon + invalid)
//   rather than exhaustive 55+ codes, since the logic is a simple map lookup
// - IsFrench/IsEnglish: we explicitly test ISO 639-2/3 codes (fra, eng, fro) to document
//   that they are NOT supported (ISO 639-1 only)

import (
	"testing"

	"github.com/alnah/go-transcript/internal/lang"
)

// ---------------------------------------------------------------------------
// TestNormalize - Normalizes language codes to lowercase with hyphen separator
// ---------------------------------------------------------------------------

func TestNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Standard cases
		{name: "lowercase code", input: "en", want: "en"},
		{name: "uppercase code", input: "EN", want: "en"},
		{name: "mixed case code", input: "En", want: "en"},

		// Locale with hyphen
		{name: "locale with hyphen lowercase", input: "pt-br", want: "pt-br"},
		{name: "locale with hyphen uppercase", input: "PT-BR", want: "pt-br"},
		{name: "locale with hyphen mixed", input: "pt-BR", want: "pt-br"},

		// Locale with underscore (converted to hyphen)
		{name: "locale with underscore", input: "pt_BR", want: "pt-br"},
		{name: "locale with underscore uppercase", input: "PT_BR", want: "pt-br"},

		// Edge cases
		{name: "empty string", input: "", want: ""},
		{name: "multiple hyphens", input: "zh-hans-cn", want: "zh-hans-cn"},
		{name: "multiple underscores", input: "zh_hans_cn", want: "zh-hans-cn"},
		{name: "mixed separators", input: "zh_hans-CN", want: "zh-hans-cn"},

		// Idempotence: normalizing twice gives same result
		{name: "already normalized", input: "pt-br", want: "pt-br"},

		// Characters not handled (documented behavior)
		{name: "double underscore preserved as double hyphen", input: "pt__BR", want: "pt--br"},
		{name: "spaces not trimmed", input: " en ", want: " en "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lang.Normalize(tt.input)
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalize_Idempotent(t *testing.T) {
	t.Parallel()

	inputs := []string{"EN", "pt_BR", "zh-Hans-CN", "fr-CA", ""}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			once := lang.Normalize(input)
			twice := lang.Normalize(once)
			if once != twice {
				t.Errorf("Normalize is not idempotent: Normalize(%q) = %q, Normalize(%q) = %q",
					input, once, once, twice)
			}
		})
	}
}
