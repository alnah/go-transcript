package main

import (
	"errors"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func TestNormalizeLanguage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"en", "en"},
		{"EN", "en"},
		{"pt-BR", "pt-br"},
		{"pt_BR", "pt-br"},
		{"PT_BR", "pt-br"},
		{"zh-CN", "zh-cn"},
		{"fr-CA", "fr-ca"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeLanguage(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeLanguage(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestValidateLanguage(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{"empty is valid (auto-detect)", "", false},
		{"english", "en", false},
		{"french", "fr", false},
		{"portuguese", "pt", false},
		{"brazilian portuguese", "pt-BR", false},
		{"brazilian portuguese lowercase", "pt-br", false},
		{"brazilian portuguese underscore", "pt_BR", false},
		{"chinese simplified", "zh-CN", false},
		{"chinese traditional", "zh-TW", false},
		{"spanish", "es", false},
		{"german", "de", false},
		{"japanese", "ja", false},
		{"korean", "ko", false},
		{"russian", "ru", false},
		{"arabic", "ar", false},
		{"invalid code", "xyz", true},
		{"full word english", "english", true},
		{"full word french", "french", true},
		{"three letter code", "eng", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLanguage(tt.input)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateLanguage(%q) expected error, got nil", tt.input)
				}
				if !errors.Is(err, ErrInvalidLanguage) {
					t.Errorf("ValidateLanguage(%q) error = %v, want ErrInvalidLanguage", tt.input, err)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateLanguage(%q) unexpected error: %v", tt.input, err)
				}
			}
		})
	}
}

func TestIsFrench(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"fr", true},
		{"FR", true},
		{"fr-FR", true},
		{"fr-CA", true},
		{"fr_CA", true},
		{"fr-BE", true},
		{"en", false},
		{"en-US", false},
		{"pt-BR", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsFrench(tt.input)
			if got != tt.expected {
				t.Errorf("IsFrench(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLanguageDisplayName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"en", "English"},
		{"EN", "English"},
		{"en-us", "American English"},
		{"en-US", "American English"},
		{"en-gb", "British English"},
		{"fr", "French"},
		{"fr-ca", "Canadian French"},
		{"pt", "Portuguese"},
		{"pt-br", "Brazilian Portuguese"},
		{"pt-BR", "Brazilian Portuguese"},
		{"pt-pt", "European Portuguese"},
		{"zh", "Chinese"},
		{"zh-cn", "Simplified Chinese"},
		{"zh-tw", "Traditional Chinese"},
		{"de", "German"},
		{"ja", "Japanese"},
		{"ko", "Korean"},
		// Unknown locale falls back to base language
		{"en-au", "English"},
		// Unknown language returns the code
		{"xyz", "xyz"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := LanguageDisplayName(tt.input)
			if got != tt.expected {
				t.Errorf("LanguageDisplayName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRestructurer_OutputLanguageInstruction(t *testing.T) {
	tests := []struct {
		name           string
		outputLang     string
		wantInstruction bool
		wantContains   string
	}{
		{"empty lang - no instruction", "", false, ""},
		{"french - no instruction (native)", "fr", false, ""},
		{"french canada - no instruction (native)", "fr-CA", false, ""},
		{"english - adds instruction", "en", true, "Respond in English"},
		{"brazilian portuguese - adds instruction", "pt-BR", true, "Respond in Brazilian Portuguese"},
		{"german - adds instruction", "de", true, "Respond in German"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockChatCompleter{
				response: successResponse("output"),
			}

			r := NewOpenAIRestructurer(nil,
				withChatCompleter(mock),
				withTemplateResolver(func(name string) (string, error) {
					return "Test template prompt", nil
				}),
			)

			_, err := r.Restructure(t.Context(), "transcript", "test", tt.outputLang)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			systemPrompt := mock.captured.Messages[0].Content
			if tt.wantInstruction {
				if !contains(systemPrompt, tt.wantContains) {
					t.Errorf("system prompt should contain %q, got: %s", tt.wantContains, systemPrompt)
				}
				// Instruction should be at the beginning
				if !hasPrefix(systemPrompt, "Respond in") {
					t.Errorf("instruction should be at the beginning, got: %s", systemPrompt)
				}
			} else {
				if contains(systemPrompt, "Respond in") {
					t.Errorf("system prompt should not contain language instruction for %q, got: %s", tt.outputLang, systemPrompt)
				}
			}
		})
	}
}

// Helper to create success response
func successResponse(content string) openai.ChatCompletionResponse {
	return openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: content}},
		},
	}
}

// Helper for string contains (avoid importing strings in test)
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Helper for string prefix check
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
