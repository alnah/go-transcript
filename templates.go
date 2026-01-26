package main

import "fmt"

// Template name constants.
// Use these instead of string literals for compile-time safety.
const (
	TemplateBrainstorm = "brainstorm"
	TemplateMeeting    = "meeting"
	TemplateLecture    = "lecture"
)

// templateOrder defines the canonical order for TemplateNames().
// This order matches the spec and is used for CLI help and error messages.
var templateOrder = []string{
	TemplateBrainstorm,
	TemplateMeeting,
	TemplateLecture,
}

// templates maps template names to their prompt strings.
// Prompts are versioned with the binary; update requires rebuild.
var templates = map[string]string{
	TemplateBrainstorm: brainstormPrompt,
	TemplateMeeting:    meetingPrompt,
	TemplateLecture:    lecturePrompt,
}

// GetTemplate returns the prompt for the given template name.
// Returns ErrUnknownTemplate if the name is not recognized.
func GetTemplate(name string) (string, error) {
	prompt, ok := templates[name]
	if !ok {
		return "", fmt.Errorf("unknown template %q: %w", name, ErrUnknownTemplate)
	}
	return prompt, nil
}

// TemplateNames returns the list of available template names.
// The order is stable and matches the spec (brainstorm, meeting, lecture).
func TemplateNames() []string {
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

const lecturePrompt = `You add structure to a lecture transcript while preserving it verbatim.

Output format: markdown with # for H1, ## for H2, ### for H3.

Rules:
- Keep the EXACT text flow - do not reorder, regroup, or summarize
- Insert # title at the beginning (infer subject from content)
- Insert ## headers when the speaker changes topic
- Insert ### headers for sub-topics within a section
- **Bold** key terms and definitions when first introduced
- Correct obvious transcription errors (spelling, grammar)
- Remove filler words (um, uh, like, you know, basically)
- Keep the text as continuous prose, NOT bullet points
- Every sentence from the transcript must appear in the output, in order
- Do not alter meaning, do not invent anything
- No table of contents`
