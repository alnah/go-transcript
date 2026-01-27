package cli

import (
	"errors"
	"fmt"
)

// Provider represents a validated LLM provider for restructuring.
// Zero value is invalid and must not be used.
// Use ParseProvider to create from user input, or the pre-parsed constants.
type Provider struct {
	name string
}

// Compile-time interface compliance check.
var _ fmt.Stringer = Provider{}

// ErrInvalidProvider indicates an invalid provider name was specified.
var ErrInvalidProvider = errors.New("invalid provider")

// Pre-parsed provider constants for use in code.
// These avoid parsing overhead and provide compile-time safety.
var (
	DeepSeekProvider = Provider{name: ProviderDeepSeek}
	OpenAIProvider   = Provider{name: ProviderOpenAI}
)

// validProviders contains the set of valid provider names.
var validProviders = map[string]bool{
	ProviderDeepSeek: true,
	ProviderOpenAI:   true,
}

// ParseProvider validates and parses a provider name string.
// Returns ErrInvalidProvider if the name is not recognized.
// Empty string returns an error (unlike Language where empty means auto-detect).
func ParseProvider(s string) (Provider, error) {
	if s == "" {
		return Provider{}, fmt.Errorf("provider cannot be empty: %w", ErrInvalidProvider)
	}
	if !validProviders[s] {
		return Provider{}, fmt.Errorf("unknown provider %q (use 'deepseek' or 'openai'): %w", s, ErrInvalidProvider)
	}
	return Provider{name: s}, nil
}

// MustParseProvider parses a provider name, panicking if invalid.
// Use only for compile-time constants and tests.
func MustParseProvider(s string) Provider {
	p, err := ParseProvider(s)
	if err != nil {
		panic(err)
	}
	return p
}

// String returns the provider name string.
// Returns empty string for zero value.
func (p Provider) String() string {
	return p.name
}

// IsZero returns true if this is the zero value (no provider set).
// Unlike Language.IsZero() which represents valid "auto-detect" mode,
// Provider.IsZero() indicates an invalid/unset state that must be
// defaulted before use (typically to DeepSeekProvider).
func (p Provider) IsZero() bool {
	return p.name == ""
}

// IsDeepSeek returns true if this provider is DeepSeek.
func (p Provider) IsDeepSeek() bool {
	return p.name == ProviderDeepSeek
}

// IsOpenAI returns true if this provider is OpenAI.
func (p Provider) IsOpenAI() bool {
	return p.name == ProviderOpenAI
}

// OrDefault returns the provider, or DeepSeekProvider if zero.
// Use this to apply the default provider consistently.
func (p Provider) OrDefault() Provider {
	if p.IsZero() {
		return DeepSeekProvider
	}
	return p
}
