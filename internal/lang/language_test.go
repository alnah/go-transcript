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
	"errors"
	"strings"
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

// ---------------------------------------------------------------------------
// TestValidate - Validates language codes against supported ISO 639-1 codes
// ---------------------------------------------------------------------------

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Empty string = auto-detect (valid)
		{name: "empty string auto-detect", input: "", wantErr: false},

		// Valid common languages
		{name: "english", input: "en", wantErr: false},
		{name: "french", input: "fr", wantErr: false},
		{name: "spanish", input: "es", wantErr: false},
		{name: "chinese", input: "zh", wantErr: false},
		{name: "japanese", input: "ja", wantErr: false},

		// Valid less common languages (sample from validLanguages)
		{name: "swahili", input: "sw", wantErr: false},
		{name: "tagalog", input: "tl", wantErr: false},
		{name: "macedonian", input: "mk", wantErr: false},
		{name: "afrikaans", input: "af", wantErr: false},

		// Valid locales (base language is valid)
		{name: "brazilian portuguese", input: "pt-BR", wantErr: false},
		{name: "canadian french", input: "fr-CA", wantErr: false},
		{name: "simplified chinese", input: "zh-CN", wantErr: false},
		{name: "british english", input: "en-GB", wantErr: false},

		// Case variations (should be normalized internally)
		{name: "uppercase", input: "EN", wantErr: false},
		{name: "mixed case locale", input: "Pt-Br", wantErr: false},
		{name: "underscore locale", input: "pt_BR", wantErr: false},

		// Unknown locale suffix with valid base (still valid)
		{name: "unknown locale suffix", input: "en-XXXXX", wantErr: false},
		{name: "french belgium", input: "fr-BE", wantErr: false},

		// Invalid codes
		{name: "invalid two letter", input: "xx", wantErr: true},
		{name: "invalid three letter", input: "xyz", wantErr: true},
		{name: "invalid numeric", input: "123", wantErr: true},
		{name: "invalid single letter", input: "e", wantErr: true},
		{name: "invalid locale with invalid base", input: "xx-YY", wantErr: true},

		// ISO 639-2/3 codes (not supported - we only support ISO 639-1)
		{name: "ISO 639-2 english", input: "eng", wantErr: true},
		{name: "ISO 639-2 french", input: "fra", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := lang.Validate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidate_ErrorWrapsErrInvalid(t *testing.T) {
	t.Parallel()

	err := lang.Validate("xyz")
	if err == nil {
		t.Fatal("Validate(\"xyz\") should return an error")
	}

	if !errors.Is(err, lang.ErrInvalid) {
		t.Errorf("Validate(\"xyz\") error should wrap ErrInvalid, got: %v", err)
	}
}

func TestValidate_ErrorContainsOriginalCode(t *testing.T) {
	t.Parallel()

	err := lang.Validate("XYZ")
	if err == nil {
		t.Fatal("Validate(\"XYZ\") should return an error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "XYZ") {
		t.Errorf("error message should contain original code \"XYZ\", got: %q", errMsg)
	}
}

// ---------------------------------------------------------------------------
// TestBaseCode - Extracts ISO 639-1 base code from locale
// ---------------------------------------------------------------------------

func TestBaseCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Simple codes (no change)
		{name: "english", input: "en", want: "en"},
		{name: "french", input: "fr", want: "fr"},

		// Locales (extract base)
		{name: "brazilian portuguese", input: "pt-BR", want: "pt"},
		{name: "canadian french", input: "fr-CA", want: "fr"},
		{name: "british english", input: "en-GB", want: "en"},
		{name: "simplified chinese", input: "zh-CN", want: "zh"},

		// Normalization applied
		{name: "uppercase", input: "EN", want: "en"},
		{name: "uppercase locale", input: "PT-BR", want: "pt"},
		{name: "underscore locale", input: "pt_BR", want: "pt"},
		{name: "mixed case", input: "Pt-Br", want: "pt"},

		// Edge cases
		{name: "empty string", input: "", want: ""},
		{name: "multiple hyphens takes first part", input: "zh-hans-cn", want: "zh"},
		{name: "multiple underscores takes first part", input: "zh_hans_cn", want: "zh"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lang.BaseCode(tt.input)
			if got != tt.want {
				t.Errorf("BaseCode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestIsFrench - Detects French language codes
// ---------------------------------------------------------------------------

func TestIsFrench(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// True cases
		{name: "french base", input: "fr", want: true},
		{name: "french uppercase", input: "FR", want: true},
		{name: "canadian french", input: "fr-CA", want: true},
		{name: "french france", input: "fr-FR", want: true},
		{name: "french belgium", input: "fr-BE", want: true},
		{name: "underscore variant", input: "fr_CA", want: true},
		{name: "mixed case", input: "Fr-Ca", want: true},

		// False cases
		{name: "empty string", input: "", want: false},
		{name: "english", input: "en", want: false},
		{name: "spanish", input: "es", want: false},

		// Edge cases - codes that start with "fr" but are not French
		// "fro" is Old French (ISO 639-3) - not supported
		{name: "old french ISO 639-3", input: "fro", want: false},
		// "frr" is Northern Frisian - not supported
		{name: "northern frisian", input: "frr", want: false},
		// ISO 639-2 "fra" is not supported
		{name: "ISO 639-2 french", input: "fra", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lang.IsFrench(tt.input)
			if got != tt.want {
				t.Errorf("IsFrench(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestIsEnglish - Detects English language codes
// ---------------------------------------------------------------------------

func TestIsEnglish(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// True cases
		{name: "english base", input: "en", want: true},
		{name: "english uppercase", input: "EN", want: true},
		{name: "american english", input: "en-US", want: true},
		{name: "british english", input: "en-GB", want: true},
		{name: "australian english", input: "en-AU", want: true},
		{name: "underscore variant", input: "en_US", want: true},
		{name: "mixed case", input: "En-Us", want: true},

		// False cases
		{name: "empty string", input: "", want: false},
		{name: "french", input: "fr", want: false},
		{name: "spanish", input: "es", want: false},

		// Edge cases - codes that start with "en" but are not English
		// ISO 639-2 "eng" is not supported
		{name: "ISO 639-2 english", input: "eng", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lang.IsEnglish(tt.input)
			if got != tt.want {
				t.Errorf("IsEnglish(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestDisplayName - Returns human-readable language names
// ---------------------------------------------------------------------------

func TestDisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Exact locale matches
		{name: "english", input: "en", want: "English"},
		{name: "american english", input: "en-us", want: "American English"},
		{name: "british english", input: "en-gb", want: "British English"},
		{name: "french", input: "fr", want: "French"},
		{name: "canadian french", input: "fr-ca", want: "Canadian French"},
		{name: "brazilian portuguese", input: "pt-br", want: "Brazilian Portuguese"},
		{name: "european portuguese", input: "pt-pt", want: "European Portuguese"},
		{name: "simplified chinese", input: "zh-cn", want: "Simplified Chinese"},
		{name: "traditional chinese", input: "zh-tw", want: "Traditional Chinese"},

		// Less common languages (all validLanguages have display names)
		{name: "swahili", input: "sw", want: "Swahili"},
		{name: "tagalog", input: "tl", want: "Tagalog"},
		{name: "macedonian", input: "mk", want: "Macedonian"},
		{name: "gujarati", input: "gu", want: "Gujarati"},

		// Case normalization
		{name: "uppercase english", input: "EN", want: "English"},
		{name: "mixed case locale", input: "Pt-Br", want: "Brazilian Portuguese"},
		{name: "underscore variant", input: "en_US", want: "American English"},

		// Fallback to base language (unknown locale, known base)
		{name: "french belgium fallback", input: "fr-BE", want: "French"},
		{name: "spanish argentina fallback", input: "es-AR", want: "Spanish"},
		{name: "portuguese angola fallback", input: "pt-AO", want: "Portuguese"},

		// Last resort: return original code (unknown language)
		{name: "unknown code", input: "xyz", want: "xyz"},
		{name: "unknown code preserves case", input: "XYZ", want: "XYZ"},
		{name: "unknown locale", input: "xx-YY", want: "xx-YY"},

		// Edge cases
		{name: "empty string", input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lang.DisplayName(tt.input)
			if got != tt.want {
				t.Errorf("DisplayName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
