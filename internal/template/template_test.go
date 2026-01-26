package template_test

// Notes:
// - Black-box testing: we test through the public API only (Get, Names, constants)
// - We deliberately do NOT test prompt content details (fragile, implementation detail)
// - We only verify prompts are non-empty, which is the observable contract
// - Case-sensitivity is a feature: the exported constants are the intended API

import (
	"errors"
	"testing"

	"github.com/alnah/go-transcript/internal/template"
)

// ---------------------------------------------------------------------------
// TestGet_ValidTemplates - Happy path: known templates return non-empty prompts
// ---------------------------------------------------------------------------

func TestGet_ValidTemplates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		templateName string
	}{
		{"brainstorm constant", template.Brainstorm},
		{"meeting constant", template.Meeting},
		{"lecture constant", template.Lecture},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prompt, err := template.Get(tt.templateName)

			if err != nil {
				t.Errorf("Get(%q) returned error: %v", tt.templateName, err)
			}
			if prompt == "" {
				t.Errorf("Get(%q) returned empty prompt", tt.templateName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestGet_InvalidTemplates - Error cases: unknown names return ErrUnknown
// ---------------------------------------------------------------------------

func TestGet_InvalidTemplates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		templateName string
	}{
		{"unknown name", "unknown"},
		{"empty string", ""},
		{"wrong case uppercase", "BRAINSTORM"},
		{"wrong case mixed", "Brainstorm"},
		{"wrong case meeting", "MEETING"},
		{"with spaces", " brainstorm"},
		{"similar but wrong", "brainstorming"},
		{"special characters", "brain-storm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prompt, err := template.Get(tt.templateName)

			if err == nil {
				t.Errorf("Get(%q) expected error, got prompt of length %d", tt.templateName, len(prompt))
			}
			if !errors.Is(err, template.ErrUnknown) {
				t.Errorf("Get(%q) error = %v, want errors.Is(err, ErrUnknown)", tt.templateName, err)
			}
			if prompt != "" {
				t.Errorf("Get(%q) returned non-empty prompt on error: %q", tt.templateName, prompt)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestNames_ReturnsCanonicalOrder - Names returns the documented order
// ---------------------------------------------------------------------------

func TestNames_ReturnsCanonicalOrder(t *testing.T) {
	t.Parallel()

	got := template.Names()
	want := []string{template.Brainstorm, template.Meeting, template.Lecture}

	if len(got) != len(want) {
		t.Fatalf("Names() returned %d elements, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Names()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// TestNames_ReturnsCopy - Names returns a defensive copy, not the internal slice
// ---------------------------------------------------------------------------

func TestNames_ReturnsCopy(t *testing.T) {
	t.Parallel()

	// Get first copy and modify it
	first := template.Names()
	original := first[0]
	first[0] = "hacked"

	// Get second copy - should be unaffected
	second := template.Names()

	if second[0] != original {
		t.Errorf("Names() returned shared slice: modification affected subsequent calls")
		t.Errorf("Expected %q, got %q", original, second[0])
	}
}

// ---------------------------------------------------------------------------
// TestConsistency_NamesAndGetAreCoherent - Every name from Names() is valid for Get()
// ---------------------------------------------------------------------------

func TestConsistency_NamesAndGetAreCoherent(t *testing.T) {
	t.Parallel()

	names := template.Names()

	if len(names) == 0 {
		t.Fatal("Names() returned empty slice")
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			prompt, err := template.Get(name)

			if err != nil {
				t.Errorf("Get(%q) failed for name returned by Names(): %v", name, err)
			}
			if prompt == "" {
				t.Errorf("Get(%q) returned empty prompt for name returned by Names()", name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestConstants_MatchExpectedValues - Exported constants have expected values
// ---------------------------------------------------------------------------

func TestConstants_MatchExpectedValues(t *testing.T) {
	t.Parallel()

	// This test documents that the constants are lowercase strings
	// If someone changes the constant values, this test will catch it
	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{"Brainstorm", template.Brainstorm, "brainstorm"},
		{"Meeting", template.Meeting, "meeting"},
		{"Lecture", template.Lecture, "lecture"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.constant != tt.want {
				t.Errorf("template.%s = %q, want %q", tt.name, tt.constant, tt.want)
			}
		})
	}
}
