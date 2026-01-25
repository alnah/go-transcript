package main

import (
	"errors"
	"slices"
	"testing"
)

func TestGetTemplate_Valid(t *testing.T) {
	tests := []struct {
		name     string
		template string
		wantLen  int // minimum expected length
	}{
		{"brainstorm", TemplateBrainstorm, 100},
		{"meeting", TemplateMeeting, 100},
		{"lecture", TemplateLecture, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetTemplate(tt.template)
			if err != nil {
				t.Fatalf("GetTemplate(%q) returned error: %v", tt.template, err)
			}
			if len(got) < tt.wantLen {
				t.Errorf("GetTemplate(%q) returned prompt with len %d, want at least %d",
					tt.template, len(got), tt.wantLen)
			}
		})
	}
}

func TestGetTemplate_Invalid(t *testing.T) {
	invalidNames := []string{
		"unknown",
		"Brainstorm", // case-sensitive
		"MEETING",
		"",
		"summary",
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			_, err := GetTemplate(name)
			if err == nil {
				t.Fatalf("GetTemplate(%q) should return error", name)
			}
			if !errors.Is(err, ErrUnknownTemplate) {
				t.Errorf("GetTemplate(%q) error = %v, want ErrUnknownTemplate", name, err)
			}
		})
	}
}

func TestTemplateNames_ReturnsAllInOrder(t *testing.T) {
	got := TemplateNames()
	want := []string{TemplateBrainstorm, TemplateMeeting, TemplateLecture}

	if !slices.Equal(got, want) {
		t.Errorf("TemplateNames() = %v, want %v", got, want)
	}
}

func TestTemplateNames_ReturnsCopy(t *testing.T) {
	names1 := TemplateNames()
	names1[0] = "modified"

	names2 := TemplateNames()
	if names2[0] == "modified" {
		t.Error("TemplateNames() should return a copy, not the original slice")
	}
}

func TestTemplates_NotEmpty(t *testing.T) {
	for _, name := range TemplateNames() {
		t.Run(name, func(t *testing.T) {
			prompt, err := GetTemplate(name)
			if err != nil {
				t.Fatalf("GetTemplate(%q) returned error: %v", name, err)
			}
			if prompt == "" {
				t.Errorf("template %q has empty prompt", name)
			}
		})
	}
}

func TestTemplates_AllConstantsInMap(t *testing.T) {
	// Verify all exported constants have corresponding entries in the map.
	constants := []string{
		TemplateBrainstorm,
		TemplateMeeting,
		TemplateLecture,
	}

	for _, c := range constants {
		t.Run(c, func(t *testing.T) {
			if _, ok := templates[c]; !ok {
				t.Errorf("constant %q has no entry in templates map", c)
			}
		})
	}

	// Verify templateOrder matches constants.
	if !slices.Equal(templateOrder, constants) {
		t.Errorf("templateOrder = %v, want %v", templateOrder, constants)
	}
}
