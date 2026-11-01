package app

import (
	"context"
	"sync"
	"testing"
)

func TestOnDispatch(t *testing.T) {
	er := newEventRegistry()
	var called bool
	er.On("test.event", func(_ context.Context, payload any) error {
		called = true
		return nil
	})
	er.Dispatch(context.Background(), "test.event", nil)
	if !called {
		t.Error("listener was not called")
	}
}

func TestOnDispatchWithPayload(t *testing.T) {
	er := newEventRegistry()
	var result string
	er.On("data", func(_ context.Context, payload any) error {
		result = payload.(string)
		return nil
	})
	er.Dispatch(context.Background(), "data", "hello")
	if result != "hello" {
		t.Errorf("expected 'hello', got %s", result)
	}
}

func TestMultipleListeners(t *testing.T) {
	er := newEventRegistry()
	count := 0
	er.On("event", func(_ context.Context, payload any) error { count++; return nil })
	er.On("event", func(_ context.Context, payload any) error { count++; return nil })
	er.Dispatch(context.Background(), "event", nil)
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestRemoveEvent(t *testing.T) {
	er := newEventRegistry()
	er.On("removed", func(_ context.Context, payload any) error { t.Error("should not be called"); return nil })
	er.Remove("removed")
	er.Dispatch(context.Background(), "removed", nil)
}

func TestClearEvents(t *testing.T) {
	er := newEventRegistry()
	er.On("a", func(_ context.Context, payload any) error { t.Error("should not be called"); return nil })
	er.On("b", func(_ context.Context, payload any) error { t.Error("should not be called"); return nil })
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
	er.On("exists", func(_ context.Context, payload any) error { return nil })
	if !er.Has("exists") {
		t.Error("Has should return true for registered event")
	}
}

func TestCountEvents(t *testing.T) {
	er := newEventRegistry()
	if er.Count() != 0 {
		t.Errorf("expected 0, got %d", er.Count())
	}
	er.On("e1", func(_ context.Context, payload any) error { return nil })
	er.On("e2", func(_ context.Context, payload any) error { return nil })
	if er.Count() != 2 {
		t.Errorf("expected 2, got %d", er.Count())
	}
}

func TestShutdownBlocksDispatch(t *testing.T) {
	er := newEventRegistry()
	var called bool
	er.On("event", func(_ context.Context, payload any) error {
		called = true
		return nil
	})
	er.Shutdown(context.Background())
	er.Dispatch(context.Background(), "event", nil)
	if called {
		t.Error("listener called after shutdown")
	}
}

func TestDispatchNoListeners(t *testing.T) {
	er := newEventRegistry()
	er.Dispatch(context.Background(), "nonexistent", nil)
}

func TestAllEvents(t *testing.T) {
	er := newEventRegistry()
	er.On("e1", func(_ context.Context, payload any) error { return nil })
	er.On("e2", func(_ context.Context, payload any) error { return nil })
	all := er.All()
	if len(all) != 2 {
		t.Errorf("expected 2, got %d", len(all))
	}
}

func TestListenerErrorLogged(t *testing.T) {
	er := newEventRegistry()
	er.On("err", func(_ context.Context, payload any) error { return nil })
	er.Dispatch(context.Background(), "err", nil)
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
	er.On("late", func(_ context.Context, payload any) error { return nil })
}

func TestConcurrentDispatchAndRegistration(t *testing.T) {
	er := newEventRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				er.On("event", func(context.Context, any) error { return nil })
				er.Dispatch(context.Background(), "event", nil)
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
	a.On("cache.hit", func(_ context.Context, payload any) error {
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
	a.On("many", func(_ context.Context, payload any) error {
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
	a.On("bare", func(_ context.Context, payload any) error {
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

// TestAListenerIsHandedTheDispatchersContext.
//
// The emitter has never had a consumer, which is why it never had a context:
// an event that cannot be attributed to a request is a line in a log, and the
// first thing anything watching the emitter wants to ask is which request
// caused it.
func TestAListenerIsHandedTheDispatchersContext(t *testing.T) {
	type key struct{}

	er := newEventRegistry()
	var got context.Context
	er.On("test.event", func(ctx context.Context, _ any) error {
		got = ctx
		return nil
	})

	er.Dispatch(context.WithValue(context.Background(), key{}, "this request"), "test.event", nil)

	if got == nil {
		t.Fatal("the listener was not called")
	}
	if got.Value(key{}) != "this request" {
		t.Errorf("the listener was handed a context carrying %v, not the dispatcher's", got.Value(key{}))
	}
}

// A nil context is what a caller reaches for when it has none, and a nil
// context reaches slog, a store and anything else the listener does.
func TestANilContextBecomesBackground(t *testing.T) {
	er := newEventRegistry()
	var got context.Context
	er.On("test.event", func(ctx context.Context, _ any) error {
		got = ctx
		return nil
	})

	//nolint:staticcheck // passing nil is exactly what is under test.
	er.Dispatch(nil, "test.event", nil)

	if got == nil {
		t.Fatal("the listener was handed a nil context")
	}
}

// TestAPanickingListenerDoesNotTakeDownTheDispatcher.
//
// Without this, a listener registered by a plugin takes down whatever
// dispatched the event — for ServerStopping that is the shutdown itself, and
// for anything dispatched during a request it is the request. Every other
// listener seam in this framework recovers; this one did not, and it is the
// one a third party is most likely to register against.
func TestAPanickingListenerDoesNotTakeDownTheDispatcher(t *testing.T) {
	er := newEventRegistry()
	second := false
	er.On("test.event", func(context.Context, any) error { panic("the listener is broken") })
	er.On("test.event", func(context.Context, any) error { second = true; return nil })

	er.Dispatch(context.Background(), "test.event", nil)

	if !second {
		t.Error("a panic in the first listener cost the second its event")
	}
}

// DispatchContext is the application-level entry point, and the payload rule
// is asserted through it rather than through the registry so the unwrapping
// in application.DispatchContext is covered too.
func TestDispatchContextCarriesTheContextAndASinglePayload(t *testing.T) {
	type key struct{}

	engine := Configure()
	var (
		got     context.Context
		payload any
	)
	engine.On("test.event", func(ctx context.Context, p any) error {
		got, payload = ctx, p
		return nil
	})

	engine.DispatchContext(
		context.WithValue(context.Background(), key{}, "this request"),
		"test.event", "one value")

	if got == nil {
		t.Fatal("the listener was not called")
	}
	if got.Value(key{}) != "this request" {
		t.Errorf("context carried %v", got.Value(key{}))
	}
	// A single payload arrives as itself, not as a one-element slice.
	if payload != "one value" {
		t.Errorf("payload = %#v, want the value itself", payload)
	}
}
