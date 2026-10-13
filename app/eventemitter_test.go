package app

import (
	"context"
	"sync"
	"testing"
)

func TestOnDispatch(t *testing.T) {
	er := newEventRegistry()
	var called bool
	er.On("test.event", func(payload any) error {
		called = true
		return nil
	})
	er.Dispatch("test.event", nil)
	if !called {
		t.Error("listener was not called")
	}
}

func TestOnDispatchWithPayload(t *testing.T) {
	er := newEventRegistry()
	var result string
	er.On("data", func(payload any) error {
		result = payload.(string)
		return nil
	})
	er.Dispatch("data", "hello")
	if result != "hello" {
		t.Errorf("expected 'hello', got %s", result)
	}
}

func TestMultipleListeners(t *testing.T) {
	er := newEventRegistry()
	count := 0
	er.On("event", func(payload any) error { count++; return nil })
	er.On("event", func(payload any) error { count++; return nil })
	er.Dispatch("event", nil)
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestRemoveEvent(t *testing.T) {
	er := newEventRegistry()
	er.On("removed", func(payload any) error { t.Error("should not be called"); return nil })
	er.Remove("removed")
	er.Dispatch("removed", nil)
}

func TestClearEvents(t *testing.T) {
	er := newEventRegistry()
	er.On("a", func(payload any) error { t.Error("should not be called"); return nil })
	er.On("b", func(payload any) error { t.Error("should not be called"); return nil })
	er.Clear()
	if er.Count() != 0 {
		t.Errorf("expected 0, got %d", er.Count())
	}
}

func TestHasEvent(t *testing.T) {
	er := newEventRegistry()
	if er.Has("missing") {
		t.Error("Has should return false for missing event")
	}
	er.On("exists", func(payload any) error { return nil })
	if !er.Has("exists") {
		t.Error("Has should return true for registered event")
	}
}

func TestCountEvents(t *testing.T) {
	er := newEventRegistry()
	if er.Count() != 0 {
		t.Errorf("expected 0, got %d", er.Count())
	}
	er.On("e1", func(payload any) error { return nil })
	er.On("e2", func(payload any) error { return nil })
	if er.Count() != 2 {
		t.Errorf("expected 2, got %d", er.Count())
	}
}

func TestShutdownBlocksDispatch(t *testing.T) {
	er := newEventRegistry()
	var called bool
	er.On("event", func(payload any) error {
		called = true
		return nil
	})
	er.Shutdown(context.Background())
	er.Dispatch("event", nil)
	if called {
		t.Error("listener called after shutdown")
	}
}

func TestDispatchNoListeners(t *testing.T) {
	er := newEventRegistry()
	er.Dispatch("nonexistent", nil)
}

func TestAllEvents(t *testing.T) {
	er := newEventRegistry()
	er.On("e1", func(payload any) error { return nil })
	er.On("e2", func(payload any) error { return nil })
	all := er.All()
	if len(all) != 2 {
		t.Errorf("expected 2, got %d", len(all))
	}
}

func TestListenerErrorLogged(t *testing.T) {
	er := newEventRegistry()
	er.On("err", func(payload any) error { return nil })
	er.Dispatch("err", nil)
}

func TestShutdownIdempotent(t *testing.T) {
	er := newEventRegistry()
	er.Shutdown(context.Background())
	er.Shutdown(context.Background())
}

func TestOnAfterShutdownPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when registering after shutdown")
		}
	}()
	er := newEventRegistry()
	er.Shutdown(context.Background())
	er.On("late", func(payload any) error { return nil })
}

func TestConcurrentDispatchAndRegistration(t *testing.T) {
	er := newEventRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				er.On("event", func(any) error { return nil })
				er.Dispatch("event", nil)
			}
		}()
	}
	wg.Wait()
}

// The tests above drive eventRegistry directly, which is how the bug below
// survived: the only broken code was the variadic wrapper on application, and
// nothing crossed it. These do.

func TestDispatchDeliversOnePayloadAsItself(t *testing.T) {
	a := Configure().(*application)

	type hit struct{ Key string }
	var got any
	a.On("cache.hit", func(payload any) error {
		got = payload
		return nil
	})
	a.Dispatch("cache.hit", hit{Key: "k"})

	// The assertion a listener would actually write. Before the fix this
	// received []any{hit{...}} and failed, which is what every documented
	// cache event listener would have done.
	event, ok := got.(hit)
	if !ok {
		t.Fatalf("listener received %T, want app.hit — a single payload must "+
			"arrive as itself, not wrapped in a slice", got)
	}
	if event.Key != "k" {
		t.Fatalf("got key %q, want k", event.Key)
	}
}

func TestDispatchDeliversSeveralPayloadsAsASlice(t *testing.T) {
	a := Configure().(*application)

	var got any
	a.On("many", func(payload any) error {
		got = payload
		return nil
	})
	a.Dispatch("many", 1, 2)

	values, ok := got.([]any)
	if !ok {
		t.Fatalf("listener received %T, want []any", got)
	}
	if len(values) != 2 {
		t.Fatalf("got %d payloads, want 2", len(values))
	}
}

func TestDispatchWithNoPayloadDeliversNil(t *testing.T) {
	a := Configure().(*application)

	called := false
	a.On("bare", func(payload any) error {
		called = true
		if payload != nil {
			t.Errorf("got payload %#v, want nil — an event with nothing to "+
				"carry must not arrive carrying an empty slice", payload)
		}
		return nil
	})
	a.Dispatch("bare")

	if !called {
		t.Fatal("listener was not called")
	}
}
