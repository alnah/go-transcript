package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/alnah/go-transcript/internal/lang"
	"github.com/alnah/go-transcript/internal/restructure"
	"github.com/alnah/go-transcript/internal/template"
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
		RestructureFunc: func(ctx context.Context, transcript string, tmpl template.Name, outputLang lang.Language) (string, bool, error) {
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

	// Zero provider should default to DeepSeek
	_, err := RestructureContent(context.Background(), env, "content", RestructureOptions{
		Template: template.MustParseName("brainstorm"),
		// Provider omitted - zero value should default to deepseek
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	calls := restructurerFactory.NewMapReducerCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Provider != DeepSeekProvider {
		t.Errorf("expected default provider %q, got %q", DeepSeekProvider, calls[0].Provider)
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
		Template: template.MustParseName("brainstorm"),
		Provider: DeepSeekProvider,
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
		Template: template.MustParseName("brainstorm"),
		Provider: OpenAIProvider,
	})

	if err == nil {
		t.Fatal("expected error for missing OpenAI key")
	}
	if !errors.Is(err, ErrAPIKeyMissing) {
		t.Errorf("expected ErrAPIKeyMissing, got %v", err)
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
		Template: template.MustParseName("brainstorm"),
		Provider: DeepSeekProvider,
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
		RestructureFunc: func(ctx context.Context, transcript string, tmpl template.Name, outputLang lang.Language) (string, bool, error) {
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
		Template: template.MustParseName("brainstorm"),
		Provider: DeepSeekProvider,
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
	if calls[0].TemplateName.String() != "brainstorm" {
		t.Errorf("expected template 'brainstorm', got %q", calls[0].TemplateName)
	}
}

func TestRestructureContent_WithOutputLang(t *testing.T) {
	t.Parallel()

	var capturedLang lang.Language
	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript string, tmpl template.Name, outputLang lang.Language) (string, bool, error) {
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
		Template:   template.MustParseName("meeting"),
		Provider:   DeepSeekProvider,
		OutputLang: lang.MustParse("fr"),
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if capturedLang.String() != "fr" {
		t.Errorf("expected output lang 'fr', got %q", capturedLang.String())
	}
}

func TestRestructureContent_WithProgressCallback(t *testing.T) {
	t.Parallel()

	mockMR := &mockMapReduceRestructurer{
		RestructureFunc: func(ctx context.Context, transcript string, tmpl template.Name, outputLang lang.Language) (string, bool, error) {
			return "restructured", false, nil
		},
	}

	var capturedOpts []restructure.MapReduceOption
	restructurerFactory := &mockRestructurerFactory{
		NewMapReducerFunc: func(provider Provider, apiKey string, opts ...restructure.MapReduceOption) (restructure.MapReducer, error) {
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
		Template: template.MustParseName("brainstorm"),
		Provider: DeepSeekProvider,
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
		RestructureFunc: func(ctx context.Context, transcript string, tmpl template.Name, outputLang lang.Language) (string, bool, error) {
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
		Template: template.MustParseName("brainstorm"),
		Provider: DeepSeekProvider,
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
		provider    Provider
		expectedKey string
	}{
		{"deepseek_uses_deepseek_key", DeepSeekProvider, "test-deepseek-key"},
		{"openai_uses_openai_key", OpenAIProvider, "test-openai-key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockMR := &mockMapReduceRestructurer{
				RestructureFunc: func(ctx context.Context, transcript string, tmpl template.Name, outputLang lang.Language) (string, bool, error) {
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
				Template: template.MustParseName("brainstorm"),
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
