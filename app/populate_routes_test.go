package app

import "testing"

// The routing table was only ever built on the way to serving HTTP:
// registerRoutes runs from serveHandler, so a console command saw an empty
// router however many routes the application serves. Anything wanting to read
// the table had no way to get it.
func TestPopulateRoutesBuildsTheTableWithoutServing(t *testing.T) {
	a := Configure(WithRoutes([]RouteCallback{
		func(a App) {
			a.Router().Get("/one", noopHandler)
			a.Router().Post("/two", noopHandler)
		},
	}))

	// Before: nothing, because the callbacks have not run.
	if before := a.Router().Routes(); len(before) != 0 {
		t.Fatalf("Routes() = %d before PopulateRoutes, want 0", len(before))
	}

	a.PopulateRoutes()

	after := a.Router().Routes()
	if len(after) != 2 {
		t.Fatalf("Routes() = %d after PopulateRoutes, want 2", len(after))
	}
	if after[0].Path != "/one" || after[1].Path != "/two" {
		t.Errorf("unexpected table: %+v", after)
	}
}

// The callbacks append to the router, so running them twice would double every
// route — and the second pass would also reach the mux, where re-registering a
// pattern panics.
func TestPopulateRoutesIsIdempotent(t *testing.T) {
	calls := 0
	a := Configure(WithRoutes([]RouteCallback{
		func(a App) {
			calls++
			a.Router().Get("/once", noopHandler)
		},
	}))

	a.PopulateRoutes()
	a.PopulateRoutes()
	a.PopulateRoutes()

	if calls != 1 {
		t.Errorf("the route callback ran %d times, want 1", calls)
	}
	if routes := a.Router().Routes(); len(routes) != 1 {
		t.Errorf("Routes() = %d after three calls, want 1: %+v", len(routes), routes)
	}
}

// Populating and then serving must not register everything twice. This is the
// interaction that would panic in the mux, so it is asserted rather than
// reasoned about.
func TestPopulateRoutesThenServingDoesNotDuplicate(t *testing.T) {
	a := Configure(WithRoutes([]RouteCallback{
		func(a App) { a.Router().Get("/shared", noopHandler) },
	}))

	a.PopulateRoutes()
	// serveHandler is what Run and Handler both go through.
	if handler := a.(*application).serveHandler(); handler == nil {
		t.Fatal("serveHandler() returned nil")
	}

	matching := 0
	for _, route := range a.Router().Routes() {
		if route.Path == "/shared" {
			matching++
		}
	}
	if matching != 1 {
		t.Errorf("/shared is registered %d times, want 1", matching)
	}
}
