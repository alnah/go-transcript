package restructure_test

// Notes:
// - Shared tests for MapReduce, splitTranscript, buildMapPrompt
// - Tests use black-box approach via package restructure_test
// - Internal functions are tested via export_test.go exports
// - OpenAI-specific tests are in openai_test.go
// - DeepSeek-specific tests are in deepseek_test.go

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/alnah/go-transcript/internal/lang"
	"github.com/alnah/go-transcript/internal/restructure"
	"github.com/alnah/go-transcript/internal/template"
)

// ---------------------------------------------------------------------------
// TestSplitTranscript - Transcript splitting
// ---------------------------------------------------------------------------

func TestSplitTranscript(t *testing.T) {
	t.Parallel()

	// Helper to create text of approximately n tokens (n*3 chars)
	makeText := func(tokens int) string {
		return strings.Repeat("a", tokens*3)
	}

	tests := []struct {
		name       string
		transcript string
		maxTokens  int
		wantNil    bool
		wantChunks int
	}{
		{
			name:       "short transcript returns nil",
			transcript: makeText(100),
			maxTokens:  200,
			wantNil:    true,
		},
		{
			name:       "exact fit returns nil",
			transcript: makeText(100),
			maxTokens:  100,
			wantNil:    true,
		},
		{
			name:       "two paragraphs split into two chunks",
			transcript: makeText(100) + "\n\n" + makeText(100),
			maxTokens:  120,
			wantChunks: 2,
		},
		{
			name:       "three paragraphs split into three chunks",
			transcript: makeText(100) + "\n\n" + makeText(100) + "\n\n" + makeText(100),
			maxTokens:  120,
			wantChunks: 3,
		},
		{
			name:       "giant single paragraph included anyway",
			transcript: makeText(500),
			maxTokens:  100,
			wantNil:    true, // Only 1 chunk, so returns nil
		},
		{
			name:       "giant paragraph with small one",
			transcript: makeText(500) + "\n\n" + makeText(50),
			maxTokens:  100,
			wantChunks: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := restructure.SplitTranscript(tt.transcript, tt.maxTokens)

			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %d chunks", len(got))
				}
				return
			}

			if len(got) != tt.wantChunks {
				t.Errorf("got %d chunks, want %d", len(got), tt.wantChunks)
				return
			}

			// Verify Index and Total are set correctly
			for i, chunk := range got {
				if chunk.Index != i {
					t.Errorf("chunk %d has Index=%d", i, chunk.Index)
				}
				if chunk.Total != tt.wantChunks {
					t.Errorf("chunk %d has Total=%d, want %d", i, chunk.Total, tt.wantChunks)
				}
				if chunk.Content == "" {
					t.Errorf("chunk %d has empty content", i)
				}
			}
		})
	}
}

func TestSplitTranscript_PreservesParagraphBoundaries(t *testing.T) {
	t.Parallel()

	para1 := "First paragraph content here."
	para2 := "Second paragraph content here."
	para3 := "Third paragraph content here."

	transcript := para1 + "\n\n" + para2 + "\n\n" + para3

	// Set maxTokens low enough to force splitting but high enough for one para
	chunks := restructure.SplitTranscript(transcript, 15)

	if chunks == nil {
		t.Fatal("expected chunks, got nil")
	}

	// Verify paragraphs are not split mid-sentence
	for i, chunk := range chunks {
		if strings.Contains(chunk.Content, "\n\n") {
			// If a chunk contains paragraph separator, it should be at most
			// two paragraphs that fit together
			t.Logf("chunk %d contains multiple paragraphs (may be OK if they fit): %q", i, chunk.Content)
		}
	}
}

// ---------------------------------------------------------------------------
// TestBuildMapPrompt - Prompt formatting
// ---------------------------------------------------------------------------

func TestBuildMapPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		basePrompt string
		chunk      restructure.TranscriptChunk
		wantPart   string
		wantTotal  string
		wantBase   bool
	}{
		{
			name:       "first of three",
			basePrompt: "Restructure this meeting.",
			chunk:      restructure.TranscriptChunk{Index: 0, Total: 3},
			wantPart:   "part 1 of 3",
			wantTotal:  "3",
			wantBase:   true,
		},
		{
			name:       "second of three",
			basePrompt: "Restructure this meeting.",
			chunk:      restructure.TranscriptChunk{Index: 1, Total: 3},
			wantPart:   "part 2 of 3",
			wantTotal:  "3",
			wantBase:   true,
		},
		{
			name:       "last of five",
			basePrompt: "Custom prompt.",
			chunk:      restructure.TranscriptChunk{Index: 4, Total: 5},
			wantPart:   "part 5 of 5",
			wantTotal:  "5",
			wantBase:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := restructure.BuildMapPrompt(tt.basePrompt, tt.chunk)

			if !strings.Contains(got, tt.wantPart) {
				t.Errorf("prompt should contain %q, got: %s", tt.wantPart, got)
			}

			if tt.wantBase && !strings.Contains(got, tt.basePrompt) {
				t.Errorf("prompt should contain base prompt %q", tt.basePrompt)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestMapReduceRestructurer_Restructure - MapReduce orchestration
// ---------------------------------------------------------------------------

func TestMapReduceRestructurer_Restructure(t *testing.T) {
	t.Parallel()

	t.Run("short transcript skips MapReduce", func(t *testing.T) {
		t.Parallel()

		mock := &mockChatCompleter{
			response: successResponse("Simple result."),
		}

		base := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(mock),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		mr := restructure.NewMapReduceRestructurer(base,
			restructure.WithMapReduceMaxTokens(1000), // High limit
		)

		result, usedMapReduce, err := mr.Restructure(context.Background(), "Short transcript.", template.MustParseName("meeting"), lang.Language{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if usedMapReduce {
			t.Error("should not use MapReduce for short transcript")
		}

		if result != "Simple result." {
			t.Errorf("unexpected result: %s", result)
		}

		if mock.CallCount() != 1 {
			t.Errorf("expected 1 API call, got %d", mock.CallCount())
		}
	})

	t.Run("long transcript uses MapReduce", func(t *testing.T) {
		t.Parallel()

		// Create paragraphs that will split into 2 chunks
		para1 := strings.Repeat("a", 300) // ~100 tokens
		para2 := strings.Repeat("b", 300) // ~100 tokens
		transcript := para1 + "\n\n" + para2

		var callsMu sync.Mutex
		var calls []string

		// We need to intercept calls to verify MapReduce behavior
		interceptMock := &interceptingMock{
			responses: []string{
				"# Part 1 Result",
				"# Part 2 Result",
				"# Merged Final Result",
			},
			calls: &calls,
			mu:    &callsMu,
		}

		base := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(interceptMock),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		mr := restructure.NewMapReduceRestructurer(base,
			restructure.WithMapReduceMaxTokens(50), // Force splitting
		)

		result, usedMapReduce, err := mr.Restructure(context.Background(), transcript, template.MustParseName("meeting"), lang.Language{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !usedMapReduce {
			t.Error("should use MapReduce for long transcript")
		}

		callsMu.Lock()
		numCalls := len(calls)
		callsMu.Unlock()

		// Expect: 2 map calls + 1 reduce call = 3 total
		if numCalls != 3 {
			t.Errorf("expected 3 API calls (2 map + 1 reduce), got %d", numCalls)
		}

		if result != "# Merged Final Result" {
			t.Errorf("unexpected result: %s", result)
		}
	})

	t.Run("progress callback is invoked", func(t *testing.T) {
		t.Parallel()

		interceptMock := &interceptingMock{
			responses: []string{"chunk1", "chunk2", "merged"},
			calls:     &[]string{},
			mu:        &sync.Mutex{},
		}

		base := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(interceptMock),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		var progressCalls []string
		var progressMu sync.Mutex

		mr := restructure.NewMapReduceRestructurer(base,
			restructure.WithMapReduceMaxTokens(50),
			restructure.WithMapReduceProgress(func(phase string, current, total int) {
				progressMu.Lock()
				progressCalls = append(progressCalls, fmt.Sprintf("%s:%d/%d", phase, current, total))
				progressMu.Unlock()
			}),
		)

		para1 := strings.Repeat("a", 300)
		para2 := strings.Repeat("b", 300)
		transcript := para1 + "\n\n" + para2

		_, _, err := mr.Restructure(context.Background(), transcript, template.MustParseName("meeting"), lang.Language{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		progressMu.Lock()
		defer progressMu.Unlock()

		// Should have map:1/2, map:2/2, reduce:1/1
		if len(progressCalls) < 3 {
			t.Errorf("expected at least 3 progress calls, got %d: %v", len(progressCalls), progressCalls)
		}

		hasMap := false
		hasReduce := false
		for _, call := range progressCalls {
			if strings.HasPrefix(call, "map:") {
				hasMap = true
			}
			if strings.HasPrefix(call, "reduce:") {
				hasReduce = true
			}
		}

		if !hasMap {
			t.Error("expected map phase in progress calls")
		}
		if !hasReduce {
			t.Error("expected reduce phase in progress calls")
		}
	})

	t.Run("context cancellation stops processing", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())

		// Mock that cancels context on first call
		cancellingMock := &cancellingMock{
			cancel: cancel,
		}

		base := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(cancellingMock),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		mr := restructure.NewMapReduceRestructurer(base,
			restructure.WithMapReduceMaxTokens(50),
		)

		para1 := strings.Repeat("a", 300)
		para2 := strings.Repeat("b", 300)
		transcript := para1 + "\n\n" + para2

		_, _, err := mr.Restructure(ctx, transcript, template.MustParseName("meeting"), lang.Language{})
		if err == nil {
			t.Fatal("expected error after context cancellation")
		}

		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	})

	// Note: "template error is returned" test removed.
	// With the template.Name type, invalid templates are caught at parse time
	// (template.ParseName), not at restructure time. This is tested in template_test.go.

	t.Run("adds language instruction in MapReduce", func(t *testing.T) {
		t.Parallel()

		var capturedPrompts []string
		var mu sync.Mutex

		capturingMock := &capturingMock{
			prompts:  &capturedPrompts,
			mu:       &mu,
			response: "result",
		}

		base := restructure.NewOpenAIRestructurer(nil,
			restructure.WithChatCompleter(capturingMock),
			restructure.WithRetryDelays(time.Millisecond, time.Millisecond),
		)

		mr := restructure.NewMapReduceRestructurer(base,
			restructure.WithMapReduceMaxTokens(50),
		)

		para1 := strings.Repeat("a", 300)
		para2 := strings.Repeat("b", 300)
		transcript := para1 + "\n\n" + para2

		_, _, err := mr.Restructure(context.Background(), transcript, template.MustParseName("meeting"), lang.MustParse("pt-BR"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()

		// All prompts (map and reduce) should have Portuguese instruction
		// Note: lang.DisplayName("pt-BR") returns "Brazilian Portuguese"
		for i, prompt := range capturedPrompts {
			if !strings.Contains(prompt, "Respond in Brazilian Portuguese") {
				t.Errorf("prompt %d should contain Portuguese instruction, got: %s", i, prompt)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Shared helper mocks for MapReduce tests
// ---------------------------------------------------------------------------

// mockChatCompleter implements chatCompleter for testing.
type mockChatCompleter struct {
	mu          sync.Mutex
	calls       []chatCall
	response    openai.ChatCompletionResponse
	err         error
	errSequence []error
	callCount   int
}

type chatCall struct {
	Model    string
	Messages []openai.ChatCompletionMessage
}

func (m *mockChatCompleter) CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, chatCall{
		Model:    req.Model,
		Messages: req.Messages,
	})

	if len(m.errSequence) > 0 {
		idx := m.callCount
		m.callCount++
		if idx < len(m.errSequence) {
			if m.errSequence[idx] != nil {
				return openai.ChatCompletionResponse{}, m.errSequence[idx]
			}
		}
	} else if m.err != nil {
		return openai.ChatCompletionResponse{}, m.err
	}

	return m.response, nil
}

func (m *mockChatCompleter) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockChatCompleter) LastCall() chatCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return chatCall{}
	}
	return m.calls[len(m.calls)-1]
}

func (m *mockChatCompleter) SystemPrompt() string {
	call := m.LastCall()
	for _, msg := range call.Messages {
		if msg.Role == openai.ChatMessageRoleSystem {
			return msg.Content
		}
	}
	return ""
}

// successResponse creates a mock response with the given content.
func successResponse(content string) openai.ChatCompletionResponse {
	return openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: content}},
		},
	}
}

// interceptingMock returns predefined responses in sequence.
type interceptingMock struct {
	responses []string
	calls     *[]string
	mu        *sync.Mutex
	index     int
}

func (m *interceptingMock) CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	*m.calls = append(*m.calls, "call")

	resp := ""
	if m.index < len(m.responses) {
		resp = m.responses[m.index]
		m.index++
	}

	return openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: resp}},
		},
	}, nil
}

// cancellingMock cancels context on first call.
type cancellingMock struct {
	cancel context.CancelFunc
	called bool
}

func (m *cancellingMock) CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	if !m.called {
		m.called = true
		m.cancel()
	}
	return openai.ChatCompletionResponse{}, context.Canceled
}

// capturingMock captures system prompts.
type capturingMock struct {
	prompts  *[]string
	mu       *sync.Mutex
	response string
}

func (m *capturingMock) CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	m.mu.Lock()
	for _, msg := range req.Messages {
		if msg.Role == openai.ChatMessageRoleSystem {
			*m.prompts = append(*m.prompts, msg.Content)
		}
	}
	m.mu.Unlock()

	return openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: m.response}},
		},
	}, nil
}
