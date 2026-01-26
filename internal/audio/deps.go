package audio

import (
	"context"
	"os"
	"os/exec"
)

// commandRunner executes external commands and returns their combined output.
type commandRunner interface {
	CombinedOutput(ctx context.Context, name string, args []string) ([]byte, error)
}

// tempDirCreator creates temporary directories.
type tempDirCreator interface {
	MkdirTemp(dir, pattern string) (string, error)
}

// fileStatter retrieves file information.
type fileStatter interface {
	Stat(name string) (os.FileInfo, error)
}

// fileRemover removes files and directories.
type fileRemover interface {
	Remove(name string) error
	RemoveAll(path string) error
}

// --- Default implementations using real OS functions ---

// osCommandRunner implements commandRunner using exec.CommandContext.
type osCommandRunner struct{}

func (osCommandRunner) CombinedOutput(ctx context.Context, name string, args []string) ([]byte, error) {
	// #nosec G204 -- name and args are controlled by the chunker, not user input
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// osTempDirCreator implements tempDirCreator using os.MkdirTemp.
type osTempDirCreator struct{}

func (osTempDirCreator) MkdirTemp(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}

// osFileStatter implements fileStatter using os.Stat.
type osFileStatter struct{}

func (osFileStatter) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

// osFileRemover implements fileRemover using os.Remove and os.RemoveAll.
type osFileRemover struct{}

func (osFileRemover) Remove(name string) error {
	return os.Remove(name)
}

func (osFileRemover) RemoveAll(path string) error {
	return os.RemoveAll(path)
}
