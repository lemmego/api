package app

import (
	"testing"

	"github.com/a-h/templ"
)

// Templ is implemented on *ctx but was missing from the Context interface,
// so this — which is what the docs site shows on four pages, and what the
// templ presets in `lemmego new` generate — did not compile:
//
//	./main.go:12:11: c.Templ undefined (type app.Context has no field or
//	method Templ)
//
// Handler takes a Context, so the concrete type is unreachable from a
// handler and the method might as well not have existed. This test is a
// compile guard: removing Templ from HttpResponder fails the build here
// rather than in somebody's project.
func TestTemplIsReachableFromTheContextInterface(t *testing.T) {
	var handler Handler = func(c Context) error {
		var component templ.Component
		return c.Templ(component)
	}
	if handler == nil {
		t.Fatal("unreachable; the point of this test is that it compiles")
	}
}
