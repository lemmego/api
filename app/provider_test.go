package app

import "testing"

func TestLookupMissingServiceDoesNotPanic(t *testing.T) {
	a := Configure()

	if value, ok := Lookup[string](a); ok || value != "" {
		t.Fatalf("Lookup missing service = %q, %v", value, ok)
	}
}

func TestLookupRegisteredService(t *testing.T) {
	a := Configure()
	a.AddService("registered")

	value, ok := Lookup[string](a)
	if !ok || value != "registered" {
		t.Fatalf("Lookup registered service = %q, %v", value, ok)
	}
}

// Lookup used to resolve through App.Service, which keys on the dynamic type
// of a value. For an interface T that value is a nil interface carrying no
// type, so every interface lookup missed no matter what was registered. These
// two tests pin both halves: the interface case that was broken, and the
// concrete case that had to keep working.

func TestLookupResolvesInterfaceKey(t *testing.T) {
	a := Configure()
	RegisterIfAbsent[testServiceContract](a, &testServiceImplementation{value: "seam"})

	service, ok := Lookup[testServiceContract](a)
	if !ok {
		t.Fatal("Lookup did not find a service registered under an interface key")
	}
	if service.Value() != "seam" {
		t.Fatalf("Lookup returned %q, want %q", service.Value(), "seam")
	}
	if got := Get[testServiceContract](a); got.Value() != "seam" {
		t.Fatalf("Get returned %q, want %q", got.Value(), "seam")
	}
}

func TestLookupStillResolvesConcreteKey(t *testing.T) {
	a := Configure()
	a.AddService(&testSvc{val: "concrete"})

	service, ok := Lookup[*testSvc](a)
	if !ok || service.val != "concrete" {
		t.Fatalf("Lookup of a concrete type = %v, %v", service, ok)
	}
}

func TestLookupReportsAbsence(t *testing.T) {
	if _, ok := Lookup[testServiceContract](Configure()); ok {
		t.Fatal("Lookup should report false when nothing is registered")
	}
}
