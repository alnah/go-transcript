package main

import (
	"testing"
)

// =============================================================================
// TestNormalizeLanguage
// =============================================================================

func TestNormalizeLanguage(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Canonical cases
		{"simple_code", "en", "en"},
		{"uppercase", "EN", "en"},
		{"locale_with_hyphen", "pt-BR", "pt-br"},
		{"underscore_to_hyphen", "pt_BR", "pt-br"},
		{"uppercase_and_underscore", "PT_BR", "pt-br"},
		{"three_part_locale", "zh-Hans-CN", "zh-hans-cn"},
		{"empty_string", "", ""},

		// Edge cases (document behavior)
		{"double_underscore", "pt__BR", "pt--br"},
		{"leading_space", " en", " en"},    // spaces preserved (not trimmed)
		{"trailing_space", "en ", "en "},   // spaces preserved (not trimmed)
		{"unicode_greek", "ΕΛ", "ελ"},      // Unicode lowercase works
		{"unicode_japanese", "日本語", "日本語"}, // Non-alphabetic unchanged
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeLanguage(tt.input)
			assertEqual(t, got, tt.want)
		})
	}
}

// =============================================================================
// TestValidateLanguage
// =============================================================================

func TestValidateLanguage_ValidCodes(t *testing.T) {
	// All these should return nil (valid)
	tests := []struct {
		name  string
		input string
	}{
		{"empty_auto_detect", ""},
		{"base_code_en", "en"},
		{"base_code_fr", "fr"},
		{"base_code_zh", "zh"},
		{"locale_pt_BR", "pt-BR"},
		{"locale_zh_CN", "zh-CN"},
		{"uppercase_PT_BR", "PT-BR"},
		{"underscore_pt_BR", "pt_BR"},
		// Base-only validation: variant is not checked, only base code
		// This is by design - OpenAI API accepts any variant if base is valid
		{"fantasy_variant_fr_XX", "fr-XX"},
		{"fantasy_variant_en_ZZZZ", "en-ZZZZ"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLanguage(tt.input)
			assertNoError(t, err)
		})
	}
}

func TestValidateLanguage_InvalidCodes(t *testing.T) {
	// All these should return ErrInvalidLanguage
	tests := []struct {
		name  string
		input string
	}{
		{"full_name_english", "english"},
		{"full_name_french", "french"},
		{"unknown_code_xx", "xx"},
		{"unknown_code_xyz", "xyz"},
		{"iso639_2_code_fra", "fra"}, // ISO 639-2, not supported
		{"iso639_2_code_deu", "deu"}, // ISO 639-2, not supported
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLanguage(tt.input)
			assertError(t, err, ErrInvalidLanguage)
		})
	}
}

func TestValidateLanguage_ErrorMessage(t *testing.T) {
	// Verify error message contains helpful information
	err := ValidateLanguage("english")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	// Should contain the invalid input
	assertContains(t, msg, "english")
	// Should contain usage hint
	assertContains(t, msg, "ISO 639-1")
	assertContains(t, msg, "en")
	assertContains(t, msg, "fr")
}

// =============================================================================
// TestBaseLanguageCode
// =============================================================================

func TestBaseLanguageCode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty_string", "", ""},
		{"simple_code", "en", "en"},
		{"locale_extracts_base", "pt-BR", "pt"},
		{"complex_locale", "zh-Hans-CN", "zh"},
		{"uppercase_normalized", "PT-BR", "pt"},
		{"underscore_normalized", "pt_BR", "pt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BaseLanguageCode(tt.input)
			assertEqual(t, got, tt.want)
		})
	}
}

// =============================================================================
// TestIsFrench
// =============================================================================

func TestIsFrench(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Positive cases (French variants)
		{"fr_base", "fr", true},
		{"fr_uppercase", "FR", true},
		{"fr_canada", "fr-CA", true},
		{"fr_france", "fr-FR", true},
		{"fr_underscore", "fr_CA", true},
		{"fr_belgium", "fr-BE", true},

		// Negative cases
		{"empty_string", "", false},
		{"english", "en", false},
		{"portuguese", "pt", false},
		// ISO 639-2 code for French - not detected as French
		// This is by design: module only supports ISO 639-1
		{"iso639_2_fra", "fra", false},
		// Full name - not detected
		{"full_name", "french", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsFrench(tt.input)
			assertEqual(t, got, tt.want)
		})
	}
}

// =============================================================================
// TestLanguageDisplayName
// =============================================================================

func TestLanguageDisplayName_DirectMapping(t *testing.T) {
	// Cases with exact match in displayNames map
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"english", "en", "English"},
		{"american_english", "en-us", "American English"},
		{"british_english", "en-gb", "British English"},
		{"french", "fr", "French"},
		{"canadian_french", "fr-ca", "Canadian French"},
		{"brazilian_portuguese", "pt-br", "Brazilian Portuguese"},
		{"simplified_chinese", "zh-cn", "Simplified Chinese"},
		{"traditional_chinese", "zh-tw", "Traditional Chinese"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LanguageDisplayName(tt.input)
			assertEqual(t, got, tt.want)
		})
	}
}

func TestLanguageDisplayName_BaseFallback(t *testing.T) {
	// Cases where locale is not mapped but base language is
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"australian_english", "en-au", "English"},    // en-au not mapped, falls back to "en"
		{"argentinian_spanish", "es-ar", "Spanish"},   // es-ar not mapped, falls back to "es"
		{"swiss_german", "de-ch", "German"},           // de-ch not mapped, falls back to "de"
		{"quebec_french", "fr-qc", "French"},          // fr-qc not mapped, falls back to "fr"
		{"uppercase_normalized", "EN-AU", "English"},  // uppercase normalized before lookup
		{"underscore_normalized", "en_au", "English"}, // underscore normalized before lookup
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LanguageDisplayName(tt.input)
			assertEqual(t, got, tt.want)
		})
	}
}

func TestLanguageDisplayName_CodeFallback(t *testing.T) {
	// Cases where neither locale nor base is mapped - returns original input
	// Note: line 171 returns `lang` (original), not `normalized`
	// This means case is preserved in the fallback
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"swahili_lowercase", "sw", "sw"},
		{"tagalog", "tl", "tl"},
		{"unknown_code", "xx", "xx"},
		// Uppercase preserved in fallback (this documents current behavior)
		{"swahili_uppercase", "SW", "SW"},
		{"mixed_case", "Sw", "Sw"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LanguageDisplayName(tt.input)
			assertEqual(t, got, tt.want)
		})
	}
}
