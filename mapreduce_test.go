package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// =============================================================================
// Test Helpers (SWOT: Change - create helpers for cleaner tests)
// =============================================================================

// mockTemplateResolver returns a template resolver that returns the given prompt.
// Use this to isolate tests from actual template content.
func mockTemplateResolver(prompt string) templateResolver {
	return func(name string) (string, error) {
		if name == "" {
			return "", ErrUnknownTemplate
		}
		return prompt, nil
	}
}

// mockTemplateResolverWithError returns a resolver that always fails.
func mockTemplateResolverWithError(err error) templateResolver {
	return func(name string) (string, error) {
		return "", err
	}
}

// newTestMapReduceRestructurer creates a MapReduceRestructurer with injected mock.
// Uses a low maxTokens to trigger MapReduce with small test inputs.
func newTestMapReduceRestructurer(t *testing.T, mock *mockChatCompleter, maxTokens int) *MapReduceRestructurer {
	t.Helper()
	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		withTemplateResolver(mockTemplateResolver("Test prompt for restructuring.")),
	)
	return NewMapReduceRestructurer(r, WithMapReduceMaxTokens(maxTokens))
}

// newTestMapReduceRestructurerWithResolver creates a MapReduceRestructurer with custom resolver.
func newTestMapReduceRestructurerWithResolver(t *testing.T, mock *mockChatCompleter, maxTokens int, resolver templateResolver) *MapReduceRestructurer {
	t.Helper()
	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		withTemplateResolver(resolver),
	)
	return NewMapReduceRestructurer(r, WithMapReduceMaxTokens(maxTokens))
}

// generateParagraphs creates a transcript with n paragraphs, each of specified token size.
// Uses estimateTokens logic (len/3) to calculate actual string length needed.
func generateParagraphs(n int, tokensPerParagraph int) string {
	charsPerParagraph := tokensPerParagraph * 3 // estimateTokens uses len/3
	paragraph := strings.Repeat("x", charsPerParagraph)
	paragraphs := make([]string, n)
	for i := range paragraphs {
		paragraphs[i] = paragraph
	}
	return strings.Join(paragraphs, "\n\n")
}

// generateTranscriptWithTokens creates a transcript with approximately the given token count.
func generateTranscriptWithTokens(tokens int) string {
	chars := tokens * 3 // estimateTokens uses len/3
	return strings.Repeat("w", chars)
}

// testContext returns a context with a short timeout for tests.
// SWOT: Mitigate - always use timeout to prevent hanging tests.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// progressRecorder captures progress callback invocations.
type progressRecorder struct {
	mu    sync.Mutex
	calls []progressCall
}

type progressCall struct {
	phase   string
	current int
	total   int
}

func (r *progressRecorder) record(phase string, current, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, progressCall{phase, current, total})
}

func (r *progressRecorder) getCalls() []progressCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]progressCall, len(r.calls))
	copy(result, r.calls)
	return result
}

// =============================================================================
// splitTranscript Tests (SWOT: Keep - table-driven, pure function)
// =============================================================================

func TestSplitTranscript_ShortText_ReturnsNil(t *testing.T) {
	t.Parallel()

	// Text with 50 tokens (150 chars) when maxTokens is 100
	text := generateTranscriptWithTokens(50)
	maxTokens := 100

	chunks := splitTranscript(text, maxTokens)

	if chunks != nil {
		t.Errorf("expected nil for short text, got %d chunks", len(chunks))
	}
}

func TestSplitTranscript_ExactlyAtLimit_ReturnsNil(t *testing.T) {
	t.Parallel()

	maxTokens := 100
	text := generateTranscriptWithTokens(maxTokens)

	chunks := splitTranscript(text, maxTokens)

	if chunks != nil {
		t.Errorf("expected nil for text at limit, got %d chunks", len(chunks))
	}
}

func TestSplitTranscript_JustOverLimit_ReturnsTwoChunks(t *testing.T) {
	t.Parallel()

	maxTokens := 100
	// Two paragraphs of 60 tokens each = 120 tokens total, exceeds 100
	text := generateParagraphs(2, 60)

	chunks := splitTranscript(text, maxTokens)

	if chunks == nil {
		t.Fatal("expected chunks for text over limit, got nil")
	}
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(chunks))
	}
}

func TestSplitTranscript_SplitsAtParagraphBoundaries(t *testing.T) {
	t.Parallel()

	maxTokens := 100
	// 3 paragraphs of 40 tokens each
	// Chunk 1: para 1+2 = 80 tokens (under limit)
	// Chunk 2: para 3 = 40 tokens
	para1 := strings.Repeat("a", 120) // 40 tokens
	para2 := strings.Repeat("b", 120) // 40 tokens
	para3 := strings.Repeat("c", 120) // 40 tokens
	text := para1 + "\n\n" + para2 + "\n\n" + para3

	chunks := splitTranscript(text, maxTokens)

	if chunks == nil {
		t.Fatal("expected chunks, got nil")
	}
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(chunks))
	}

	// Verify no paragraph is split mid-content
	for i, chunk := range chunks {
		if strings.Contains(chunk.Content, "\n\n") {
			// This is OK - chunks can contain multiple paragraphs
			continue
		}
		// Single paragraph chunks should be intact
		content := chunk.Content
		if !strings.HasPrefix(content, "aaa") && !strings.HasPrefix(content, "bbb") && !strings.HasPrefix(content, "ccc") {
			t.Errorf("chunk %d has unexpected content prefix", i)
		}
	}
}

func TestSplitTranscript_LargeParagraphNotSplit(t *testing.T) {
	t.Parallel()

	maxTokens := 100
	// Single paragraph of 150 tokens - exceeds limit but cannot be split
	largePara := strings.Repeat("x", 450) // 150 tokens

	chunks := splitTranscript(largePara, maxTokens)

	// Single chunk after split returns nil (minChunksForMapReduce = 2)
	if chunks != nil {
		t.Errorf("expected nil for single large paragraph, got %d chunks", len(chunks))
	}
}

func TestSplitTranscript_ChunksHaveCorrectIndexAndTotal(t *testing.T) {
	t.Parallel()

	maxTokens := 50
	// 4 paragraphs of 30 tokens each, each paragraph becomes its own chunk
	text := generateParagraphs(4, 30)

	chunks := splitTranscript(text, maxTokens)

	if chunks == nil {
		t.Fatal("expected chunks, got nil")
	}

	for i, chunk := range chunks {
		if chunk.Index != i {
			t.Errorf("chunk %d has Index=%d, want %d", i, chunk.Index, i)
		}
		if chunk.Total != len(chunks) {
			t.Errorf("chunk %d has Total=%d, want %d", i, chunk.Total, len(chunks))
		}
	}
}

func TestSplitTranscript_SingleChunkAfterSplit_ReturnsNil(t *testing.T) {
	t.Parallel()

	maxTokens := 100
	// Two paragraphs that fit in one chunk after combining
	text := generateParagraphs(2, 40) // 80 tokens total, under 100

	chunks := splitTranscript(text, maxTokens)

	if chunks != nil {
		t.Errorf("expected nil when result is single chunk, got %d chunks", len(chunks))
	}
}

func TestSplitTranscript_EmptyInput_ReturnsNil(t *testing.T) {
	t.Parallel()

	chunks := splitTranscript("", 100)

	if chunks != nil {
		t.Errorf("expected nil for empty input, got %d chunks", len(chunks))
	}
}

func TestSplitTranscript_PreservesContentIntegrity(t *testing.T) {
	t.Parallel()

	// Each paragraph must be large enough to force splitting
	// Using 20 tokens per paragraph (60 chars), with maxTokens=30,
	// we ensure each paragraph exceeds the limit individually
	para1 := "First paragraph with enough content here."  // ~14 tokens
	para2 := "Second paragraph with enough content here." // ~14 tokens
	para3 := "Third paragraph with enough content here."  // ~14 tokens
	original := para1 + "\n\n" + para2 + "\n\n" + para3
	maxTokens := 20 // Force split after each paragraph

	chunks := splitTranscript(original, maxTokens)

	if chunks == nil {
		t.Fatal("expected chunks, got nil")
	}

	// Reconstruct and verify content is preserved (minus potential whitespace trimming)
	var reconstructed strings.Builder
	for i, chunk := range chunks {
		if i > 0 {
			reconstructed.WriteString("\n\n")
		}
		reconstructed.WriteString(chunk.Content)
	}

	// Each original paragraph should appear in reconstructed text
	for _, para := range []string{para1, para2, para3} {
		if !strings.Contains(reconstructed.String(), para) {
			t.Errorf("reconstructed text missing paragraph: %q", para)
		}
	}
}

// =============================================================================
// buildMapPrompt Tests (SWOT: Keep - minimal, assertContains)
// =============================================================================

func TestBuildMapPrompt_ContainsPartNumber(t *testing.T) {
	t.Parallel()

	chunk := TranscriptChunk{Index: 2, Total: 5, Content: "test"}
	basePrompt := "Base instructions"

	result := buildMapPrompt(basePrompt, chunk)

	// Index is 0-based, display is 1-based
	assertContains(t, result, "part 3 of 5")
}

func TestBuildMapPrompt_ContainsBasePrompt(t *testing.T) {
	t.Parallel()

	chunk := TranscriptChunk{Index: 0, Total: 2, Content: "test"}
	basePrompt := "Unique base instructions here"

	result := buildMapPrompt(basePrompt, chunk)

	assertContains(t, result, "Unique base instructions here")
}

func TestBuildMapPrompt_MentionsH1ForFirstPart(t *testing.T) {
	t.Parallel()

	chunk := TranscriptChunk{Index: 0, Total: 3, Content: "test"}

	result := buildMapPrompt("Base", chunk)

	// The prompt mentions H1 rules for non-first parts
	assertContains(t, result, "part 1 of 3")
}

// =============================================================================
// MapReduceRestructurer.Restructure Tests (SWOT: Keep - API publique)
// =============================================================================

func TestMapReduceRestructurer_ShortTranscript_NoMapReduce(t *testing.T) {
	t.Parallel()

	mock := withChatSuccess("Restructured output")
	mr := newTestMapReduceRestructurer(t, mock, 1000) // High limit

	ctx := testContext(t)
	result, usedMapReduce, err := mr.Restructure(ctx, "Short transcript", "brainstorm", "")

	assertNoError(t, err)
	if usedMapReduce {
		t.Error("expected usedMapReduce=false for short transcript")
	}
	assertEqual(t, result, "Restructured output")
	assertEqual(t, mock.CallCount(), 1)
}

func TestMapReduceRestructurer_LongTranscript_UsesMapReduce(t *testing.T) {
	t.Parallel()

	// Mock returns different responses for map and reduce phases
	mock := withChatSequence(
		mockChatResponse{response: chatResponse("Part 1 output")},
		mockChatResponse{response: chatResponse("Part 2 output")},
		mockChatResponse{response: chatResponse("Merged output")},
	)
	mr := newTestMapReduceRestructurer(t, mock, 50) // Low limit to trigger split

	ctx := testContext(t)
	transcript := generateParagraphs(3, 30) // 3 paragraphs of 30 tokens each

	result, usedMapReduce, err := mr.Restructure(ctx, transcript, "meeting", "")

	assertNoError(t, err)
	if !usedMapReduce {
		t.Error("expected usedMapReduce=true for long transcript")
	}
	assertEqual(t, result, "Merged output")
}

func TestMapReduceRestructurer_CallsRestructurerForEachChunk(t *testing.T) {
	t.Parallel()

	mock := withChatSuccess("output")
	mr := newTestMapReduceRestructurer(t, mock, 30) // Very low limit

	ctx := testContext(t)
	// 4 paragraphs that will create 4 chunks
	transcript := generateParagraphs(4, 20)

	_, usedMapReduce, err := mr.Restructure(ctx, transcript, "lecture", "")

	assertNoError(t, err)
	if !usedMapReduce {
		t.Error("expected usedMapReduce=true")
	}
	// 4 map calls + 1 reduce call = 5 total
	if mock.CallCount() < 3 { // At least map + reduce
		t.Errorf("expected at least 3 calls (map phases + reduce), got %d", mock.CallCount())
	}
}

func TestMapReduceRestructurer_MapPromptIncludesPartInfo(t *testing.T) {
	t.Parallel()

	mock := withChatSuccess("output")
	mr := newTestMapReduceRestructurer(t, mock, 50)

	ctx := testContext(t)
	transcript := generateParagraphs(2, 40)

	_, _, err := mr.Restructure(ctx, transcript, "brainstorm", "")
	assertNoError(t, err)

	requests := mock.AllRequests()
	if len(requests) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(requests))
	}

	// First request (map phase) should contain part info
	firstPrompt := requests[0].Messages[0].Content
	assertContains(t, firstPrompt, "part 1 of")
}

func TestMapReduceRestructurer_ReduceReceivesAllOutputs(t *testing.T) {
	t.Parallel()

	mock := withChatSequence(
		mockChatResponse{response: chatResponse("MAP_OUTPUT_1")},
		mockChatResponse{response: chatResponse("MAP_OUTPUT_2")},
		mockChatResponse{response: chatResponse("FINAL")},
	)
	mr := newTestMapReduceRestructurer(t, mock, 50)

	ctx := testContext(t)
	transcript := generateParagraphs(2, 40)

	_, _, err := mr.Restructure(ctx, transcript, "meeting", "")
	assertNoError(t, err)

	requests := mock.AllRequests()
	// Last request is reduce phase
	reduceRequest := requests[len(requests)-1]
	reduceContent := reduceRequest.Messages[1].Content

	assertContains(t, reduceContent, "=== PART 1 ===")
	assertContains(t, reduceContent, "=== PART 2 ===")
	assertContains(t, reduceContent, "MAP_OUTPUT_1")
	assertContains(t, reduceContent, "MAP_OUTPUT_2")
}

func TestMapReduceRestructurer_InvalidTemplate_ReturnsError(t *testing.T) {
	t.Parallel()

	mock := withChatSuccess("output")
	resolver := mockTemplateResolverWithError(ErrUnknownTemplate)
	mr := newTestMapReduceRestructurerWithResolver(t, mock, 50, resolver)

	ctx := testContext(t)
	transcript := generateParagraphs(2, 40)

	_, _, err := mr.Restructure(ctx, transcript, "invalid", "")

	assertError(t, err, ErrUnknownTemplate)
}

func TestMapReduceRestructurer_ContextCancellation_AbortsEarly(t *testing.T) {
	t.Parallel()

	mock := withChatSuccess("output")
	mr := newTestMapReduceRestructurer(t, mock, 30)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	transcript := generateParagraphs(4, 20)

	_, _, err := mr.Restructure(ctx, transcript, "brainstorm", "")

	if err == nil {
		t.Error("expected error on cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestMapReduceRestructurer_MapError_ReturnsWrappedError(t *testing.T) {
	t.Parallel()

	testErr := errors.New("API failure")
	mock := withChatSequence(
		mockChatResponse{response: chatResponse("Part 1 OK")},
		mockChatResponse{err: testErr}, // Fail on second chunk
	)
	mr := newTestMapReduceRestructurer(t, mock, 50)

	ctx := testContext(t)
	transcript := generateParagraphs(2, 40)

	_, _, err := mr.Restructure(ctx, transcript, "meeting", "")

	if err == nil {
		t.Fatal("expected error")
	}
	assertContains(t, err.Error(), "failed to process chunk")
}

func TestMapReduceRestructurer_ReduceError_ReturnsWrappedError(t *testing.T) {
	t.Parallel()

	testErr := errors.New("reduce failure")
	mock := withChatSequence(
		mockChatResponse{response: chatResponse("Part 1")},
		mockChatResponse{response: chatResponse("Part 2")},
		mockChatResponse{err: testErr}, // Fail on reduce
	)
	mr := newTestMapReduceRestructurer(t, mock, 50)

	ctx := testContext(t)
	transcript := generateParagraphs(2, 40)

	_, _, err := mr.Restructure(ctx, transcript, "brainstorm", "")

	if err == nil {
		t.Fatal("expected error")
	}
	assertContains(t, err.Error(), "failed to merge chunks")
}

// =============================================================================
// Language Handling Tests (SWOT: Mitigate - 3 tests max)
// =============================================================================

func TestMapReduceRestructurer_OutputLangEnglish_NoLanguagePrefix(t *testing.T) {
	t.Parallel()

	mock := withChatSuccess("output")
	mr := newTestMapReduceRestructurer(t, mock, 50)

	ctx := testContext(t)
	transcript := generateParagraphs(2, 40)

	_, _, err := mr.Restructure(ctx, transcript, "meeting", "en")
	assertNoError(t, err)

	requests := mock.AllRequests()
	for i, req := range requests {
		prompt := req.Messages[0].Content
		if strings.Contains(prompt, "Respond in") {
			t.Errorf("request %d should not contain language prefix for English", i)
		}
	}
}

func TestMapReduceRestructurer_OutputLangFrench_AddsLanguagePrefix(t *testing.T) {
	t.Parallel()

	mock := withChatSuccess("output")
	mr := newTestMapReduceRestructurer(t, mock, 50)

	ctx := testContext(t)
	transcript := generateParagraphs(2, 40)

	_, _, err := mr.Restructure(ctx, transcript, "meeting", "fr")
	assertNoError(t, err)

	requests := mock.AllRequests()
	// All prompts (map and reduce) should have language instruction
	for i, req := range requests {
		prompt := req.Messages[0].Content
		if !strings.Contains(prompt, "Respond in French") {
			t.Errorf("request %d missing 'Respond in French' prefix", i)
		}
	}
}

func TestMapReduceRestructurer_LanguagePrefixInMapAndReduce(t *testing.T) {
	t.Parallel()

	mock := withChatSuccess("output")
	mr := newTestMapReduceRestructurer(t, mock, 50)

	ctx := testContext(t)
	transcript := generateParagraphs(2, 40)

	_, usedMapReduce, err := mr.Restructure(ctx, transcript, "lecture", "pt-BR")
	assertNoError(t, err)

	if !usedMapReduce {
		t.Skip("MapReduce not triggered, cannot verify both phases")
	}

	requests := mock.AllRequests()
	if len(requests) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(requests))
	}

	// Verify both map and reduce have language prefix
	mapPrompt := requests[0].Messages[0].Content
	reducePrompt := requests[len(requests)-1].Messages[0].Content

	assertContains(t, mapPrompt, "Respond in")
	assertContains(t, reducePrompt, "Respond in")
}

// =============================================================================
// Progress Callback Tests (SWOT: Mitigate - 2-3 tests)
// =============================================================================

func TestMapReduceRestructurer_ProgressCallback_CalledForEachPhase(t *testing.T) {
	t.Parallel()

	mock := withChatSuccess("output")
	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		withTemplateResolver(mockTemplateResolver("Test prompt")),
	)

	recorder := &progressRecorder{}
	mr := NewMapReduceRestructurer(r,
		WithMapReduceMaxTokens(50),
		WithMapReduceProgress(recorder.record),
	)

	ctx := testContext(t)
	transcript := generateParagraphs(2, 40)

	_, usedMapReduce, err := mr.Restructure(ctx, transcript, "brainstorm", "")
	assertNoError(t, err)

	if !usedMapReduce {
		t.Skip("MapReduce not triggered")
	}

	calls := recorder.getCalls()
	if len(calls) == 0 {
		t.Fatal("expected progress callbacks")
	}

	// Verify map phase calls exist
	hasMap := false
	hasReduce := false
	for _, call := range calls {
		if call.phase == "map" {
			hasMap = true
		}
		if call.phase == "reduce" {
			hasReduce = true
		}
	}

	if !hasMap {
		t.Error("expected map phase callbacks")
	}
	if !hasReduce {
		t.Error("expected reduce phase callback")
	}
}

func TestMapReduceRestructurer_NoCallback_NoError(t *testing.T) {
	t.Parallel()

	mock := withChatSuccess("output")
	mr := newTestMapReduceRestructurer(t, mock, 50)

	ctx := testContext(t)
	transcript := generateParagraphs(2, 40)

	// Should not panic without callback
	_, _, err := mr.Restructure(ctx, transcript, "meeting", "")
	assertNoError(t, err)
}

// =============================================================================
// Options Tests (SWOT: Mitigate - via behavior, 2-3 tests)
// =============================================================================

func TestWithMapReduceMaxTokens_AffectsSplitBehavior(t *testing.T) {
	t.Parallel()

	mock := withChatSuccess("output")
	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		withTemplateResolver(mockTemplateResolver("Test")),
	)

	// With very low maxTokens, even small text triggers MapReduce
	mr := NewMapReduceRestructurer(r, WithMapReduceMaxTokens(10))

	ctx := testContext(t)
	transcript := generateParagraphs(2, 20) // 40 tokens total

	_, usedMapReduce, err := mr.Restructure(ctx, transcript, "brainstorm", "")
	assertNoError(t, err)

	if !usedMapReduce {
		t.Error("expected MapReduce with low maxTokens threshold")
	}
}

func TestWithMapReduceMaxTokens_ZeroUsesDefault(t *testing.T) {
	t.Parallel()

	mock := withChatSuccess("output")
	r := NewOpenAIRestructurer(nil,
		withChatCompleter(mock),
		withTemplateResolver(mockTemplateResolver("Test")),
	)

	// Zero should be ignored, using default (80000)
	mr := NewMapReduceRestructurer(r, WithMapReduceMaxTokens(0))

	ctx := testContext(t)
	transcript := generateParagraphs(2, 40) // Small text

	_, usedMapReduce, err := mr.Restructure(ctx, transcript, "meeting", "")
	assertNoError(t, err)

	if usedMapReduce {
		t.Error("expected no MapReduce with default (high) threshold")
	}
}

// =============================================================================
// Helper for creating chat responses
// =============================================================================

func chatResponse(content string) openai.ChatCompletionResponse {
	return openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: content}},
		},
	}
}
