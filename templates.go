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

// Prompt templates in French.
// These instruct the LLM how to restructure raw transcripts.

const brainstormPrompt = `Tu restructures un transcript de session de brainstorming en markdown.

Regles :
- Titre H1 : sujet principal identifie
- Sections H2 : un theme par section (regroupe les idees connexes)
- Bullet points : une idee = un point
- Section finale "Idees cles" : 3-5 insights les plus importants
- Section finale "Actions" : uniquement si des actions concretes sont mentionnees
- Corrige les erreurs de transcription evidentes
- Supprime les filler words (euh, hum, en fait, du coup)
- Ne modifie pas le sens, n'invente rien
- Pas de table des matieres`

const meetingPrompt = `Tu restructures un transcript de reunion en compte-rendu markdown.

Regles :
- Titre H1 : objet de la reunion
- Section "Participants" : uniquement si des noms sont mentionnes
- Section "Points abordes" : H2 par sujet discute
- Section "Decisions" : liste des decisions prises (si aucune, omettre la section)
- Section "Actions" : format "- [ ] Action (Responsable, Deadline)" si mentionnes
- Corrige les erreurs de transcription evidentes
- Supprime les filler words
- Ne modifie pas le sens, n'invente rien
- Pas de table des matieres`

const lecturePrompt = `Tu restructures un transcript de cours ou conference en notes markdown.

Regles :
- Titre H1 : sujet du cours/conference
- Sections H2 : concepts principaux
- Sous-sections H3 : sous-concepts si necessaire
- Bullet points pour les details
- **Gras** pour les termes et definitions importants
- Citations verbatim en blockquote uniquement si particulierement memorables
- Corrige les erreurs de transcription evidentes
- Supprime les filler words
- Ne modifie pas le sens, n'invente rien
- Pas de table des matieres`
