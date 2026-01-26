package restructure

import "errors"

// ErrTranscriptTooLong indicates the transcript exceeds the 100K token limit.
var ErrTranscriptTooLong = errors.New("transcript exceeds 100K token limit")
