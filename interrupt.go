package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// InterruptBehavior defines what happens after the first Ctrl+C.
type InterruptBehavior int

const (
	// InterruptContinue means continue with partial work (e.g., transcribe partial recording).
	InterruptContinue InterruptBehavior = iota
	// InterruptAbort means discard all work and exit.
	InterruptAbort
)

// interruptWindow is the time window for a second Ctrl+C to trigger abort.
const interruptWindow = 2 * time.Second

// InterruptHandler manages graceful interrupt handling with double Ctrl+C detection.
// First Ctrl+C triggers early stop with continuation.
// Second Ctrl+C within the window triggers abort.
type InterruptHandler struct {
	mu             sync.Mutex
	firstInterrupt time.Time
	interrupted    bool
	aborted        bool
	cancelFunc     context.CancelFunc

	// Injected dependencies (for testing)
	exitFunc func(int)
	nowFunc  func() time.Time
	stderr   interface{ Write([]byte) (int, error) }
}

// interruptOptions holds injectable dependencies for testing.
type interruptOptions struct {
	sigCh    <-chan os.Signal
	exitFunc func(int)
	nowFunc  func() time.Time
	stderr   interface{ Write([]byte) (int, error) }
}

// NewInterruptHandler creates a handler that listens for SIGINT/SIGTERM.
// Returns the handler and a context that is canceled on first interrupt.
func NewInterruptHandler(parent context.Context) (*InterruptHandler, context.Context) {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	return newInterruptHandler(parent, interruptOptions{sigCh: sigCh})
}

// newInterruptHandler creates a handler with injectable dependencies.
// Used by tests to inject mock signal channels, exit functions, and clocks.
func newInterruptHandler(parent context.Context, opts interruptOptions) (*InterruptHandler, context.Context) {
	ctx, cancel := context.WithCancel(parent)

	// Apply defaults for nil options
	exitFunc := opts.exitFunc
	if exitFunc == nil {
		exitFunc = os.Exit
	}
	nowFunc := opts.nowFunc
	if nowFunc == nil {
		nowFunc = time.Now
	}
	stderr := opts.stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	h := &InterruptHandler{
		cancelFunc: cancel,
		exitFunc:   exitFunc,
		nowFunc:    nowFunc,
		stderr:     stderr,
	}

	go h.listen(opts.sigCh)

	return h, ctx
}

// listen handles incoming signals.
func (h *InterruptHandler) listen(sigCh <-chan os.Signal) {
	for range sigCh {
		h.mu.Lock()
		now := h.nowFunc()

		if !h.interrupted {
			// First interrupt
			h.interrupted = true
			h.firstInterrupt = now
			h.cancelFunc()
			h.mu.Unlock()
			continue
		}

		// Second interrupt - check if within window
		if now.Sub(h.firstInterrupt) <= interruptWindow {
			h.aborted = true
			h.mu.Unlock()
			// Exit immediately on double Ctrl+C
			fmt.Fprintln(h.stderr, "\nAborted.")
			h.exitFunc(ExitInterrupt)
			return // In case exitFunc doesn't actually exit (tests)
		}

		h.mu.Unlock()
	}
}

// WasInterrupted returns true if at least one interrupt was received.
func (h *InterruptHandler) WasInterrupted() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.interrupted
}

// WaitForDecision waits for the interrupt window and returns the user's intent.
// If a second Ctrl+C is received within the window, returns InterruptAbort.
// Otherwise, returns InterruptContinue after the timeout.
// The message parameter is displayed to guide the user.
func (h *InterruptHandler) WaitForDecision(message string) InterruptBehavior {
	h.mu.Lock()
	if !h.interrupted {
		h.mu.Unlock()
		return InterruptContinue
	}
	if h.aborted {
		h.mu.Unlock()
		return InterruptAbort
	}
	firstInterrupt := h.firstInterrupt
	h.mu.Unlock()

	// Calculate remaining time in window
	elapsed := h.nowFunc().Sub(firstInterrupt)
	remaining := interruptWindow - elapsed
	if remaining <= 0 {
		return InterruptContinue
	}

	// Display message and wait
	fmt.Fprintln(h.stderr, message)

	// Wait for remaining time or abort
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(remaining)

	for {
		select {
		case <-deadline:
			return InterruptContinue
		case <-ticker.C:
			h.mu.Lock()
			if h.aborted {
				h.mu.Unlock()
				return InterruptAbort
			}
			h.mu.Unlock()
		}
	}
}

// Stop cleans up the handler. Should be called when done.
func (h *InterruptHandler) Stop() {
	signal.Reset(syscall.SIGINT, syscall.SIGTERM)
}
