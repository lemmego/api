package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// These guards existed but could never run: Session() resolves the service and
// panics when it is absent, so the nil check after it was unreachable. Every
// one of these is on the path of an ordinary request — a failed form
// validation, a redirect carrying input, a rendered error page — so an
// application without a session crashed rather than degrading.
func TestSessionAccessorsWithoutASession(t *testing.T) {
	a := Configure()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	c := &ctx{app: a.(*application), request: request, writer: httptest.NewRecorder()}

	tests := map[string]func(){
		"PutSession":       func() { c.PutSession("k", "v") },
		"PopSession":       func() { c.PopSession("k") },
		"PopSessionString": func() { c.PopSessionString("k") },
		"Session":          func() { c.Session("k") },
		"SessionString":    func() { c.SessionString("k") },
	}

	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s panicked without a session: %v", name, r)
				}
			}()
			call()
		})
	}
}
