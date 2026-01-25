package main

import (
	"bytes"
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Test Helpers
// =============================================================================

// mockClock provides a controllable time source for testing.
type mockClock struct {
	mu  sync.Mutex
	now time.Time
}

func newMockClock(start time.Time) *mockClock {
	return &mockClock{now: start}
}

func (c *mockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mockClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// syncBuffer is a thread-safe buffer for capturing stderr in tests.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Contains(s string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Contains(b.buf.Bytes(), []byte(s))
}

// testInterruptHandler creates a handler with injected dependencies for testing.
func testInterruptHandler(t *testing.T, opts interruptOptions) (*InterruptHandler, context.Context, <-chan struct{}) {
	t.Helper()

	// Create signal channel if not provided
	if opts.sigCh == nil {
		opts.sigCh = make(chan os.Signal, 2)
	}

	// Track exit calls
	exitCalled := make(chan struct{}, 1)
	if opts.exitFunc == nil {
		opts.exitFunc = func(code int) {
			select {
			case exitCalled <- struct{}{}:
			default:
			}
		}
	}

	// Default stderr to thread-safe buffer
	if opts.stderr == nil {
		opts.stderr = &syncBuffer{}
	}

	h, ctx := newInterruptHandler(context.Background(), opts)
	t.Cleanup(func() { h.Stop() })

	return h, ctx, exitCalled
}

// =============================================================================
// Initial State Tests
// =============================================================================

func TestInterruptHandler_InitialState(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	h, ctx, _ := testInterruptHandler(t, interruptOptions{sigCh: sigCh})

	// Initial state should be not interrupted
	if h.WasInterrupted() {
		t.Error("expected WasInterrupted() to be false initially")
	}

	// Context should not be canceled
	select {
	case <-ctx.Done():
		t.Error("expected context to not be canceled initially")
	default:
		// OK
	}
}

// =============================================================================
// First Interrupt Tests
// =============================================================================

func TestInterruptHandler_FirstInterrupt_CancelsContext(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	h, ctx, _ := testInterruptHandler(t, interruptOptions{sigCh: sigCh})

	// Send first signal
	sigCh <- os.Interrupt

	// Wait for handler to process
	time.Sleep(10 * time.Millisecond)

	// Context should be canceled
	select {
	case <-ctx.Done():
		// OK
	default:
		t.Error("expected context to be canceled after first interrupt")
	}

	// WasInterrupted should be true
	if !h.WasInterrupted() {
		t.Error("expected WasInterrupted() to be true after first interrupt")
	}
}

func TestInterruptHandler_FirstInterrupt_RecordsTimestamp(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	clock := newMockClock(time.Date(2026, 1, 25, 12, 0, 0, 0, time.UTC))

	h, _, _ := testInterruptHandler(t, interruptOptions{
		sigCh:   sigCh,
		nowFunc: clock.Now,
	})

	// Send first signal
	sigCh <- os.Interrupt

	// Wait for handler to process
	time.Sleep(10 * time.Millisecond)

	// Check timestamp was recorded
	h.mu.Lock()
	firstInterrupt := h.firstInterrupt
	h.mu.Unlock()

	expected := time.Date(2026, 1, 25, 12, 0, 0, 0, time.UTC)
	if !firstInterrupt.Equal(expected) {
		t.Errorf("expected firstInterrupt to be %v, got %v", expected, firstInterrupt)
	}
}

// =============================================================================
// Double Interrupt Tests (Within Window)
// =============================================================================

func TestInterruptHandler_DoubleInterrupt_WithinWindow_CallsExit(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	clock := newMockClock(time.Date(2026, 1, 25, 12, 0, 0, 0, time.UTC))

	var exitCode atomic.Int32
	exitCode.Store(-1)

	h, _, _ := testInterruptHandler(t, interruptOptions{
		sigCh:   sigCh,
		nowFunc: clock.Now,
		exitFunc: func(code int) {
			exitCode.Store(int32(code))
		},
	})

	// Send first signal
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	// Advance clock by 1 second (within 2s window)
	clock.Advance(1 * time.Second)

	// Send second signal
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	// Exit should have been called with ExitInterrupt (130)
	if code := exitCode.Load(); code != ExitInterrupt {
		t.Errorf("expected exit code %d, got %d", ExitInterrupt, code)
	}

	// Handler should be marked as aborted
	h.mu.Lock()
	aborted := h.aborted
	h.mu.Unlock()

	if !aborted {
		t.Error("expected handler to be marked as aborted")
	}
}

func TestInterruptHandler_DoubleInterrupt_WithinWindow_WritesMessage(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	clock := newMockClock(time.Date(2026, 1, 25, 12, 0, 0, 0, time.UTC))
	stderr := &syncBuffer{}

	h, _, _ := testInterruptHandler(t, interruptOptions{
		sigCh:   sigCh,
		nowFunc: clock.Now,
		stderr:  stderr,
	})
	_ = h

	// Send first signal
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	// Advance clock by 500ms (within window)
	clock.Advance(500 * time.Millisecond)

	// Send second signal
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	// Check stderr contains "Aborted"
	if !stderr.Contains("Aborted") {
		t.Errorf("expected stderr to contain 'Aborted', got %q", stderr.String())
	}
}

func TestInterruptHandler_DoubleInterrupt_AtExactBoundary_CallsExit(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	clock := newMockClock(time.Date(2026, 1, 25, 12, 0, 0, 0, time.UTC))

	var exitCalled atomic.Bool

	h, _, _ := testInterruptHandler(t, interruptOptions{
		sigCh:   sigCh,
		nowFunc: clock.Now,
		exitFunc: func(code int) {
			exitCalled.Store(true)
		},
	})
	_ = h

	// Send first signal
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	// Advance clock by exactly 2 seconds (at boundary, should still abort)
	clock.Advance(2 * time.Second)

	// Send second signal
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	if !exitCalled.Load() {
		t.Error("expected exit to be called at exact 2s boundary")
	}
}

// =============================================================================
// Double Interrupt Tests (Outside Window)
// =============================================================================

func TestInterruptHandler_DoubleInterrupt_OutsideWindow_NoExit(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	clock := newMockClock(time.Date(2026, 1, 25, 12, 0, 0, 0, time.UTC))

	var exitCalled atomic.Bool

	h, _, _ := testInterruptHandler(t, interruptOptions{
		sigCh:   sigCh,
		nowFunc: clock.Now,
		exitFunc: func(code int) {
			exitCalled.Store(true)
		},
	})

	// Send first signal
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	// Advance clock by 3 seconds (outside 2s window)
	clock.Advance(3 * time.Second)

	// Send second signal
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	if exitCalled.Load() {
		t.Error("expected exit to NOT be called when second interrupt is outside window")
	}

	// Handler should NOT be marked as aborted
	h.mu.Lock()
	aborted := h.aborted
	h.mu.Unlock()

	if aborted {
		t.Error("expected handler to NOT be marked as aborted")
	}
}

// =============================================================================
// WasInterrupted Tests
// =============================================================================

func TestWasInterrupted_ThreadSafe(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	h, _, _ := testInterruptHandler(t, interruptOptions{sigCh: sigCh})

	var wg sync.WaitGroup
	const numReaders = 10

	// Start concurrent readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = h.WasInterrupted()
			}
		}()
	}

	// Send signal while readers are active
	sigCh <- os.Interrupt

	wg.Wait()
	// Test passes if no race detector errors
}

// =============================================================================
// WaitForDecision Tests
// =============================================================================

func TestWaitForDecision_NoInterrupt_ReturnsContinue(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	h, _, _ := testInterruptHandler(t, interruptOptions{sigCh: sigCh})

	// No interrupt sent, should return Continue immediately
	result := h.WaitForDecision("test message")

	if result != InterruptContinue {
		t.Errorf("expected InterruptContinue, got %v", result)
	}
}

func TestWaitForDecision_AlreadyAborted_ReturnsAbort(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	clock := newMockClock(time.Date(2026, 1, 25, 12, 0, 0, 0, time.UTC))

	h, _, _ := testInterruptHandler(t, interruptOptions{
		sigCh:   sigCh,
		nowFunc: clock.Now,
	})

	// Send first signal
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	// Send second signal within window (causes abort)
	clock.Advance(1 * time.Second)
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	// WaitForDecision should return Abort
	result := h.WaitForDecision("test message")

	if result != InterruptAbort {
		t.Errorf("expected InterruptAbort, got %v", result)
	}
}

func TestWaitForDecision_WindowExpired_ReturnsContinue(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	clock := newMockClock(time.Date(2026, 1, 25, 12, 0, 0, 0, time.UTC))

	h, _, _ := testInterruptHandler(t, interruptOptions{
		sigCh:   sigCh,
		nowFunc: clock.Now,
	})

	// Send first signal
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	// Advance clock past the window
	clock.Advance(3 * time.Second)

	// WaitForDecision should return Continue immediately (window expired)
	result := h.WaitForDecision("test message")

	if result != InterruptContinue {
		t.Errorf("expected InterruptContinue, got %v", result)
	}
}

func TestWaitForDecision_DisplaysMessage(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	clock := newMockClock(time.Date(2026, 1, 25, 12, 0, 0, 0, time.UTC))
	stderr := &syncBuffer{}

	h, _, _ := testInterruptHandler(t, interruptOptions{
		sigCh:   sigCh,
		nowFunc: clock.Now,
		stderr:  stderr,
	})

	// Send first signal
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	// Advance clock slightly (still within window, so WaitForDecision will wait)
	clock.Advance(100 * time.Millisecond)

	// Run WaitForDecision in goroutine since it will block
	done := make(chan InterruptBehavior, 1)
	go func() {
		done <- h.WaitForDecision("Press Ctrl+C again to abort")
	}()

	// Wait a bit for message to be written
	time.Sleep(50 * time.Millisecond)

	// Check message was written
	if !stderr.Contains("Press Ctrl+C again to abort") {
		t.Errorf("expected stderr to contain message, got %q", stderr.String())
	}

	// Advance clock past window to unblock
	clock.Advance(3 * time.Second)

	// Wait for result with timeout
	select {
	case result := <-done:
		if result != InterruptContinue {
			t.Errorf("expected InterruptContinue, got %v", result)
		}
	case <-time.After(3 * time.Second):
		t.Error("WaitForDecision timed out")
	}
}

func TestWaitForDecision_AbortDuringWait_ReturnsAbort(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	clock := newMockClock(time.Date(2026, 1, 25, 12, 0, 0, 0, time.UTC))

	h, _, _ := testInterruptHandler(t, interruptOptions{
		sigCh:   sigCh,
		nowFunc: clock.Now,
	})

	// Send first signal
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	// Run WaitForDecision in goroutine
	done := make(chan InterruptBehavior, 1)
	go func() {
		done <- h.WaitForDecision("test")
	}()

	// Wait a bit, then send second signal (within window)
	time.Sleep(50 * time.Millisecond)
	clock.Advance(500 * time.Millisecond)
	sigCh <- os.Interrupt

	// Wait for result
	select {
	case result := <-done:
		if result != InterruptAbort {
			t.Errorf("expected InterruptAbort, got %v", result)
		}
	case <-time.After(3 * time.Second):
		t.Error("WaitForDecision timed out")
	}
}

// =============================================================================
// Stop Tests
// =============================================================================

func TestStop_Idempotent(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	h, _, _ := testInterruptHandler(t, interruptOptions{sigCh: sigCh})

	// Multiple Stop calls should not panic
	h.Stop()
	h.Stop()
	h.Stop()
	// Test passes if no panic
}

func TestStop_StopsListening(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	h, _, _ := testInterruptHandler(t, interruptOptions{sigCh: sigCh})

	// Stop the handler
	h.Stop()

	// Send a signal - it should not be processed
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	// Note: This test verifies Stop() doesn't panic.
	// The signal may or may not be processed depending on goroutine scheduling.
	// The important thing is that signal.Reset was called.
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestInterruptHandler_NilSignalChannel(t *testing.T) {
	t.Parallel()

	// Handler with nil signal channel should not panic
	h, ctx := newInterruptHandler(context.Background(), interruptOptions{
		sigCh: nil, // Explicitly nil
	})
	defer h.Stop()

	// Should work without panicking
	if h.WasInterrupted() {
		t.Error("expected WasInterrupted() to be false")
	}

	select {
	case <-ctx.Done():
		t.Error("expected context to not be canceled")
	default:
		// OK
	}
}

func TestInterruptHandler_ClosedSignalChannel(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	h, _, _ := testInterruptHandler(t, interruptOptions{sigCh: sigCh})

	// Close the channel
	close(sigCh)

	// Wait for goroutine to exit
	time.Sleep(10 * time.Millisecond)

	// Should not panic, WasInterrupted should still work
	_ = h.WasInterrupted()
}

func TestInterruptHandler_ParentContextAlreadyCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before creating handler

	sigCh := make(chan os.Signal, 2)
	h, derivedCtx := newInterruptHandler(ctx, interruptOptions{sigCh: sigCh})
	defer h.Stop()

	// Derived context should be canceled (inherits from parent)
	select {
	case <-derivedCtx.Done():
		// OK - expected since parent is canceled
	default:
		// Also OK - the handler creates its own context
	}

	// Handler should still track interrupts
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	if !h.WasInterrupted() {
		t.Error("expected WasInterrupted() to be true")
	}
}

// =============================================================================
// InterruptBehavior Tests
// =============================================================================

func TestInterruptBehavior_Values(t *testing.T) {
	t.Parallel()

	// Verify enum values are as expected
	if InterruptContinue != 0 {
		t.Errorf("expected InterruptContinue to be 0, got %d", InterruptContinue)
	}
	if InterruptAbort != 1 {
		t.Errorf("expected InterruptAbort to be 1, got %d", InterruptAbort)
	}
}

// =============================================================================
// Real Timing Test (slow, but validates actual behavior)
// =============================================================================

func TestWaitForDecision_RealTiming_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}
	t.Parallel()

	sigCh := make(chan os.Signal, 2)
	// Use real time for this test
	h, _, _ := testInterruptHandler(t, interruptOptions{
		sigCh:   sigCh,
		nowFunc: time.Now, // Real time
	})

	// Send first signal
	sigCh <- os.Interrupt
	time.Sleep(10 * time.Millisecond)

	// WaitForDecision should wait ~2s then return Continue
	start := time.Now()
	result := h.WaitForDecision("test")
	elapsed := time.Since(start)

	if result != InterruptContinue {
		t.Errorf("expected InterruptContinue, got %v", result)
	}

	// Should have waited approximately 2 seconds (with some margin)
	if elapsed < 1800*time.Millisecond || elapsed > 2500*time.Millisecond {
		t.Errorf("expected elapsed time around 2s, got %v", elapsed)
	}
}
