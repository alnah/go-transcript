package cli

import "errors"

// CLI-specific sentinel errors.
// These are validation/usage errors that don't belong to domain packages.

var (
	// ErrAPIKeyMissing indicates OPENAI_API_KEY environment variable is not set.
	ErrAPIKeyMissing = errors.New("OPENAI_API_KEY environment variable not set")

	// ErrInvalidDuration indicates a duration string could not be parsed.
	ErrInvalidDuration = errors.New("invalid duration format")

	// ErrUnsupportedFormat indicates an audio file has an unsupported extension.
	ErrUnsupportedFormat = errors.New("unsupported audio format")

	// ErrFileNotFound indicates the specified input file does not exist.
	ErrFileNotFound = errors.New("file not found")

	// ErrOutputExists indicates the output file already exists.
	ErrOutputExists = errors.New("output file already exists")
)
