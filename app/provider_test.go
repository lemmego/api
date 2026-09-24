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
