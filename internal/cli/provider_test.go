package cli

import (
	"errors"
	"fmt"
	"testing"
)

func TestParseProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    Provider
		wantErr bool
	}{
		{
			name:    "deepseek valid",
			input:   "deepseek",
			want:    DeepSeekProvider,
			wantErr: false,
		},
		{
			name:    "openai valid",
			input:   "openai",
			want:    OpenAIProvider,
			wantErr: false,
		},
		{
			name:    "empty string returns error",
			input:   "",
			want:    Provider{},
			wantErr: true,
		},
		{
			name:    "invalid provider returns error",
			input:   "invalid",
			want:    Provider{},
			wantErr: true,
		},
		{
			name:    "case sensitive - DEEPSEEK invalid",
			input:   "DEEPSEEK",
			want:    Provider{},
			wantErr: true,
		},
		{
			name:    "case sensitive - OpenAI invalid",
			input:   "OpenAI",
			want:    Provider{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseProvider(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseProvider(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseProvider(%q) = %v, want %v", tt.input, got, tt.want)
			}
			if tt.wantErr && !errors.Is(err, ErrInvalidProvider) {
				t.Errorf("ParseProvider(%q) error should wrap ErrInvalidProvider, got %v", tt.input, err)
			}
		})
	}
}

func TestMustParseProvider(t *testing.T) {
	t.Parallel()

	t.Run("valid provider does not panic", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("MustParseProvider(\"deepseek\") panicked: %v", r)
			}
		}()

		p := MustParseProvider("deepseek")
		if p != DeepSeekProvider {
			t.Errorf("MustParseProvider(\"deepseek\") = %v, want %v", p, DeepSeekProvider)
		}
	})

	t.Run("invalid provider panics", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r == nil {
				t.Error("MustParseProvider(\"invalid\") did not panic")
			}
		}()

		_ = MustParseProvider("invalid")
	})

	t.Run("empty string panics", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r == nil {
				t.Error("MustParseProvider(\"\") did not panic")
			}
		}()

		_ = MustParseProvider("")
	})
}

func TestProvider_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider Provider
		want     string
	}{
		{"deepseek", DeepSeekProvider, "deepseek"},
		{"openai", OpenAIProvider, "openai"},
		{"zero value", Provider{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.provider.String(); got != tt.want {
				t.Errorf("Provider.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProvider_IsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider Provider
		want     bool
	}{
		{"zero value is zero", Provider{}, true},
		{"deepseek is not zero", DeepSeekProvider, false},
		{"openai is not zero", OpenAIProvider, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.provider.IsZero(); got != tt.want {
				t.Errorf("Provider.IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvider_IsDeepSeek(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider Provider
		want     bool
	}{
		{"deepseek returns true", DeepSeekProvider, true},
		{"openai returns false", OpenAIProvider, false},
		{"zero value returns false", Provider{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.provider.IsDeepSeek(); got != tt.want {
				t.Errorf("Provider.IsDeepSeek() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvider_IsOpenAI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider Provider
		want     bool
	}{
		{"openai returns true", OpenAIProvider, true},
		{"deepseek returns false", DeepSeekProvider, false},
		{"zero value returns false", Provider{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.provider.IsOpenAI(); got != tt.want {
				t.Errorf("Provider.IsOpenAI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvider_PreParsedConstants(t *testing.T) {
	t.Parallel()

	// Verify pre-parsed constants match parsed values
	deepseek, err := ParseProvider("deepseek")
	if err != nil {
		t.Fatalf("ParseProvider(\"deepseek\") failed: %v", err)
	}
	if deepseek != DeepSeekProvider {
		t.Errorf("DeepSeekProvider != ParseProvider(\"deepseek\")")
	}

	openai, err := ParseProvider("openai")
	if err != nil {
		t.Fatalf("ParseProvider(\"openai\") failed: %v", err)
	}
	if openai != OpenAIProvider {
		t.Errorf("OpenAIProvider != ParseProvider(\"openai\")")
	}
}

func TestProvider_OrDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider Provider
		want     Provider
	}{
		{"zero value returns DeepSeek", Provider{}, DeepSeekProvider},
		{"DeepSeek returns itself", DeepSeekProvider, DeepSeekProvider},
		{"OpenAI returns itself", OpenAIProvider, OpenAIProvider},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.provider.OrDefault(); got != tt.want {
				t.Errorf("Provider.OrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestProvider_ImplementsStringer verifies Provider implements fmt.Stringer.
// This is also enforced at compile time in provider.go.
func TestProvider_ImplementsStringer(t *testing.T) {
	t.Parallel()

	var p Provider = DeepSeekProvider
	var _ fmt.Stringer = p

	// Verify String() returns expected value
	s := p.String()
	if s != "deepseek" {
		t.Errorf("DeepSeekProvider.String() = %q, want \"deepseek\"", s)
	}
}
