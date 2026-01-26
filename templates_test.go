package main

import (
	"strings"
	"testing"
)

// =============================================================================
// GetTemplate - Valid Templates
// =============================================================================

// TestGetTemplate_ValidTemplates verifies all templates from TemplateNames() are valid.
// Uses TemplateNames() as source of truth for automatic synchronization.
func TestGetTemplate_ValidTemplates(t *testing.T) {
	for _, name := range TemplateNames() {
		t.Run(name, func(t *testing.T) {
			prompt, err := GetTemplate(name)
			assertNoError(t, err)
			if len(prompt) == 0 {
				t.Error("expected non-empty prompt")
			}
		})
	}
}

// TestGetTemplate_UsingConstants verifies constants are synchronized with the map.
// This catches mismatches between constants and the templates map.
func TestGetTemplate_UsingConstants(t *testing.T) {
	constants := []string{TemplateBrainstorm, TemplateMeeting, TemplateLecture}

	for _, c := range constants {
		t.Run(c, func(t *testing.T) {
			_, err := GetTemplate(c)
			assertNoError(t, err)
		})
	}
}

// =============================================================================
// GetTemplate - Invalid Templates
// =============================================================================

// TestGetTemplate_InvalidTemplates verifies error handling for invalid names.
// Table-driven for extensibility with explicit case names.
func TestGetTemplate_InvalidTemplates(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"unknown_name", "summary"},
		{"empty_string", ""},
		{"case_sensitive_upper", "Brainstorm"},
		{"case_sensitive_mixed", "BrainStorm"},
		{"typo_underscore", "brain_storm"},
		{"typo_hyphen", "brain-storm"},
		{"whitespace_prefix", " brainstorm"},
		{"whitespace_suffix", "brainstorm "},
		{"whitespace_both", " brainstorm "},
		{"similar_meeting", "meetings"},
		{"similar_lecture", "lectures"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := GetTemplate(tc.input)
			assertError(t, err, ErrUnknownTemplate)
		})
	}
}

// =============================================================================
// TemplateNames
// =============================================================================

// TestTemplateNames_ContentAndOrder verifies the list contains expected names in order.
// Order is specified in the spec: brainstorm, meeting, lecture.
func TestTemplateNames_ContentAndOrder(t *testing.T) {
	names := TemplateNames()
	expected := []string{"brainstorm", "meeting", "lecture"}

	if len(names) != len(expected) {
		t.Fatalf("got %d names, want %d", len(names), len(expected))
	}

	for i, name := range names {
		if name != expected[i] {
			t.Errorf("index %d: got %q, want %q", i, name, expected[i])
		}
	}
}

// TestTemplateNames_ReturnsCopy verifies the function returns a copy, not a reference.
// Mutating the result should not affect subsequent calls.
func TestTemplateNames_ReturnsCopy(t *testing.T) {
	names1 := TemplateNames()
	original := names1[0]
	names1[0] = "corrupted"

	names2 := TemplateNames()
	if names2[0] != original {
		t.Errorf("TemplateNames returned reference instead of copy: got %q, want %q", names2[0], original)
	}
}

// TestTemplateNames_AllNamesAreValidTemplates verifies bidirectional consistency.
// Every name returned by TemplateNames() must work with GetTemplate().
func TestTemplateNames_AllNamesAreValidTemplates(t *testing.T) {
	for _, name := range TemplateNames() {
		_, err := GetTemplate(name)
		if err != nil {
			t.Errorf("TemplateNames() contains %q but GetTemplate(%q) failed: %v", name, name, err)
		}
	}
}

// =============================================================================
// Template Consistency
// =============================================================================

// TestTemplates_ConstantsInNamesList verifies all constants appear in TemplateNames().
func TestTemplates_ConstantsInNamesList(t *testing.T) {
	names := TemplateNames()
	constants := []string{TemplateBrainstorm, TemplateMeeting, TemplateLecture}

	for _, c := range constants {
		found := false
		for _, name := range names {
			if name == c {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("constant %q not found in TemplateNames()", c)
		}
	}
}

// TestTemplates_AllHaveExpectedStructure performs sanity checks on prompt content.
// Detects obvious corruption without coupling to exact wording.
func TestTemplates_AllHaveExpectedStructure(t *testing.T) {
	const minPromptLength = 100 // Prompts should be substantial

	for _, name := range TemplateNames() {
		t.Run(name, func(t *testing.T) {
			prompt, err := GetTemplate(name)
			assertNoError(t, err)

			// Sanity check: prompts should be substantial
			if len(prompt) < minPromptLength {
				t.Errorf("template %q suspiciously short: %d chars (min %d)", name, len(prompt), minPromptLength)
			}

			// All prompts should have a "Rules" section (English templates)
			if !strings.Contains(prompt, "Rules") {
				t.Errorf("template %q missing 'Rules' section", name)
			}

			// All prompts should mention markdown output format
			if !strings.Contains(prompt, "markdown") {
				t.Errorf("template %q missing 'markdown' reference", name)
			}
		})
	}
}

// TestTemplates_NoDuplicatePrompts verifies each template has unique content.
func TestTemplates_NoDuplicatePrompts(t *testing.T) {
	seen := make(map[string]string) // prompt content -> template name

	for _, name := range TemplateNames() {
		prompt, _ := GetTemplate(name)

		if existing, ok := seen[prompt]; ok {
			t.Errorf("templates %q and %q have identical content", existing, name)
		}
		seen[prompt] = name
	}
}
