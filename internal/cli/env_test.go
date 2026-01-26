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

func TestDefaultEnv_ReturnsValidEnv(t *testing.T) {
	t.Parallel()

	env := DefaultEnv()

	if env == nil {
		t.Fatal("DefaultEnv should not return nil")
	}

	// Verify all fields are set
	if env.Stderr == nil {
		t.Error("Stderr should be set")
	}
	if env.Getenv == nil {
		t.Error("Getenv should be set")
	}
	if env.Now == nil {
		t.Error("Now should be set")
	}
	if env.FFmpegResolver == nil {
		t.Error("FFmpegResolver should be set")
	}
	if env.ConfigLoader == nil {
		t.Error("ConfigLoader should be set")
	}
	if env.TranscriberFactory == nil {
		t.Error("TranscriberFactory should be set")
	}
	if env.RestructurerFactory == nil {
		t.Error("RestructurerFactory should be set")
	}
	if env.ChunkerFactory == nil {
		t.Error("ChunkerFactory should be set")
	}
	if env.RecorderFactory == nil {
		t.Error("RecorderFactory should be set")
	}
}

func TestDefaultEnv_StderrIsOsStderr(t *testing.T) {
	t.Parallel()

	env := DefaultEnv()

	if env.Stderr != os.Stderr {
		t.Error("Stderr should be os.Stderr by default")
	}
}

func TestDefaultEnv_GetenvUsesOsGetenv(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	testKey := "GO_TRANSCRIPT_TEST_KEY_12345"
	testValue := "test_value_xyz"
	t.Setenv(testKey, testValue)

	env := DefaultEnv()

	result := env.Getenv(testKey)
	if result != testValue {
		t.Errorf("Getenv should use os.Getenv, got %q, want %q", result, testValue)
	}
}

func TestDefaultEnv_NowReturnsCurrentTime(t *testing.T) {
	t.Parallel()

	env := DefaultEnv()

	before := time.Now()
	result := env.Now()
	after := time.Now()

	if result.Before(before) || result.After(after) {
		t.Errorf("Now() should return current time, got %v (expected between %v and %v)", result, before, after)
	}
}

// ---------------------------------------------------------------------------
// Tests for NewEnv with options
// ---------------------------------------------------------------------------

func TestNewEnv_WithStderr(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	env := NewEnv(WithStderr(buf))

	if env.Stderr != buf {
		t.Error("WithStderr should set custom stderr")
	}
}

func TestNewEnv_WithGetenv(t *testing.T) {
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
		t.Errorf("WithGetenv should set custom getenv, got %q", result)
	}
}

func TestNewEnv_WithNow(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	customNow := func() time.Time {
		return fixedTime
	}

	env := NewEnv(WithNow(customNow))

	result := env.Now()
	if !result.Equal(fixedTime) {
		t.Errorf("WithNow should set custom time provider, got %v, want %v", result, fixedTime)
	}
}

func TestNewEnv_WithFFmpegResolver(t *testing.T) {
	t.Parallel()

	resolver := &mockFFmpegResolver{}
	env := NewEnv(WithFFmpegResolver(resolver))

	if env.FFmpegResolver != resolver {
		t.Error("WithFFmpegResolver should set custom resolver")
	}
}

func TestNewEnv_WithConfigLoader(t *testing.T) {
	t.Parallel()

	loader := &mockConfigLoader{}
	env := NewEnv(WithConfigLoader(loader))

	if env.ConfigLoader != loader {
		t.Error("WithConfigLoader should set custom loader")
	}
}

func TestNewEnv_WithTranscriberFactory(t *testing.T) {
	t.Parallel()

	factory := &mockTranscriberFactory{}
	env := NewEnv(WithTranscriberFactory(factory))

	if env.TranscriberFactory != factory {
		t.Error("WithTranscriberFactory should set custom factory")
	}
}

func TestNewEnv_WithRestructurerFactory(t *testing.T) {
	t.Parallel()

	factory := &mockRestructurerFactory{}
	env := NewEnv(WithRestructurerFactory(factory))

	if env.RestructurerFactory != factory {
		t.Error("WithRestructurerFactory should set custom factory")
	}
}

func TestNewEnv_WithChunkerFactory(t *testing.T) {
	t.Parallel()

	factory := &mockChunkerFactory{}
	env := NewEnv(WithChunkerFactory(factory))

	if env.ChunkerFactory != factory {
		t.Error("WithChunkerFactory should set custom factory")
	}
}

func TestNewEnv_WithRecorderFactory(t *testing.T) {
	t.Parallel()

	factory := &mockRecorderFactory{}
	env := NewEnv(WithRecorderFactory(factory))

	if env.RecorderFactory != factory {
		t.Error("WithRecorderFactory should set custom factory")
	}
}

func TestNewEnv_MultipleOptions(t *testing.T) {
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
		t.Error("Stderr should be set")
	}
	if env.Getenv("any") != "custom" {
		t.Error("Getenv should be set")
	}
	if !env.Now().Equal(fixedTime) {
		t.Error("Now should be set")
	}
}

func TestNewEnv_OptionsOverrideDefaults(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	env := NewEnv(WithStderr(buf))

	// Custom option should override default
	if env.Stderr != buf {
		t.Error("custom stderr should override default")
	}

	// Other defaults should still be set
	if env.Getenv == nil {
		t.Error("Getenv should still have default")
	}
	if env.FFmpegResolver == nil {
		t.Error("FFmpegResolver should still have default")
	}
}

func TestNewEnv_NoOptions(t *testing.T) {
	t.Parallel()

	env := NewEnv()

	// Should behave like DefaultEnv
	if env.Stderr == nil {
		t.Error("Stderr should be set even with no options")
	}
	if env.FFmpegResolver == nil {
		t.Error("FFmpegResolver should be set even with no options")
	}
}
