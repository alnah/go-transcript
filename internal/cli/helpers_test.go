package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alnah/go-transcript/internal/config"
)

// ---------------------------------------------------------------------------
// syncBuffer - thread-safe bytes.Buffer for concurrent test output
// ---------------------------------------------------------------------------

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

// Compile-time check that syncBuffer implements io.Writer.
var _ io.Writer = (*syncBuffer)(nil)

// ---------------------------------------------------------------------------
// testMocks - convenience struct for grouping all mocks
// ---------------------------------------------------------------------------

type testMocks struct {
	ffmpegResolver *mockFFmpegResolver
	configLoader   *mockConfigLoader
	transcriber    *mockTranscriberFactory
	restructurer   *mockRestructurerFactory
	chunker        *mockChunkerFactory
	recorder       *mockRecorderFactory
}

func newTestMocks() *testMocks {
	return &testMocks{
		ffmpegResolver: &mockFFmpegResolver{},
		configLoader:   &mockConfigLoader{},
		transcriber:    &mockTranscriberFactory{},
		restructurer:   &mockRestructurerFactory{},
		chunker:        &mockChunkerFactory{},
		recorder:       &mockRecorderFactory{},
	}
}

// ---------------------------------------------------------------------------
// testEnv - creates a fully mocked Env for testing
// ---------------------------------------------------------------------------

// testEnvOptions configures a test environment.
type testEnvOptions struct {
	stderr io.Writer
	getenv func(string) string
	now    func() time.Time
	mocks  *testMocks
}

// testEnvOption configures testEnv.
type testEnvOption func(*testEnvOptions)

// testEnv creates a test Env with all dependencies mocked.
// Returns the Env and the mocks for assertions.
func testEnv(opts ...testEnvOption) (*Env, *testMocks) {
	options := &testEnvOptions{
		stderr: &syncBuffer{},
		getenv: defaultTestEnv,
		now: func() time.Time {
			return time.Date(2026, 1, 26, 14, 30, 52, 0, time.UTC)
		},
		mocks: newTestMocks(),
	}

	for _, opt := range opts {
		opt(options)
	}

	env := &Env{
		Stderr:              options.stderr,
		Getenv:              options.getenv,
		Now:                 options.now,
		FFmpegResolver:      options.mocks.ffmpegResolver,
		ConfigLoader:        options.mocks.configLoader,
		TranscriberFactory:  options.mocks.transcriber,
		RestructurerFactory: options.mocks.restructurer,
		ChunkerFactory:      options.mocks.chunker,
		RecorderFactory:     options.mocks.recorder,
	}

	return env, options.mocks
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// fixedTime returns a function that always returns the given time.
func fixedTime(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// staticEnv returns a getenv function that returns values from the given map.
func staticEnv(env map[string]string) func(string) string {
	return func(key string) string {
		return env[key]
	}
}

// defaultTestEnv returns API keys for both OpenAI and DeepSeek.
func defaultTestEnv(key string) string {
	switch key {
	case EnvOpenAIAPIKey:
		return "test-openai-key"
	case EnvDeepSeekAPIKey:
		return "test-deepseek-key"
	default:
		return ""
	}
}

// createTestAudioFile creates a temporary audio file for testing.
// Returns the file path. The file is automatically cleaned up after the test.
func createTestAudioFile(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)

	// Write minimal content to make the file non-empty
	if err := os.WriteFile(path, []byte("fake audio content"), 0644); err != nil {
		t.Fatalf("failed to create test audio file: %v", err)
	}
	return path
}

// configWithOutputDir returns a ConfigLoader that returns a config with the given output directory.
func configWithOutputDir(outputDir string) *mockConfigLoader {
	return &mockConfigLoader{
		LoadFunc: func() (config.Config, error) {
			return config.Config{OutputDir: outputDir}, nil
		},
	}
}
