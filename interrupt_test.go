package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestInterruptHandler_NotInterrupted(t *testing.T) {
	ctx := context.Background()
	h, _ := NewInterruptHandler(ctx)
	defer h.Stop()

	if h.WasInterrupted() {
		t.Error("WasInterrupted() should be false initially")
	}
}

func TestInterruptHandler_WaitForDecision_NotInterrupted(t *testing.T) {
	ctx := context.Background()
	h, _ := NewInterruptHandler(ctx)
	defer h.Stop()

	// Should return immediately if not interrupted
	behavior := h.WaitForDecision("test message")
	if behavior != InterruptContinue {
		t.Errorf("WaitForDecision() = %v, want InterruptContinue", behavior)
	}
}

func TestInterruptHandler_SimulateInterrupt(t *testing.T) {
	ctx := context.Background()
	h, derivedCtx := NewInterruptHandler(ctx)
	defer h.Stop()

	// Simulate first interrupt by directly setting state
	h.mu.Lock()
	h.interrupted = true
	h.firstInterrupt = time.Now()
	h.cancelFunc()
	h.mu.Unlock()

	if !h.WasInterrupted() {
		t.Error("WasInterrupted() should be true after interrupt")
	}

	// Derived context should be canceled
	select {
	case <-derivedCtx.Done():
		// Expected
	default:
		t.Error("derived context should be canceled after interrupt")
	}
}

func TestInterruptHandler_WaitForDecision_Continue(t *testing.T) {
	ctx := context.Background()
	h, _ := NewInterruptHandler(ctx)
	defer h.Stop()

	// Simulate first interrupt
	h.mu.Lock()
	h.interrupted = true
	h.firstInterrupt = time.Now()
	h.mu.Unlock()

	// Should wait and return Continue since no second interrupt
	start := time.Now()
	behavior := h.WaitForDecision("test")
	elapsed := time.Since(start)

	if behavior != InterruptContinue {
		t.Errorf("WaitForDecision() = %v, want InterruptContinue", behavior)
	}

	// Should have waited approximately interruptWindow
	if elapsed < interruptWindow-100*time.Millisecond {
		t.Errorf("WaitForDecision() returned too fast: %v", elapsed)
	}
}

func TestInterruptHandler_WaitForDecision_Abort(t *testing.T) {
	ctx := context.Background()
	h, _ := NewInterruptHandler(ctx)
	defer h.Stop()

	// Simulate first interrupt
	h.mu.Lock()
	h.interrupted = true
	h.firstInterrupt = time.Now()
	h.mu.Unlock()

	// Simulate second interrupt after a short delay
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(200 * time.Millisecond)
		h.mu.Lock()
		h.aborted = true
		h.mu.Unlock()
	}()

	start := time.Now()
	behavior := h.WaitForDecision("test")
	elapsed := time.Since(start)

	wg.Wait()

	if behavior != InterruptAbort {
		t.Errorf("WaitForDecision() = %v, want InterruptAbort", behavior)
	}

	// Should have returned quickly after the simulated second interrupt
	if elapsed > 500*time.Millisecond {
		t.Errorf("WaitForDecision() took too long: %v", elapsed)
	}
}

func TestInterruptHandler_WaitForDecision_ExpiredWindow(t *testing.T) {
	ctx := context.Background()
	h, _ := NewInterruptHandler(ctx)
	defer h.Stop()

	// Simulate interrupt that happened long ago (window expired)
	h.mu.Lock()
	h.interrupted = true
	h.firstInterrupt = time.Now().Add(-5 * time.Second) // 5s ago
	h.mu.Unlock()

	// Should return immediately since window expired
	start := time.Now()
	behavior := h.WaitForDecision("test")
	elapsed := time.Since(start)

	if behavior != InterruptContinue {
		t.Errorf("WaitForDecision() = %v, want InterruptContinue", behavior)
	}

	if elapsed > 100*time.Millisecond {
		t.Errorf("WaitForDecision() should return immediately for expired window: %v", elapsed)
	}
}

func TestInterruptBehavior_Constants(t *testing.T) {
	// Verify constants have distinct values
	if InterruptContinue == InterruptAbort {
		t.Error("InterruptContinue and InterruptAbort should be different")
	}
}

func TestInterruptWindow_Value(t *testing.T) {
	// Verify the window is 2 seconds as documented
	if interruptWindow != 2*time.Second {
		t.Errorf("interruptWindow = %v, want 2s", interruptWindow)
	}
}
