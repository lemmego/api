package app

import (
	"reflect"
	"testing"
)

type testSvc struct{ val string }

func (t *testSvc) Provide(a App) error { return nil }

func TestRegisterAndGet(t *testing.T) {
	sr := newServiceRegistry()
	sr.Register(&testSvc{val: "hello"})
	val, ok := sr.Get(&testSvc{})
	if !ok {
		t.Error("Get returned false for registered service")
	}
	if v := val.(*testSvc); v.val != "hello" {
		t.Errorf("expected 'hello', got %s", v.val)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for duplicate registration")
		}
	}()
	sr := newServiceRegistry()
	sr.Register(&testSvc{val: "a"})
	sr.Register(&testSvc{val: "b"})
}

func TestGetByType(t *testing.T) {
	sr := newServiceRegistry()
	s := &testSvc{val: "test"}
	sr.Register(s)

	got, ok := sr.GetByType(reflect.TypeOf(&testSvc{}))
	if !ok {
		t.Error("GetByType returned false")
	}
	ms := got.(*testSvc)
	if ms.val != "test" {
		t.Errorf("expected 'test', got %s", ms.val)
	}
}

func TestGetNotFound(t *testing.T) {
	sr := newServiceRegistry()
	_, ok := sr.Get(&testSvc{})
	if ok {
		t.Error("Get should return false for missing service")
	}
}

func TestRemove(t *testing.T) {
	sr := newServiceRegistry()
	s := &testSvc{val: "remove"}
	sr.Register(s)
	removed := sr.Remove(s)
	if !removed {
		t.Error("Remove should return true")
	}
	_, ok := sr.Get(s)
	if ok {
		t.Error("service still exists after remove")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	sr := newServiceRegistry()
	removed := sr.Remove(&testSvc{})
	if removed {
		t.Error("Remove should return false for missing service")
	}
}

type testSvc2 struct{ val string }
func (t *testSvc2) Provide(a App) error { return nil }

func TestClear(t *testing.T) {
	sr := newServiceRegistry()
	sr.Register(&testSvc{val: "a"})
	sr.Register(&testSvc2{val: "b"})
	sr.Clear()
	if sr.Count() != 0 {
		t.Errorf("expected 0 after clear, got %d", sr.Count())
	}
}

func TestHas(t *testing.T) {
	sr := newServiceRegistry()
	if sr.Has(&testSvc{}) {
		t.Error("Has should return false for missing")
	}
	sr.Register(&testSvc{val: "present"})
	if !sr.Has(&testSvc{}) {
		t.Error("Has should return true for registered type")
	}
}

func TestCount(t *testing.T) {
	sr := newServiceRegistry()
	if sr.Count() != 0 {
		t.Errorf("expected 0, got %d", sr.Count())
	}
	s1 := &testSvc{val: "x"}
	sr.Register(s1)
	if sr.Count() != 1 {
		t.Errorf("expected 1, got %d", sr.Count())
	}
	sr.Remove(s1)
	if sr.Count() != 0 {
		t.Errorf("expected 0 after remove, got %d", sr.Count())
	}
}

func TestAll(t *testing.T) {
	sr := newServiceRegistry()
	sr.Register(&testSvc{val: "a"})
	sr.Register(&testSvc2{val: "b"})
	all := sr.All()
	if len(all) != 2 {
		t.Errorf("expected 2, got %d", len(all))
	}
}

func TestGetTyped(t *testing.T) {
	sr := newServiceRegistry()
	sr.Register(&testSvc{val: "works"})
	svc, ok := GetTyped[*testSvc](sr)
	if !ok {
		t.Error("GetTyped returned false")
	}
	if svc.val != "works" {
		t.Errorf("expected 'works', got %s", svc.val)
	}
}

func TestGetTypedNotFound(t *testing.T) {
	sr := newServiceRegistry()
	_, ok := GetTyped[*testSvc](sr)
	if ok {
		t.Error("GetTyped should return false for unregistered type")
	}
}

