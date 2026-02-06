package restructure

import "errors"

// ErrTranscriptTooLong indicates the transcript exceeds the 100K token limit.
var ErrTranscriptTooLong = errors.New("transcript exceeds 100K token limit")

// ErrEmptyAPIKey indicates that the API key was not provided.
var ErrEmptyAPIKey = errors.New("API key is required")
