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

// Normalize normalizes a language code to lowercase with hyphen separator.
// Accepts: "pt-BR", "pt_BR", "PT-BR", "pt-br" -> "pt-br"
func Normalize(lang string) string {
	return strings.ToLower(strings.ReplaceAll(lang, "_", "-"))
}

// Validate checks if the language code is valid.
// Accepts ISO 639-1 codes (e.g., "en", "fr") and locales (e.g., "pt-BR", "zh-CN").
// Returns ErrInvalid if the base language is not recognized.
func Validate(lang string) error {
	if lang == "" {
		return nil // Empty means auto-detect, which is valid
	}

	normalized := Normalize(lang)

	// Extract base language from locale (pt-br -> pt)
	base := normalized
	if idx := strings.Index(normalized, "-"); idx != -1 {
		base = normalized[:idx]
	}

	if !validLanguages[base] {
		return fmt.Errorf("invalid language code %q (use ISO 639-1 codes like 'en', 'fr', 'pt-BR'): %w",
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

// DisplayName returns a human-readable name for common locales.
// Falls back to the code itself for unknown locales.
// Used in the restructuring prompt instruction.
func DisplayName(lang string) string {
	normalized := Normalize(lang)

	// Common locale display names
	displayNames := map[string]string{
		"en":    "English",
		"en-us": "American English",
		"en-gb": "British English",
		"fr":    "French",
		"fr-ca": "Canadian French",
		"es":    "Spanish",
		"es-mx": "Mexican Spanish",
		"pt":    "Portuguese",
		"pt-br": "Brazilian Portuguese",
		"pt-pt": "European Portuguese",
		"zh":    "Chinese",
		"zh-cn": "Simplified Chinese",
		"zh-tw": "Traditional Chinese",
		"de":    "German",
		"it":    "Italian",
		"ja":    "Japanese",
		"ko":    "Korean",
		"ru":    "Russian",
		"ar":    "Arabic",
		"nl":    "Dutch",
		"pl":    "Polish",
		"sv":    "Swedish",
		"da":    "Danish",
		"no":    "Norwegian",
		"fi":    "Finnish",
	}

	if name, ok := displayNames[normalized]; ok {
		return name
	}

	// Extract base language for fallback
	base := normalized
	if idx := strings.Index(normalized, "-"); idx != -1 {
		base = normalized[:idx]
	}

	if name, ok := displayNames[base]; ok {
		return name
	}

	// Last resort: return the code itself
	return lang
}
