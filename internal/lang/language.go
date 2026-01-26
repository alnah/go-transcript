package lang

import (
	"fmt"
	"strings"
)

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

// Validate checks if the language code is valid.
// Accepts ISO 639-1 codes (e.g., "en", "fr") or locales with valid base (e.g., "pt-BR", "zh-CN").
// Only the base code is validated; regional suffixes are ignored.
// Returns nil for empty string (auto-detect mode).
// Returns ErrInvalid if the base language is not recognized.
func Validate(lang string) error {
	if lang == "" {
		return nil // Empty means auto-detect, which is valid
	}

	base := BaseCode(lang)
	if !validLanguages[base] {
		return fmt.Errorf("invalid language code %q (use ISO 639-1 codes like 'en', 'fr', or locales like 'pt-BR'): %w",
			lang, ErrInvalid)
	}

	return nil
}

// BaseCode extracts the ISO 639-1 base language code from a locale.
// OpenAI's transcription API only accepts base codes, not regional variants.
// Examples: "pt-BR" -> "pt", "zh-CN" -> "zh", "en" -> "en"
func BaseCode(lang string) string {
	if lang == "" {
		return ""
	}
	normalized := Normalize(lang)
	if idx := strings.Index(normalized, "-"); idx != -1 {
		return normalized[:idx]
	}
	return normalized
}

// IsFrench returns true if the language code represents French.
func IsFrench(lang string) bool {
	if lang == "" {
		return false
	}
	normalized := Normalize(lang)
	return normalized == "fr" || strings.HasPrefix(normalized, "fr-")
}

// IsEnglish returns true if the language code represents English.
// Used to skip the output language instruction when output is English
// (since templates are already in English).
func IsEnglish(lang string) bool {
	if lang == "" {
		return false
	}
	normalized := Normalize(lang)
	return normalized == "en" || strings.HasPrefix(normalized, "en-")
}

// DisplayName returns a human-readable name for a language code.
// Lookup order: exact locale match -> base language -> original code.
// Returns empty string for empty input.
// Used in the restructuring prompt instruction.
func DisplayName(lang string) string {
	normalized := Normalize(lang)

	if name, ok := displayNames[normalized]; ok {
		return name
	}

	// Fallback to base language name
	base := BaseCode(lang)
	if name, ok := displayNames[base]; ok {
		return name
	}

	// Last resort: return the code itself
	return lang
}
