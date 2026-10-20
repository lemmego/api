package app

import (
	"net/http"
	"testing"
)

func noopHandler(c Context) error { return nil }

// The table is what an agent, a `routes` command or a person needs, and until
// Routes existed the only way to see it was to read the source.
func TestRoutesReportsEveryRegisteredRoute(t *testing.T) {
	r := newRouter()
	r.Get("/", noopHandler)
	r.Post("/login", noopHandler)
	r.Delete("/posts/{id}", noopHandler)

	routes := r.Routes()
	if len(routes) != 3 {
		t.Fatalf("Routes() returned %d routes, want 3", len(routes))
	}

	want := []struct{ method, path string }{
		{http.MethodGet, "/"},
		{http.MethodPost, "/login"},
		{http.MethodDelete, "/posts/{id}"},
	}
	// Registration order, because a table shuffled on every call is useless
	// for diffing one run against another.
	for i, w := range want {
		if routes[i].Method != w.method || routes[i].Path != w.path {
			t.Errorf("route %d = %s %s, want %s %s",
				i, routes[i].Method, routes[i].Path, w.method, w.path)
		}
	}
}

// A group's prefix has to be resolved in the reported path. Reporting the
// route's own fragment would make the table a set of paths the application
// does not serve, which is worse than no table.
func TestRoutesResolvesGroupPrefixes(t *testing.T) {
	r := newRouter()
	api := r.Group("/api")
	api.Get("/ping", noopHandler)

	routes := r.Routes()
	if len(routes) != 1 {
		t.Fatalf("Routes() returned %d routes, want 1", len(routes))
	}
	if routes[0].Path != "/api/ping" {
		t.Errorf("grouped route path = %q, want %q", routes[0].Path, "/api/ping")
	}
}

// Counts, not the function values: a handler tells a reader nothing, and the
// count is what distinguishes a bare route from one behind a guard.
func TestRoutesCountsHandlersAndMiddleware(t *testing.T) {
	r := newRouter()
	r.Get("/guarded", noopHandler, noopHandler)

	routes := r.Routes()
	if len(routes) != 1 {
		t.Fatalf("Routes() returned %d routes, want 1", len(routes))
	}
	if routes[0].Handlers != 2 {
		t.Errorf("Handlers = %d, want 2", routes[0].Handlers)
	}
}

// An empty table must not be read as "nothing is served": Handle and
// HandleFunc go straight to the ServeMux, so this router never sees them. The
// doc comment says so, and this records it.
func TestRoutesOmitsRawMuxHandlers(t *testing.T) {
	r := newRouter()
	r.HandleFunc("/raw", func(http.ResponseWriter, *http.Request) {})

	if routes := r.Routes(); len(routes) != 0 {
		t.Errorf("Routes() reported %d routes for a raw mux handler, want 0", len(routes))
	}
}

// Callers must not be able to rewrite the routing table by editing what they
// enumerated.
func TestRoutesReturnsCopies(t *testing.T) {
	r := newRouter()
	r.Get("/original", noopHandler)

	routes := r.Routes()
	routes[0].Path = "/tampered"

	if again := r.Routes(); again[0].Path != "/original" {
		t.Errorf("mutating the result changed the router: path = %q", again[0].Path)
	}
}
