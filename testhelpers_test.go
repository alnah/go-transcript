package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// =============================================================================
// Mock OpenAI Chat Completer
// =============================================================================

// mockChatCompleter implements chatCompleter interface for testing restructurer.
// Supports response sequences for retry testing.
type mockChatCompleter struct {
	mu        sync.Mutex
	responses []mockChatResponse
	calls     []openai.ChatCompletionRequest
	callIndex int
}

type mockChatResponse struct {
	response openai.ChatCompletionResponse
	err      error
}

// newMockChatCompleter creates a mock that returns the given responses in sequence.
// If more calls are made than responses provided, the last response is repeated.
func newMockChatCompleter(responses ...mockChatResponse) *mockChatCompleter {
	return &mockChatCompleter{
		responses: responses,
	}
}

// withChatSuccess creates a mock that always returns the given content.
func withChatSuccess(content string) *mockChatCompleter {
	return newMockChatCompleter(mockChatResponse{
		response: openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: content}},
			},
		},
	})
}

// withChatError creates a mock that always returns the given error.
func withChatError(err error) *mockChatCompleter {
	return newMockChatCompleter(mockChatResponse{err: err})
}

// withChatSequence creates a mock that returns different responses in sequence.
// Useful for testing retry logic (e.g., first call fails, second succeeds).
func withChatSequence(responses ...mockChatResponse) *mockChatCompleter {
	return newMockChatCompleter(responses...)
}

func (m *mockChatCompleter) CreateChatCompletion(_ context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, req)

	if len(m.responses) == 0 {
		return openai.ChatCompletionResponse{}, nil
	}

	idx := m.callIndex
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1 // Repeat last response
	}
	m.callIndex++

	resp := m.responses[idx]
	return resp.response, resp.err
}

// CallCount returns the number of times CreateChatCompletion was called.
func (m *mockChatCompleter) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// LastRequest returns the most recent request, or nil if no calls were made.
func (m *mockChatCompleter) LastRequest() *openai.ChatCompletionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return nil
	}
	return &m.calls[len(m.calls)-1]
}

// AllRequests returns all requests made to this mock.
func (m *mockChatCompleter) AllRequests() []openai.ChatCompletionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]openai.ChatCompletionRequest, len(m.calls))
	copy(result, m.calls)
	return result
}

// =============================================================================
// Mock Audio Transcriber
// =============================================================================

// mockAudioTranscriber implements audioTranscriber interface for testing transcriber.
// Supports response sequences for retry testing.
type mockAudioTranscriber struct {
	mu        sync.Mutex
	responses []mockAudioResponse
	calls     []openai.AudioRequest
	callIndex int
}

type mockAudioResponse struct {
	response openai.AudioResponse
	err      error
}

// newMockAudioTranscriber creates a mock that returns the given responses in sequence.
func newMockAudioTranscriber(responses ...mockAudioResponse) *mockAudioTranscriber {
	return &mockAudioTranscriber{
		responses: responses,
	}
}

// withAudioSuccess creates a mock that always returns the given text.
func withAudioSuccess(text string) *mockAudioTranscriber {
	return newMockAudioTranscriber(mockAudioResponse{
		response: openai.AudioResponse{Text: text},
	})
}

// withAudioDiarized creates a mock that returns a diarized response with segments.
// segments is a slice of [id, text] pairs.
func withAudioDiarized(segments [][2]any) *mockAudioTranscriber {
	resp := openai.AudioResponse{}
	for _, seg := range segments {
		id, _ := seg[0].(int)
		text, _ := seg[1].(string)
		resp.Segments = append(resp.Segments, struct {
			ID               int     `json:"id"`
			Seek             int     `json:"seek"`
			Start            float64 `json:"start"`
			End              float64 `json:"end"`
			Text             string  `json:"text"`
			Tokens           []int   `json:"tokens"`
			Temperature      float64 `json:"temperature"`
			AvgLogprob       float64 `json:"avg_logprob"`
			CompressionRatio float64 `json:"compression_ratio"`
			NoSpeechProb     float64 `json:"no_speech_prob"`
			Transient        bool    `json:"transient"`
		}{ID: id, Text: text})
	}
	return newMockAudioTranscriber(mockAudioResponse{response: resp})
}

// withAudioError creates a mock that always returns the given error.
func withAudioError(err error) *mockAudioTranscriber {
	return newMockAudioTranscriber(mockAudioResponse{err: err})
}

// withAudioSequence creates a mock that returns different responses in sequence.
func withAudioSequence(responses ...mockAudioResponse) *mockAudioTranscriber {
	return newMockAudioTranscriber(responses...)
}

func (m *mockAudioTranscriber) CreateTranscription(_ context.Context, req openai.AudioRequest) (openai.AudioResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, req)

	if len(m.responses) == 0 {
		return openai.AudioResponse{}, nil
	}

	idx := m.callIndex
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	m.callIndex++

	resp := m.responses[idx]
	return resp.response, resp.err
}

// CallCount returns the number of times CreateTranscription was called.
func (m *mockAudioTranscriber) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// LastRequest returns the most recent request, or nil if no calls were made.
func (m *mockAudioTranscriber) LastRequest() *openai.AudioRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return nil
	}
	return &m.calls[len(m.calls)-1]
}

// AllRequests returns all requests made to this mock.
func (m *mockAudioTranscriber) AllRequests() []openai.AudioRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]openai.AudioRequest, len(m.calls))
	copy(result, m.calls)
	return result
}

// =============================================================================
// Mock FFmpeg Runner
// =============================================================================

// mockFFmpegResponse holds the result of a mock FFmpeg execution.
type mockFFmpegResponse struct {
	output string
	err    error
}

// ffmpegOutputFunc is the function signature for runFFmpegOutput.
// Tests can replace runFFmpegOutputVar to inject mock behavior.
type ffmpegOutputFunc func(ctx context.Context, ffmpegPath string, args []string) (string, error)

// mockFFmpegRunner manages FFmpeg mock state for tests.
type mockFFmpegRunner struct {
	mu        sync.Mutex
	responses []mockFFmpegResponse
	calls     [][]string // Each call's args
	callIndex int
}

// newMockFFmpegRunner creates a new mock runner.
func newMockFFmpegRunner(responses ...mockFFmpegResponse) *mockFFmpegRunner {
	return &mockFFmpegRunner{
		responses: responses,
	}
}

// withFFmpegOutput creates a mock that always returns the given output.
func withFFmpegOutput(output string) *mockFFmpegRunner {
	return newMockFFmpegRunner(mockFFmpegResponse{output: output})
}

// withFFmpegError creates a mock that always returns the given error.
func withFFmpegError(err error) *mockFFmpegRunner {
	return newMockFFmpegRunner(mockFFmpegResponse{err: err})
}

// Run implements the mock FFmpeg execution.
func (m *mockFFmpegRunner) Run(_ context.Context, _ string, args []string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, args)

	if len(m.responses) == 0 {
		return "", nil
	}

	idx := m.callIndex
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	m.callIndex++

	resp := m.responses[idx]
	return resp.output, resp.err
}

// CallCount returns the number of times Run was called.
func (m *mockFFmpegRunner) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// installFFmpegMock replaces the global runFFmpegOutputFunc for testing.
// Returns a cleanup function that restores the original.
// Use with t.Cleanup: t.Cleanup(installFFmpegMock(t, mock))
func installFFmpegMock(t *testing.T, mock *mockFFmpegRunner) func() {
	t.Helper()
	original := runFFmpegOutputFunc
	runFFmpegOutputFunc = mock.Run
	return func() {
		runFFmpegOutputFunc = original
	}
}

// =============================================================================
// Mock Exec Command Runner
// =============================================================================

// mockExecResponse holds the result of a mock command execution.
type mockExecResponse struct {
	output []byte
	err    error
}

// mockExecRunner manages exec.Command mock state for tests.
// Supports routing responses by command name.
type mockExecRunner struct {
	mu        sync.Mutex
	responses map[string][]mockExecResponse // command name -> responses
	callIndex map[string]int                // command name -> current index
	calls     []execCall                    // all calls for inspection
}

// execCall records a single exec.Command call for verification.
type execCall struct {
	name string
	args []string
}

// newMockExecRunner creates a new mock runner.
func newMockExecRunner() *mockExecRunner {
	return &mockExecRunner{
		responses: make(map[string][]mockExecResponse),
		callIndex: make(map[string]int),
	}
}

// OnCommand configures the mock to return the given output when the command is called.
// Can be called multiple times to set up sequences.
func (m *mockExecRunner) OnCommand(name string, output []byte, err error) *mockExecRunner {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[name] = append(m.responses[name], mockExecResponse{output: output, err: err})
	return m
}

// Run implements the mock command execution.
func (m *mockExecRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, execCall{name: name, args: args})

	responses, ok := m.responses[name]
	if !ok || len(responses) == 0 {
		return nil, errors.New("command not found: " + name)
	}

	idx := m.callIndex[name]
	if idx >= len(responses) {
		idx = len(responses) - 1 // Repeat last response
	}
	m.callIndex[name]++

	resp := responses[idx]
	return resp.output, resp.err
}

// CallCount returns the number of times a specific command was called.
func (m *mockExecRunner) CallCount(name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, c := range m.calls {
		if c.name == name {
			count++
		}
	}
	return count
}

// TotalCallCount returns the total number of calls made.
func (m *mockExecRunner) TotalCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// installExecMock replaces the global execCommandOutput for testing.
// Returns a cleanup function that restores the original.
// Use with t.Cleanup: t.Cleanup(installExecMock(t, mock))
func installExecMock(t *testing.T, mock *mockExecRunner) func() {
	t.Helper()
	original := execCommandOutput
	execCommandOutput = mock.Run
	return func() {
		execCommandOutput = original
	}
}

// =============================================================================
// Filesystem Helpers
// =============================================================================

// tempFile creates a temporary file with the given content.
// The file is automatically cleaned up when the test ends.
func tempFile(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	return path
}

// tempFileWithName creates a temporary file with a specific name.
func tempFileWithName(t *testing.T, name, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	return path
}

// tempAudioFile creates a minimal valid OGG file for testing.
// This is not a real audio file but has the OGG magic bytes.
func tempAudioFile(t *testing.T) string {
	t.Helper()

	// OGG magic bytes + minimal header (not playable, but recognizable as OGG)
	oggHeader := []byte{
		0x4F, 0x67, 0x67, 0x53, // "OggS" magic
		0x00,                   // version
		0x02,                   // header type (BOS)
		0x00, 0x00, 0x00, 0x00, // granule position (8 bytes)
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, // serial number
		0x00, 0x00, 0x00, 0x00, // page sequence
		0x00, 0x00, 0x00, 0x00, // checksum
		0x01,       // segment count
		0x00,       // segment table
		0x00, 0x00, // padding
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.ogg")
	if err := os.WriteFile(path, oggHeader, 0o644); err != nil {
		t.Fatalf("failed to create temp audio file: %v", err)
	}
	return path
}

// tempConfigDir creates a temporary config directory structure.
// Returns the path to the config file (which may or may not exist).
func tempConfigDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	configDir := filepath.Join(dir, ".config", "go-transcript")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config directory: %v", err)
	}
	return filepath.Join(configDir, "config.json")
}

// =============================================================================
// Assertion Helpers
// =============================================================================

// assertError checks that err wraps target using errors.Is.
func assertError(t *testing.T, err, target error) {
	t.Helper()

	if err == nil {
		t.Errorf("expected error wrapping %v, got nil", target)
		return
	}
	if target == nil {
		t.Errorf("target error is nil, use assertNoError instead")
		return
	}
	if !errorIs(err, target) {
		t.Errorf("expected error wrapping %v, got %v", target, err)
	}
}

// assertNoError fails if err is not nil.
func assertNoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// assertContains checks that haystack contains needle.
func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()

	if !containsString(haystack, needle) {
		t.Errorf("expected %q to contain %q", truncate(haystack, 100), needle)
	}
}

// assertNotContains checks that haystack does not contain needle.
func assertNotContains(t *testing.T, haystack, needle string) {
	t.Helper()

	if containsString(haystack, needle) {
		t.Errorf("expected %q to not contain %q", truncate(haystack, 100), needle)
	}
}

// assertEqual checks that got equals want.
func assertEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()

	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// assertFileExists checks that the file at path exists.
func assertFileExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file to exist: %s", path)
	}
}

// assertFileContains checks that the file at path contains content.
func assertFileContains(t *testing.T, path, content string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("failed to read file %s: %v", path, err)
		return
	}
	if !containsString(string(data), content) {
		t.Errorf("file %s does not contain %q", path, content)
	}
}

// =============================================================================
// OpenAI Error Helpers
// =============================================================================

// apiError creates an OpenAI APIError with the given HTTP status code and message.
// Use for testing error classification and retry logic.
func apiError(statusCode int, message string) *openai.APIError {
	return &openai.APIError{
		HTTPStatusCode: statusCode,
		Message:        message,
	}
}

// =============================================================================
// Helper utilities (not exported)
// =============================================================================

// errorIs wraps errors.Is for use in assertions.
func errorIs(err, target error) bool {
	return errors.Is(err, target)
}

// containsString checks if s contains substr.
func containsString(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && searchString(s, substr))
}

// searchString does a simple substring search.
func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// truncate shortens a string for display in error messages.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// =============================================================================
// Fixture Helpers
// =============================================================================

// loadFixture reads a file from testdata/ directory.
func loadFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("failed to load fixture %s: %v", name, err)
	}
	return string(data)
}

// silenceDetectOutput returns sample FFmpeg silencedetect output for testing.
func silenceDetectOutput(silences [][2]float64, durationSec float64) string {
	var output string
	for _, s := range silences {
		output += "[silencedetect @ 0x0] silence_start: " + formatFloat(s[0]) + "\n"
		output += "[silencedetect @ 0x0] silence_end: " + formatFloat(s[1]) + " | silence_duration: " + formatFloat(s[1]-s[0]) + "\n"
	}
	// Add duration info
	h := int(durationSec) / 3600
	m := (int(durationSec) % 3600) / 60
	sec := durationSec - float64(h*3600+m*60)
	output += "Duration: " + formatTime(h, m, sec) + "\n"
	return output
}

// formatFloat formats a float for FFmpeg output simulation.
func formatFloat(f float64) string {
	return string(formatFloatBytes(f))
}

func formatFloatBytes(f float64) []byte {
	// Simple float formatting without importing strconv
	intPart := int(f)
	fracPart := int((f - float64(intPart)) * 1000)
	if fracPart < 0 {
		fracPart = -fracPart
	}
	return []byte(itoa(intPart) + "." + padZeros(itoa(fracPart), 3))
}

func formatTime(h, m int, s float64) string {
	return padZeros(itoa(h), 2) + ":" + padZeros(itoa(m), 2) + ":" + formatFloat(s)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func padZeros(s string, width int) string {
	for len(s) < width {
		s = "0" + s
	}
	return s
}
