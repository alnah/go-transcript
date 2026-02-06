package cli

import (
	"bytes"
	"os"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Tests for DefaultEnv
// ---------------------------------------------------------------------------

func TestDefaultEnvReturnsValidEnv(t *testing.T) {
	t.Parallel()

	env := DefaultEnv()

	if env == nil {
		t.Fatal("DefaultEnv() returned nil")
	}

	// Verify all fields are set
	if env.Stderr == nil {
		t.Error("DefaultEnv() Stderr = nil, want non-nil")
	}
	if env.Getenv == nil {
		t.Error("DefaultEnv() Getenv = nil, want non-nil")
	}
	if env.Now == nil {
		t.Error("DefaultEnv() Now = nil, want non-nil")
	}
	if env.FFmpegResolver == nil {
		t.Error("DefaultEnv() FFmpegResolver = nil, want non-nil")
	}
	if env.ConfigLoader == nil {
		t.Error("DefaultEnv() ConfigLoader = nil, want non-nil")
	}
	if env.TranscriberFactory == nil {
		t.Error("DefaultEnv() TranscriberFactory = nil, want non-nil")
	}
	if env.RestructurerFactory == nil {
		t.Error("DefaultEnv() RestructurerFactory = nil, want non-nil")
	}
	if env.ChunkerFactory == nil {
		t.Error("DefaultEnv() ChunkerFactory = nil, want non-nil")
	}
	if env.RecorderFactory == nil {
		t.Error("DefaultEnv() RecorderFactory = nil, want non-nil")
	}
}

func TestDefaultEnvStderrIsOsStderr(t *testing.T) {
	t.Parallel()

	env := DefaultEnv()

	if env.Stderr != os.Stderr {
		t.Errorf("DefaultEnv() Stderr = %v, want os.Stderr", env.Stderr)
	}
}

func TestDefaultEnvGetenvUsesOsGetenv(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	testKey := "GO_TRANSCRIPT_TEST_KEY_12345"
	testValue := "test_value_xyz"
	t.Setenv(testKey, testValue)

	env := DefaultEnv()

	result := env.Getenv(testKey)
	if result != testValue {
		t.Errorf("DefaultEnv().Getenv(%q) = %q, want %q", testKey, result, testValue)
	}
}

func TestDefaultEnvNowReturnsCurrentTime(t *testing.T) {
	t.Parallel()

	env := DefaultEnv()

	before := time.Now()
	result := env.Now()
	after := time.Now()

	if result.Before(before) || result.After(after) {
		t.Errorf("DefaultEnv().Now() = %v, want time between %v and %v", result, before, after)
	}
}

// ---------------------------------------------------------------------------
// Tests for NewEnv with options
// ---------------------------------------------------------------------------

func TestNewEnvWithStderr(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	env := NewEnv(WithStderr(buf))

	if env.Stderr != buf {
		t.Errorf("NewEnv(WithStderr(buf)) Stderr = %v, want %v", env.Stderr, buf)
	}
}

func TestNewEnvWithGetenv(t *testing.T) {
	t.Parallel()

	customGetenv := func(key string) string {
		if key == "TEST" {
			return "custom_value"
		}
		return ""
	}

	env := NewEnv(WithGetenv(customGetenv))

	result := env.Getenv("TEST")
	if result != "custom_value" {
		t.Errorf("NewEnv(WithGetenv(customGetenv)).Getenv(%q) = %q, want %q", "TEST", result, "custom_value")
	}
}

func TestNewEnvWithNow(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	customNow := func() time.Time {
		return fixedTime
	}

	env := NewEnv(WithNow(customNow))

	result := env.Now()
	if !result.Equal(fixedTime) {
		t.Errorf("NewEnv(WithNow(customNow)).Now() = %v, want %v", result, fixedTime)
	}
}

func TestNewEnvWithFFmpegResolver(t *testing.T) {
	t.Parallel()

	resolver := &mockFFmpegResolver{}
	env := NewEnv(WithFFmpegResolver(resolver))

	if env.FFmpegResolver != resolver {
		t.Errorf("NewEnv(WithFFmpegResolver(resolver)) FFmpegResolver = %v, want %v", env.FFmpegResolver, resolver)
	}
}

func TestNewEnvWithConfigLoader(t *testing.T) {
	t.Parallel()

	loader := &mockConfigLoader{}
	env := NewEnv(WithConfigLoader(loader))

	if env.ConfigLoader != loader {
		t.Errorf("NewEnv(WithConfigLoader(loader)) ConfigLoader = %v, want %v", env.ConfigLoader, loader)
	}
}

func TestNewEnvWithTranscriberFactory(t *testing.T) {
	t.Parallel()

	factory := &mockTranscriberFactory{}
	env := NewEnv(WithTranscriberFactory(factory))

	if env.TranscriberFactory != factory {
		t.Errorf("NewEnv(WithTranscriberFactory(factory)) TranscriberFactory = %v, want %v", env.TranscriberFactory, factory)
	}
}

func TestNewEnvWithRestructurerFactory(t *testing.T) {
	t.Parallel()

	factory := &mockRestructurerFactory{}
	env := NewEnv(WithRestructurerFactory(factory))

	if env.RestructurerFactory != factory {
		t.Errorf("NewEnv(WithRestructurerFactory(factory)) RestructurerFactory = %v, want %v", env.RestructurerFactory, factory)
	}
}

func TestNewEnvWithChunkerFactory(t *testing.T) {
	t.Parallel()

	factory := &mockChunkerFactory{}
	env := NewEnv(WithChunkerFactory(factory))

	if env.ChunkerFactory != factory {
		t.Errorf("NewEnv(WithChunkerFactory(factory)) ChunkerFactory = %v, want %v", env.ChunkerFactory, factory)
	}
}

func TestNewEnvWithRecorderFactory(t *testing.T) {
	t.Parallel()

	factory := &mockRecorderFactory{}
	env := NewEnv(WithRecorderFactory(factory))

	if env.RecorderFactory != factory {
		t.Errorf("NewEnv(WithRecorderFactory(factory)) RecorderFactory = %v, want %v", env.RecorderFactory, factory)
	}
}

func TestNewEnvMultipleOptions(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	customGetenv := func(string) string { return "custom" }

	env := NewEnv(
		WithStderr(buf),
		WithGetenv(customGetenv),
		WithNow(func() time.Time { return fixedTime }),
	)

	if env.Stderr != buf {
		t.Errorf("NewEnv(...) Stderr = %v, want %v", env.Stderr, buf)
	}
	if env.Getenv("any") != "custom" {
		t.Errorf("NewEnv(...).Getenv(%q) = %q, want %q", "any", env.Getenv("any"), "custom")
	}
	if !env.Now().Equal(fixedTime) {
		t.Errorf("NewEnv(...).Now() = %v, want %v", env.Now(), fixedTime)
	}
}

func TestNewEnvOptionsOverrideDefaults(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	env := NewEnv(WithStderr(buf))

	// Custom option should override default
	if env.Stderr != buf {
		t.Errorf("NewEnv(WithStderr(buf)) Stderr = %v, want %v", env.Stderr, buf)
	}

	// Other defaults should still be set
	if env.Getenv == nil {
		t.Error("NewEnv(WithStderr(buf)) Getenv = nil, want non-nil")
	}
	if env.FFmpegResolver == nil {
		t.Error("NewEnv(WithStderr(buf)) FFmpegResolver = nil, want non-nil")
	}
}

func TestNewEnvNoOptions(t *testing.T) {
	t.Parallel()

	env := NewEnv()

	// Should behave like DefaultEnv
	if env.Stderr == nil {
		t.Error("NewEnv() Stderr = nil, want non-nil")
	}
	if env.FFmpegResolver == nil {
		t.Error("NewEnv() FFmpegResolver = nil, want non-nil")
	}
}
