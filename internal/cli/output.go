package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// warnNonMarkdownExtension writes a warning to w if path has an extension
// that is not .md. This alerts users that the output will be Markdown
// regardless of the file extension they specified.
func warnNonMarkdownExtension(w io.Writer, path string) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != "" && ext != ".md" {
		_, _ = fmt.Fprintf(w, "Warning: output is Markdown regardless of %s extension\n", ext)
	}
}

// defaultProgressCallback returns a progress callback that writes status
// messages to w. Used by restructuring operations in live and transcribe commands.
func defaultProgressCallback(w io.Writer) func(phase string, current, total int) {
	return func(phase string, current, total int) {
		if phase == "map" {
			_, _ = fmt.Fprintf(w, "  Processing part %d/%d...\n", current, total)
		} else {
			_, _ = fmt.Fprintln(w, "  Merging parts...")
		}
	}
}

// writeFileAtomic writes content to path atomically.
// It fails if the file already exists (O_EXCL), preventing accidental overwrites.
// On write failure, the partial file is removed.
func writeFileAtomic(path, content string) error {
	// #nosec G302 G304 -- user-specified output file with standard permissions
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("output file already exists: %s: %w", path, ErrOutputExists)
		}
		return fmt.Errorf("cannot create output file: %w", err)
	}

	writeErr := func() error {
		defer func() { _ = f.Close() }()
		if _, err := f.WriteString(content); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
		return nil
	}()

	if writeErr != nil {
		_ = os.Remove(path)
		return writeErr
	}

	return nil
}
