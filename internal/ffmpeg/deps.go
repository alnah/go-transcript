package ffmpeg

import (
	"io"
	"net/http"
	"os"
	"os/exec"
)

// ---------------------------------------------------------------------------
// Interfaces - local to this package, following Go idiom
// ---------------------------------------------------------------------------

// fileReader abstracts read operations on the filesystem.
type fileReader interface {
	Stat(name string) (os.FileInfo, error)
	ReadFile(name string) ([]byte, error)
	Open(name string) (io.ReadCloser, error)
}

// fileWriter abstracts write operations on the filesystem.
type fileWriter interface {
	WriteFile(name string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	Remove(name string) error
	Rename(oldpath, newpath string) error
	Chmod(name string, mode os.FileMode) error
	CreateTemp(dir, pattern string) (*os.File, error)
}

// httpDoer abstracts HTTP client operations.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// envProvider abstracts environment and path lookup operations.
type envProvider interface {
	Getenv(key string) string
	UserHomeDir() (string, error)
	LookPath(file string) (string, error)
}

// ---------------------------------------------------------------------------
// Default implementations - delegate to standard library
// ---------------------------------------------------------------------------

// Compile-time interface verification.
var (
	_ fileReader  = osFileReader{}
	_ fileWriter  = osFileWriter{}
	_ envProvider = osEnvProvider{}
)

// osFileReader implements fileReader using the os package.
type osFileReader struct{}

func (osFileReader) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (osFileReader) ReadFile(name string) ([]byte, error) {
	// #nosec G304 -- paths come from internal resolution, not user input
	return os.ReadFile(name)
}

func (osFileReader) Open(name string) (io.ReadCloser, error) {
	// #nosec G304 -- paths come from internal resolution, not user input
	return os.Open(name)
}

// osFileWriter implements fileWriter using the os package.
type osFileWriter struct{}

func (osFileWriter) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (osFileWriter) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osFileWriter) Remove(name string) error {
	return os.Remove(name)
}

func (osFileWriter) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (osFileWriter) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

func (osFileWriter) CreateTemp(dir, pattern string) (*os.File, error) {
	return os.CreateTemp(dir, pattern)
}

// osEnvProvider implements envProvider using os and exec packages.
type osEnvProvider struct{}

func (osEnvProvider) Getenv(key string) string {
	return os.Getenv(key)
}

func (osEnvProvider) UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

func (osEnvProvider) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}
