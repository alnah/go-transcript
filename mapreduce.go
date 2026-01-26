package main

import (
	"context"
	"fmt"
	"strings"
)

// MapReduce configuration for long transcript handling.
const (
	// maxChunkTokens is the target size for each chunk.
	// We use 80K to leave room for the prompt and response within the 128K limit.
	maxChunkTokens = 80000

	// minChunksForMapReduce is the minimum number of chunks to trigger MapReduce.
	// If transcript fits in 1 chunk after splitting, we skip MapReduce overhead.
	minChunksForMapReduce = 2
)

// TranscriptChunk represents a portion of a transcript for MapReduce processing.
type TranscriptChunk struct {
	Index   int    // 0-based index
	Content string // The chunk content
	Total   int    // Total number of chunks
}

// splitTranscript divides a transcript into chunks at paragraph boundaries.
// Each chunk targets maxTokens size but respects paragraph boundaries.
// Returns nil if transcript fits in a single chunk.
func splitTranscript(transcript string, maxTokens int) []TranscriptChunk {
	totalTokens := estimateTokens(transcript)
	if totalTokens <= maxTokens {
		return nil // No splitting needed
	}

	paragraphs := strings.Split(transcript, "\n\n")
	var chunks []TranscriptChunk
	var currentChunk strings.Builder
	currentTokens := 0

	for _, para := range paragraphs {
		paraTokens := estimateTokens(para)

		// If single paragraph exceeds limit, we must include it anyway
		// (splitting mid-paragraph would break coherence)
		if currentTokens+paraTokens > maxTokens && currentChunk.Len() > 0 {
			// Save current chunk and start new one
			chunks = append(chunks, TranscriptChunk{
				Index:   len(chunks),
				Content: strings.TrimSpace(currentChunk.String()),
			})
			currentChunk.Reset()
			currentTokens = 0
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
		}
		currentChunk.WriteString(para)
		currentTokens += paraTokens
	}

	// Don't forget the last chunk
	if currentChunk.Len() > 0 {
		chunks = append(chunks, TranscriptChunk{
			Index:   len(chunks),
			Content: strings.TrimSpace(currentChunk.String()),
		})
	}

	// Set total count on all chunks
	total := len(chunks)
	for i := range chunks {
		chunks[i].Total = total
	}

	// If we ended up with only 1 chunk, no MapReduce needed
	if len(chunks) < minChunksForMapReduce {
		return nil
	}

	return chunks
}

// Prompts for MapReduce processing.
const (
	// mapChunkPromptPrefix is prepended to the template for chunk processing.
	// It provides context about the chunking.
	mapChunkPromptPrefix = `IMPORTANT: This transcript has been split into multiple parts due to length.
You are processing part %d of %d.

%s

Process this part following the rules above. The final output will be merged with other parts.
If this is not part 1, continue the structure from where the previous part left off.
Do not add a main title (H1) unless this is part 1.`

	// reducePrompt is used to merge chunk outputs into a coherent whole.
	reducePrompt = `You receive multiple parts of a restructured markdown document.
Merge them into a single coherent document.

Rules:
- Keep only one H1 title (from the first part)
- Merge H2 sections that cover the same topic
- Eliminate exact duplicates only (same sentence repeated)
- Preserve ALL unique content, even if topics are similar
- Do not summarize or condense - every detail must be kept
- Maintain a logical and coherent structure
- Keep "Key Ideas", "Decisions", "Actions" sections at the end (merged if present in multiple parts)
- Do not alter meaning, do not invent anything`
)

// buildMapPrompt creates the prompt for processing a single chunk.
func buildMapPrompt(basePrompt string, chunk TranscriptChunk) string {
	return fmt.Sprintf(mapChunkPromptPrefix, chunk.Index+1, chunk.Total, basePrompt)
}

// MapReduceRestructurer handles long transcripts by splitting, processing, and merging.
type MapReduceRestructurer struct {
	restructurer *OpenAIRestructurer
	maxTokens    int
	onProgress   func(phase string, current, total int) // Optional progress callback
}

// MapReduceOption configures a MapReduceRestructurer.
type MapReduceOption func(*MapReduceRestructurer)

// WithMapReduceMaxTokens sets the max tokens per chunk.
func WithMapReduceMaxTokens(max int) MapReduceOption {
	return func(mr *MapReduceRestructurer) {
		if max > 0 {
			mr.maxTokens = max
		}
	}
}

// WithMapReduceProgress sets a progress callback.
func WithMapReduceProgress(fn func(phase string, current, total int)) MapReduceOption {
	return func(mr *MapReduceRestructurer) {
		mr.onProgress = fn
	}
}

// NewMapReduceRestructurer creates a MapReduceRestructurer wrapping an existing restructurer.
func NewMapReduceRestructurer(r *OpenAIRestructurer, opts ...MapReduceOption) *MapReduceRestructurer {
	mr := &MapReduceRestructurer{
		restructurer: r,
		maxTokens:    maxChunkTokens,
	}
	for _, opt := range opts {
		opt(mr)
	}
	return mr
}

// Restructure processes a transcript, using MapReduce if it exceeds the token limit.
// Returns the restructured output, whether MapReduce was used, and any error.
func (mr *MapReduceRestructurer) Restructure(ctx context.Context, transcript, templateName, outputLang string) (string, bool, error) {
	// Check if MapReduce is needed
	chunks := splitTranscript(transcript, mr.maxTokens)
	if chunks == nil {
		// Fits in one chunk, use standard restructuring
		result, err := mr.restructurer.Restructure(ctx, transcript, templateName, outputLang)
		return result, false, err
	}

	// MapReduce needed
	return mr.mapReduce(ctx, chunks, templateName, outputLang)
}

// mapReduce executes the map and reduce phases.
func (mr *MapReduceRestructurer) mapReduce(ctx context.Context, chunks []TranscriptChunk, templateName, outputLang string) (string, bool, error) {
	// Get base template
	basePrompt, err := mr.restructurer.resolveTemplate(templateName)
	if err != nil {
		return "", true, err
	}

	// Add language instruction if needed (skip for English, template's native language)
	if outputLang != "" && !IsEnglish(outputLang) {
		langName := LanguageDisplayName(outputLang)
		basePrompt = fmt.Sprintf("Respond in %s.\n\n%s", langName, basePrompt)
	}

	// Map phase: process each chunk
	chunkOutputs := make([]string, len(chunks))
	for i, chunk := range chunks {
		if ctx.Err() != nil {
			return "", true, ctx.Err()
		}

		if mr.onProgress != nil {
			mr.onProgress("map", i+1, len(chunks))
		}

		mapPrompt := buildMapPrompt(basePrompt, chunk)
		output, err := mr.restructurer.restructureWithCustomPrompt(ctx, chunk.Content, mapPrompt)
		if err != nil {
			return "", true, fmt.Errorf("failed to process chunk %d/%d: %w", i+1, len(chunks), err)
		}
		chunkOutputs[i] = output
	}

	// Reduce phase: merge all outputs
	if mr.onProgress != nil {
		mr.onProgress("reduce", 1, 1)
	}

	merged, err := mr.reduce(ctx, chunkOutputs, outputLang)
	if err != nil {
		return "", true, fmt.Errorf("failed to merge chunks: %w", err)
	}

	return merged, true, nil
}

// reduce merges multiple chunk outputs into a coherent document.
func (mr *MapReduceRestructurer) reduce(ctx context.Context, outputs []string, outputLang string) (string, error) {
	// Build the input for the reduce phase
	var input strings.Builder
	for i, output := range outputs {
		if i > 0 {
			input.WriteString("\n\n---\n\n")
		}
		fmt.Fprintf(&input, "=== PART %d ===\n\n%s", i+1, output)
	}

	// Build reduce prompt with language instruction (skip for English, template's native language)
	prompt := reducePrompt
	if outputLang != "" && !IsEnglish(outputLang) {
		langName := LanguageDisplayName(outputLang)
		prompt = fmt.Sprintf("Respond in %s.\n\n%s", langName, prompt)
	}

	return mr.restructurer.restructureWithCustomPrompt(ctx, input.String(), prompt)
}
