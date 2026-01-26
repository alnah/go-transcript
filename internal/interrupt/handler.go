package interrupt

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Behavior defines what happens after the first Ctrl+C.
type Behavior int

const (
	// Continue means continue with partial work (e.g., transcribe partial recording).
	Continue Behavior = iota
	// Abort means discard all work and exit.
	Abort
)

// String returns the string representation of the Behavior.
func (b Behavior) String() string {
	switch b {
	case Continue:
		return "Continue"
	case Abort:
		return "Abort"
	default:
		return fmt.Sprintf("Behavior(%d)", b)
	}
}

// ExitInterrupt is the exit code for interrupt (130 = 128 + SIGINT).
const ExitInterrupt = 130

// interruptWindow is the time window for a second Ctrl+C to trigger abort.
const interruptWindow = 2 * time.Second

// pollInterval is how often WaitForDecision checks for abort status.
const pollInterval = 100 * time.Millisecond

// abortMessage is the message displayed when the user aborts via double Ctrl+C.
const abortMessage = "\nAborted."

// Handler manages graceful interrupt handling with double Ctrl+C detection.
// First Ctrl+C triggers early stop with continuation.
// Second Ctrl+C within the window triggers abort.
type Handler struct {
	mu             sync.Mutex
	firstInterrupt time.Time
	interrupted    bool
	aborted        bool
	stopped        bool
	cancelFunc     context.CancelFunc
	done           chan struct{} // Signals listen goroutine to exit

	// Injected dependencies (for testing)
	exitFunc func(int)
	nowFunc  func() time.Time
	stderr   io.Writer
}

// Options holds injectable dependencies for testing.
type Options struct {
	SigCh    <-chan os.Signal
	ExitFunc func(int)
	NowFunc  func() time.Time
	// Stderr is the writer for user-facing messages.
	// Must be safe for concurrent writes from multiple goroutines.
	// Defaults to os.Stderr which is safe at the OS level.
	Stderr io.Writer
}

// NewHandler creates a handler that listens for SIGINT/SIGTERM.
// Returns the handler and a context that is canceled on first interrupt.
func NewHandler(parent context.Context) (*Handler, context.Context) {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	return newHandler(parent, Options{SigCh: sigCh})
}

// NewHandlerWithOptions creates a handler with injectable dependencies.
// Used by tests to inject mock signal channels, exit functions, and clocks.
func NewHandlerWithOptions(parent context.Context, opts Options) (*Handler, context.Context) {
	return newHandler(parent, opts)
}

// newHandler creates a handler with injectable dependencies.
func newHandler(parent context.Context, opts Options) (*Handler, context.Context) {
	ctx, cancel := context.WithCancel(parent)

	// Apply defaults for nil options
	exitFunc := opts.ExitFunc
	if exitFunc == nil {
		exitFunc = os.Exit
	}
	nowFunc := opts.NowFunc
	if nowFunc == nil {
		nowFunc = time.Now
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	h := &Handler{
		cancelFunc: cancel,
		done:       make(chan struct{}),
		exitFunc:   exitFunc,
		nowFunc:    nowFunc,
		stderr:     stderr,
	}

	// Only start listener if sigCh is provided (nil check for safety)
	if opts.SigCh != nil {
		go h.listen(opts.SigCh)
	}

	return h, ctx
}

// listen handles incoming signals.
func (h *Handler) listen(sigCh <-chan os.Signal) {
	for {
		select {
		case <-h.done:
			return
		case _, ok := <-sigCh:
			if !ok {
				return // Channel closed
			}

			h.mu.Lock()
			if h.stopped {
				h.mu.Unlock()
				return
			}
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
				fmt.Fprintln(h.stderr, abortMessage)
				h.exitFunc(ExitInterrupt)
				return // In case exitFunc doesn't actually exit (tests)
			}

			h.mu.Unlock()
		}
	}
}

// WasInterrupted returns true if at least one interrupt was received.
func (h *Handler) WasInterrupted() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.interrupted
}

// WaitForDecision waits for the interrupt window and returns the user's intent.
// If a second Ctrl+C is received within the window, returns Abort.
// Otherwise, returns Continue after the timeout.
// The message parameter is displayed to guide the user.
func (h *Handler) WaitForDecision(message string) Behavior {
	h.mu.Lock()
	if !h.interrupted {
		h.mu.Unlock()
		return Continue
	}
	if h.aborted {
		h.mu.Unlock()
		return Abort
	}
	firstInterrupt := h.firstInterrupt
	h.mu.Unlock()

	// Calculate remaining time in window
	elapsed := h.nowFunc().Sub(firstInterrupt)
	remaining := interruptWindow - elapsed
	if remaining <= 0 {
		return Continue
	}

	// Display message and wait
	fmt.Fprintln(h.stderr, message)

	// Wait for remaining time or abort
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	deadline := time.NewTimer(remaining)
	defer deadline.Stop()

	for {
		select {
		case <-deadline.C:
			return Continue
		case <-ticker.C:
			h.mu.Lock()
			if h.aborted {
				h.mu.Unlock()
				return Abort
			}
			h.mu.Unlock()
		}
	}
}

// Stop cleans up the handler. Should be called when done.
func (h *Handler) Stop() {
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		return
	}
	h.stopped = true
	h.mu.Unlock()

	signal.Reset(syscall.SIGINT, syscall.SIGTERM)
	close(h.done) // Signal listen goroutine to exit
}
