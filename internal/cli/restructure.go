package cli

import (
	"context"
	"fmt"

	"github.com/alnah/go-transcript/internal/restructure"
)

// RestructureOptions configures transcript restructuring.
type RestructureOptions struct {
	// Template name (required): brainstorm, meeting, lecture, notes
	Template string
	// LLM provider: "deepseek" (default) or "openai"
	Provider string
	// Output language (optional): ISO 639-1 code, empty = English
	OutputLang string
	// Optional progress callback for long transcripts
	OnProgress func(phase string, current, total int)
}

// restructureContent transforms content using a template and LLM.
// Resolves API key internally based on opts.Provider.
// Returns template.ErrUnknown if template is invalid.
// Returns ErrUnsupportedProvider if provider is invalid.
func restructureContent(ctx context.Context, env *Env, content string, opts RestructureOptions) (string, error) {
	// 1. Default provider
	if opts.Provider == "" {
		opts.Provider = ProviderDeepSeek
	}

	// 2. Validate provider and resolve API key
	var apiKey string
	switch opts.Provider {
	case ProviderDeepSeek:
		apiKey = env.Getenv(EnvDeepSeekAPIKey)
		if apiKey == "" {
			return "", fmt.Errorf("%w (set it with: export %s=sk-...)", ErrDeepSeekKeyMissing, EnvDeepSeekAPIKey)
		}
	case ProviderOpenAI:
		apiKey = env.Getenv(EnvOpenAIAPIKey)
		if apiKey == "" {
			return "", fmt.Errorf("%w (set it with: export %s=sk-...)", ErrAPIKeyMissing, EnvOpenAIAPIKey)
		}
	default:
		return "", ErrUnsupportedProvider
	}

	// 3. Validate template (empty check - actual template validation done by MapReducer)
	if opts.Template == "" {
		return "", fmt.Errorf("template is required for restructuring")
	}

	// 4. Create restructurer with options
	var mrOpts []restructure.MapReduceOption
	if opts.OnProgress != nil {
		mrOpts = append(mrOpts, restructure.WithMapReduceProgress(opts.OnProgress))
	}

	mr, err := env.RestructurerFactory.NewMapReducer(opts.Provider, apiKey, mrOpts...)
	if err != nil {
		return "", err
	}

	// 5. Restructure content
	result, _, err := mr.Restructure(ctx, content, opts.Template, opts.OutputLang)
	return result, err
}
