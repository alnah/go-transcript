package lang

import (
	"fmt"
	"strings"
)

// Language represents a validated ISO 639-1 language code.
// The zero value represents "auto-detect" mode and is valid.
// Use Parse to create a Language from user input.
type Language struct {
	code string // normalized: lowercase, hyphen separator (e.g., "pt-br")
}

// Parse validates and returns a Language from a string.
// Empty string represents "auto-detect" mode and returns a zero Language.
// Returns ErrInvalid if the language code is not recognized.
func Parse(s string) (Language, error) {
	if s == "" {
		return Language{}, nil // auto-detect
	}

	normalized := Normalize(s)
	base := baseCode(normalized)
	if !validLanguages[base] {
		return Language{}, fmt.Errorf("invalid language code %q (use ISO 639-1 codes like 'en', 'fr', or locales like 'pt-BR'): %w",
			s, ErrInvalid)
	}

	return Language{code: normalized}, nil
}

// MustParse parses a language code and panics if invalid.
// Use only for compile-time constants and tests.
func MustParse(s string) Language {
	l, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return l
}

// String returns the normalized language code, or empty string for auto-detect.
func (l Language) String() string {
	return l.code
}

// IsZero returns true if this is the zero value (auto-detect mode).
// Unlike Provider.IsZero() which indicates an invalid state,
// Language.IsZero() represents a valid "auto-detect" mode where
// the API will automatically detect the language.
func (l Language) IsZero() bool {
	return l.code == ""
}

// IsEnglish returns true if this language is English.
func (l Language) IsEnglish() bool {
	if l.code == "" {
		return false
	}
	return l.code == "en" || strings.HasPrefix(l.code, "en-")
}

// IsFrench returns true if this language is French.
func (l Language) IsFrench() bool {
	if l.code == "" {
		return false
	}
	return l.code == "fr" || strings.HasPrefix(l.code, "fr-")
}

// BaseCode returns the ISO 639-1 base code (without region).
// Returns empty string for auto-detect mode.
func (l Language) BaseCode() string {
	return baseCode(l.code)
}

// DisplayName returns a human-readable name for this language.
// Returns empty string for auto-detect mode.
func (l Language) DisplayName() string {
	if l.code == "" {
		return ""
	}

	if name, ok := displayNames[l.code]; ok {
		return name
	}

	base := l.BaseCode()
	if name, ok := displayNames[base]; ok {
		return name
	}

	return l.code
}

// baseCode extracts the ISO 639-1 base code from a normalized locale.
// This is the internal helper; use Language.BaseCode() for the public API.
// The deprecated package-level BaseCode() function delegates here for backward compatibility.
func baseCode(normalized string) string {
	if normalized == "" {
		return ""
	}
	if idx := strings.Index(normalized, "-"); idx != -1 {
		return normalized[:idx]
	}
	return normalized
}

// validLanguages contains ISO 639-1 language codes supported by OpenAI's transcription API.
// This is not exhaustive but covers the most common languages.
// OpenAI supports additional languages; users can request additions.
var validLanguages = map[string]bool{
	"af": true, // Afrikaans
	"ar": true, // Arabic
	"bg": true, // Bulgarian
	"bn": true, // Bengali
	"ca": true, // Catalan
	"cs": true, // Czech
	"da": true, // Danish
	"de": true, // German
	"el": true, // Greek
	"en": true, // English
	"es": true, // Spanish
	"et": true, // Estonian
	"fa": true, // Persian
	"fi": true, // Finnish
	"fr": true, // French
	"gu": true, // Gujarati
	"he": true, // Hebrew
	"hi": true, // Hindi
	"hr": true, // Croatian
	"hu": true, // Hungarian
	"id": true, // Indonesian
	"it": true, // Italian
	"ja": true, // Japanese
	"kn": true, // Kannada
	"ko": true, // Korean
	"lt": true, // Lithuanian
	"lv": true, // Latvian
	"mk": true, // Macedonian
	"ml": true, // Malayalam
	"mr": true, // Marathi
	"ms": true, // Malay
	"nl": true, // Dutch
	"no": true, // Norwegian
	"pa": true, // Punjabi
	"pl": true, // Polish
	"pt": true, // Portuguese
	"ro": true, // Romanian
	"ru": true, // Russian
	"sk": true, // Slovak
	"sl": true, // Slovenian
	"sr": true, // Serbian
	"sv": true, // Swedish
	"sw": true, // Swahili
	"ta": true, // Tamil
	"te": true, // Telugu
	"th": true, // Thai
	"tl": true, // Tagalog
	"tr": true, // Turkish
	"uk": true, // Ukrainian
	"ur": true, // Urdu
	"vi": true, // Vietnamese
	"zh": true, // Chinese
}

// displayNames maps language codes to human-readable names for user-facing output.
// All base codes from validLanguages are included, plus common regional variants.
// Used by DisplayName to provide friendly names in prompts and messages.
var displayNames = map[string]string{
	// Base languages (aligned with validLanguages)
	"af": "Afrikaans",
	"ar": "Arabic",
	"bg": "Bulgarian",
	"bn": "Bengali",
	"ca": "Catalan",
	"cs": "Czech",
	"da": "Danish",
	"de": "German",
	"el": "Greek",
	"en": "English",
	"es": "Spanish",
	"et": "Estonian",
	"fa": "Persian",
	"fi": "Finnish",
	"fr": "French",
	"gu": "Gujarati",
	"he": "Hebrew",
	"hi": "Hindi",
	"hr": "Croatian",
	"hu": "Hungarian",
	"id": "Indonesian",
	"it": "Italian",
	"ja": "Japanese",
	"kn": "Kannada",
	"ko": "Korean",
	"lt": "Lithuanian",
	"lv": "Latvian",
	"mk": "Macedonian",
	"ml": "Malayalam",
	"mr": "Marathi",
	"ms": "Malay",
	"nl": "Dutch",
	"no": "Norwegian",
	"pa": "Punjabi",
	"pl": "Polish",
	"pt": "Portuguese",
	"ro": "Romanian",
	"ru": "Russian",
	"sk": "Slovak",
	"sl": "Slovenian",
	"sr": "Serbian",
	"sv": "Swedish",
	"sw": "Swahili",
	"ta": "Tamil",
	"te": "Telugu",
	"th": "Thai",
	"tl": "Tagalog",
	"tr": "Turkish",
	"uk": "Ukrainian",
	"ur": "Urdu",
	"vi": "Vietnamese",
	"zh": "Chinese",

	// Regional variants (common locales)
	"en-us": "American English",
	"en-gb": "British English",
	"es-mx": "Mexican Spanish",
	"fr-ca": "Canadian French",
	"pt-br": "Brazilian Portuguese",
	"pt-pt": "European Portuguese",
	"zh-cn": "Simplified Chinese",
	"zh-tw": "Traditional Chinese",
}

// Normalize normalizes a language code to lowercase with hyphen separator.
// Converts underscores to hyphens and lowercases the entire string.
// Does not trim whitespace or validate format.
// Examples: "pt-BR" -> "pt-br", "pt_BR" -> "pt-br", "PT-BR" -> "pt-br"
func Normalize(lang string) string {
	return strings.ToLower(strings.ReplaceAll(lang, "_", "-"))
}
