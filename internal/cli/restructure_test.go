package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/alnah/go-transcript/internal/restructure"
)

// Notes:
// - Tests focus on restructureContent which is the shared restructuring logic
// - Provider defaulting, API key validation, and template validation are tested
// - The actual restructuring is mocked via mockRestructurerFactory
// - Progress callback is tested via mock inspection

// ---------------------------------------------------------------------------
// Tests for restructureContent - Shared restructuring logic
// ---------------------------------------------------------------------------

func TestRestructureContent_DefaultProvider(t *testing.T) {
	t.Parallel()

	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			return "restructured", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		RestructurerFactory: restructurerFactory,
	}

	// Empty provider should default to DeepSeek
	_, err := RestructureContent(context.Background(), env, "content", RestructureOptions{
		Template: "brainstorm",
		Provider: "", // Empty - should default to deepseek
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	calls := restructurerFactory.NewMapReducerCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Provider != ProviderDeepSeek {
		t.Errorf("expected default provider %q, got %q", ProviderDeepSeek, calls[0].Provider)
	}
}

func TestRestructureContent_DeepSeekMissingKey(t *testing.T) {
	t.Parallel()

	env := &Env{
		Stderr: &syncBuffer{},
		Getenv: func(key string) string {
			if key == EnvOpenAIAPIKey {
				return "openai-key"
			}
			return "" // No DeepSeek key
		},
		RestructurerFactory: &mockRestructurerFactory{},
	}

	_, err := RestructureContent(context.Background(), env, "content", RestructureOptions{
		Template: "brainstorm",
		Provider: ProviderDeepSeek,
	})

	if err == nil {
		t.Fatal("expected error for missing DeepSeek key")
	}
	if !errors.Is(err, ErrDeepSeekKeyMissing) {
		t.Errorf("expected ErrDeepSeekKeyMissing, got %v", err)
	}
}

func TestRestructureContent_OpenAIMissingKey(t *testing.T) {
	t.Parallel()

	env := &Env{
		Stderr: &syncBuffer{},
		Getenv: func(key string) string {
			if key == EnvDeepSeekAPIKey {
				return "deepseek-key"
			}
			return "" // No OpenAI key
		},
		RestructurerFactory: &mockRestructurerFactory{},
	}

	_, err := RestructureContent(context.Background(), env, "content", RestructureOptions{
		Template: "brainstorm",
		Provider: ProviderOpenAI,
	})

	if err == nil {
		t.Fatal("expected error for missing OpenAI key")
	}
	if !errors.Is(err, ErrAPIKeyMissing) {
		t.Errorf("expected ErrAPIKeyMissing, got %v", err)
	}
}

func TestRestructureContent_InvalidProvider(t *testing.T) {
	t.Parallel()

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		RestructurerFactory: &mockRestructurerFactory{},
	}

	_, err := RestructureContent(context.Background(), env, "content", RestructureOptions{
		Template: "brainstorm",
		Provider: "invalid-provider",
	})

	if err == nil {
		t.Fatal("expected error for invalid provider")
	}
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Errorf("expected ErrUnsupportedProvider, got %v", err)
	}
}

func TestRestructureContent_EmptyTemplate(t *testing.T) {
	t.Parallel()

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		RestructurerFactory: &mockRestructurerFactory{},
	}

	_, err := RestructureContent(context.Background(), env, "content", RestructureOptions{
		Template: "", // Empty template
		Provider: ProviderDeepSeek,
	})

	if err == nil {
		t.Fatal("expected error for empty template")
	}
	if err.Error() != "template is required for restructuring" {
		t.Errorf("expected template required error, got %v", err)
	}
}

func TestRestructureContent_FactoryError(t *testing.T) {
	t.Parallel()

	factoryErr := errors.New("factory initialization failed")
	restructurerFactory := &mockRestructurerFactory{
		NewMapReducerErr: factoryErr,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		RestructurerFactory: restructurerFactory,
	}

	_, err := RestructureContent(context.Background(), env, "content", RestructureOptions{
		Template: "brainstorm",
		Provider: ProviderDeepSeek,
	})

	if err == nil {
		t.Fatal("expected error when factory fails")
	}
	if !errors.Is(err, factoryErr) {
		t.Errorf("expected factory error, got %v", err)
	}
}

func TestRestructureContent_Success(t *testing.T) {
	t.Parallel()

	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			return "# Restructured\n\nContent here.", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		RestructurerFactory: restructurerFactory,
	}

	result, err := RestructureContent(context.Background(), env, "raw content", RestructureOptions{
		Template: "brainstorm",
		Provider: ProviderDeepSeek,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "# Restructured\n\nContent here." {
		t.Errorf("expected restructured content, got %q", result)
	}

	// Verify the restructurer was called with correct args
	calls := mockMR.RestructureCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 restructure call, got %d", len(calls))
	}
	if calls[0].Transcript != "raw content" {
		t.Errorf("expected transcript 'raw content', got %q", calls[0].Transcript)
	}
	if calls[0].TemplateName != "brainstorm" {
		t.Errorf("expected template 'brainstorm', got %q", calls[0].TemplateName)
	}
}

func TestRestructureContent_WithOutputLang(t *testing.T) {
	t.Parallel()

	var capturedLang string
	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			capturedLang = outputLang
			return "restructured", false, nil
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		RestructurerFactory: restructurerFactory,
	}

	_, err := RestructureContent(context.Background(), env, "content", RestructureOptions{
		Template:   "meeting",
		Provider:   ProviderDeepSeek,
		OutputLang: "fr",
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if capturedLang != "fr" {
		t.Errorf("expected output lang 'fr', got %q", capturedLang)
	}
}

func TestRestructureContent_WithProgressCallback(t *testing.T) {
	t.Parallel()

	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			return "restructured", false, nil
		},
	}

	var capturedOpts []restructure.MapReduceOption
	restructurerFactory := &mockRestructurerFactory{
		NewMapReducerFunc: func(provider, apiKey string, opts ...restructure.MapReduceOption) (restructure.MapReducer, error) {
			capturedOpts = opts
			return mockMR, nil
		},
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		RestructurerFactory: restructurerFactory,
	}

	_, err := RestructureContent(context.Background(), env, "content", RestructureOptions{
		Template: "brainstorm",
		Provider: ProviderDeepSeek,
		OnProgress: func(phase string, current, total int) {
			// Callback provided to verify option is passed to factory
		},
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify that options were passed to the factory.
	// Note: We only verify the option is passed, not that it's invoked,
	// since the mock doesn't call the callback.
	if len(capturedOpts) == 0 {
		t.Error("expected progress option to be passed to factory")
	}
}

func TestRestructureContent_RestructureError(t *testing.T) {
	t.Parallel()

	restructureErr := errors.New("LLM API error")
	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
			return "", false, restructureErr
		},
	}
	restructurerFactory := &mockRestructurerFactory{
		mockMapReducer: mockMR,
	}

	env := &Env{
		Stderr:              &syncBuffer{},
		Getenv:              defaultTestEnv,
		RestructurerFactory: restructurerFactory,
	}

	_, err := RestructureContent(context.Background(), env, "content", RestructureOptions{
		Template: "brainstorm",
		Provider: ProviderDeepSeek,
	})

	if err == nil {
		t.Fatal("expected error when restructuring fails")
	}
	if !errors.Is(err, restructureErr) {
		t.Errorf("expected restructure error, got %v", err)
	}
}

func TestRestructureContent_CorrectAPIKeyUsed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		provider    string
		expectedKey string
	}{
		{"deepseek_uses_deepseek_key", ProviderDeepSeek, "test-deepseek-key"},
		{"openai_uses_openai_key", ProviderOpenAI, "test-openai-key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockMR := &mockMapReduceRestructurer{
				RestructureFunc: func(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
					return "restructured", false, nil
				},
			}
			restructurerFactory := &mockRestructurerFactory{
				mockMapReducer: mockMR,
			}

			env := &Env{
				Stderr:              &syncBuffer{},
				Getenv:              defaultTestEnv,
				RestructurerFactory: restructurerFactory,
			}

			_, err := RestructureContent(context.Background(), env, "content", RestructureOptions{
				Template: "brainstorm",
				Provider: tt.provider,
			})

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			calls := restructurerFactory.NewMapReducerCalls()
			if len(calls) != 1 {
				t.Fatalf("expected 1 call, got %d", len(calls))
			}
			if calls[0].APIKey != tt.expectedKey {
				t.Errorf("expected API key %q, got %q", tt.expectedKey, calls[0].APIKey)
			}
		})
	}
}
