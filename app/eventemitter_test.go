package app

import (
	"context"
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
