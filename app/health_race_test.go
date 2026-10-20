package app

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Check held h.mu.RLock and then called defaultTimeout, which took the same
// RLock again. Recursive read locking is prohibited: once a writer is waiting
// on Lock, sync.RWMutex blocks new readers — so a RegisterCheck arriving
// between the two RLocks deadlocked every goroutine involved, permanently.
// Nothing timed out and nothing logged; the process stopped answering.
//
// Reachable in any application serving /health while something registers a
// check, which is ordinary during boot. Found by two MCP tool calls arriving
// together, because the SDK handles each in its own goroutine.
//
// The assertion is completion, not a value: against the old code this test
// hangs until the package timeout rather than failing, so the signal is that
// it returns at all.
func TestCheckAndRegisterDoNotDeadlock(t *testing.T) {
	checker := NewHealthChecker(Configure())
	checker.RegisterCheck("seed", HealthCheck{
		Name:      "seed",
		CheckFunc: func(context.Context) error { return nil },
		Timeout:   time.Second,
	})

	// Caching would let a later Check return without taking the lock at all,
	// which would hide the very interleaving this is testing.
	checker.SetCacheDuration(0)

	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				checker.Check(context.Background())
			}()
			go func(n int) {
				defer wg.Done()
				checker.RegisterCheck("late", HealthCheck{
					Name:      "late",
					CheckFunc: func(context.Context) error { return nil },
					Timeout:   time.Second,
				})
			}(i)
		}
		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("Check and RegisterCheck deadlocked: recursive RLock against a waiting writer")
	}
}

// defaultTimeout is still correct for callers that do not hold the lock, which
// is the half that had to keep working.
func TestDefaultTimeoutStillLocksForOutsideCallers(t *testing.T) {
	checker := NewHealthChecker(Configure())

	if got := checker.defaultTimeout(); got != 10*time.Second {
		t.Errorf("defaultTimeout() = %v with no timeout set, want the 10s fallback", got)
	}
	checker.SetTimeout(3 * time.Second)
	if got := checker.defaultTimeout(); got != 3*time.Second {
		t.Errorf("defaultTimeout() = %v, want 3s", got)
	}
}
