package template

import (
	"errors"
	"fmt"
)

// ErrUnknown indicates an invalid template name was specified.
var ErrUnknown = errors.New("unknown template")

// Template name constants.
// Use these instead of string literals for compile-time safety.
const (
	Brainstorm = "brainstorm"
	Meeting    = "meeting"
	Lecture    = "lecture"
	Notes      = "notes"
)

// templateOrder defines the canonical order for Names().
// This order matches the spec and is used for CLI help and error messages.
var templateOrder = []string{
	Brainstorm,
	Meeting,
	Lecture,
	Notes,
}

// templates maps template names to their prompt strings.
// Prompts are versioned with the binary; update requires rebuild.
var templates = map[string]string{
	Brainstorm: brainstormPrompt,
	Meeting:    meetingPrompt,
	Lecture:    lecturePrompt,
	Notes:      notesPrompt,
}

// Get returns the prompt for the given template name.
// Returns ErrUnknown if the name is not recognized.
func Get(name string) (string, error) {
	prompt, ok := templates[name]
	if !ok {
		return "", fmt.Errorf("unknown template %q: %w", name, ErrUnknown)
	}
	return prompt, nil
}

// Names returns the list of available template names.
// The order is stable and matches the spec (brainstorm, meeting, lecture).
func Names() []string {
	result := make([]string, len(templateOrder))
	copy(result, templateOrder)
	return result
}

// Prompt templates in English.
// These instruct the LLM how to restructure raw transcripts.
// For non-English output, a "Respond in {language}" instruction is prepended.

const brainstormPrompt = `You restructure a brainstorming session transcript into markdown.

Rules:
- H1 title: main topic identified
- H2 sections: one theme per section (group related ideas)
- Bullet points: one idea = one point
- Final section "Key Ideas": 3-5 most important insights
- Final section "Actions": only if concrete actions are mentioned
- Correct obvious transcription errors
- Remove filler words (um, uh, like, you know, basically)
- Do not summarize - include ALL ideas mentioned
- Do not alter meaning, do not invent anything
- No table of contents`

const meetingPrompt = `You restructure a meeting transcript into markdown meeting notes.

Rules:
- H1 title: meeting subject
- "Participants" section: only if names are mentioned
- "Topics Discussed" section: H2 per topic discussed
- "Decisions" section: list of decisions made (if none, omit section)
- "Actions" section: format "- [ ] Action (Owner, Deadline)" if mentioned
- Correct obvious transcription errors
- Remove filler words
- Do not summarize - include ALL points discussed
- Do not alter meaning, do not invent anything
- No table of contents`

const lecturePrompt = `You restructure a lecture transcript into clean, readable prose while preserving all informational content.

Output format: markdown with # for H1, ## for H2, ### for H3.

Rules:
- Preserve ALL informational content - every distinct concept must appear
- Write as continuous prose, flowing and readable
- Insert # title at the beginning (infer subject from content)
- Insert ## headers when the speaker changes topic
- Insert ### headers for sub-topics within a section
- **Bold** key terms and definitions when first introduced
- Consolidate repetitions: when the same idea is stated multiple times, keep ONE clear formulation
- Remove verbal padding: filler words, rhetorical questions that add no information, hedging phrases
- Correct transcription errors (spelling, grammar)
- Maintain logical order of concepts as presented
- Do not invent content or alter meaning
- No table of contents`

const notesPrompt = `You restructure a lecture transcript into organized bullet points while preserving all informational content.

Output format: markdown with ## for themes, bullet points for content.

Rules:
- Preserve ALL informational content - every distinct concept must appear
- Group related concepts under ## thematic headers
- One bullet point = one distinct idea or fact
- Use sub-bullets for details, examples, or qualifications
- **Bold** key terms and definitions
- Consolidate repetitions: merge redundant statements into single clear bullets
- Remove verbal padding: filler words, rhetorical questions, hedging phrases
- Correct transcription errors (spelling, grammar)
- Reorder for logical flow within each theme (not strict transcript order)
- Do not invent content or alter meaning
- No table of contents`
