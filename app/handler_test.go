package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/lemmego/api/config"
)

// Handler is the seam a test harness boots through. Run does the same work
// and then binds a port, so what Handler returns has to be the stack that
// ships — providers registered, middleware installed, routes mounted, the
// session wrapper outermost. These tests pin each of those, because a harness
// that boots a subtly different application proves nothing about production.
func testConfig() config.M {
	return config.M{
		"app": config.M{"name": "handlertest", "port": 0, "env": "test"},
	}
}

func TestHandlerServesRegisteredRoutes(t *testing.T) {
	engine := Configure().WithConfig(testConfig()).WithRoutes([]RouteCallback{
		func(a App) {
			a.Router().Get("/ping", func(c Context) error {
				return c.JSON(M{"message": "pong"})
			})
		},
	})

	recorder := httptest.NewRecorder()
	engine.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ping", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if got := recorder.Body.String(); got == "" {
		t.Error("no body")
	}
}

// A provider's Provide must have run before the first request, or a handler
// resolving a service gets nil.
func TestHandlerRegistersProviders(t *testing.T) {
	provided := false
	engine := Configure().WithConfig(testConfig()).
		WithProviders([]Provider{&probeProvider{onProvide: func() { provided = true }}})

	engine.Handler()

	if !provided {
		t.Fatal("Handler returned without registering providers")
	}
}

// Application middleware has to be in the chain. Run installs it; a harness
// that skipped it would let an unauthenticated request through a test that
// was meant to prove the opposite.
func TestHandlerInstallsMiddleware(t *testing.T) {
	var ran bool
	engine := Configure().WithConfig(testConfig()).
		WithMiddlewares([]Handler{func(c Context) error {
			ran = true
			return c.Next()
		}}).
		WithRoutes([]RouteCallback{func(a App) {
			a.Router().Get("/guarded", func(c Context) error { return c.JSON(M{"ok": true}) })
		}})

	recorder := httptest.NewRecorder()
	engine.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/guarded", nil))

	if !ran {
		t.Fatalf("middleware did not run (status %d)", recorder.Code)
	}
}

func TestHandlerInstallsHTTPMiddleware(t *testing.T) {
	engine := Configure().WithConfig(testConfig()).
		WithHTTPMiddlewares([]HTTPMiddleware{func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Probe", "seen")
				next.ServeHTTP(w, r)
			})
		}}).
		WithRoutes([]RouteCallback{func(a App) {
			a.Router().Get("/x", func(c Context) error { return c.JSON(M{"ok": true}) })
		}})

	recorder := httptest.NewRecorder()
	engine.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/x", nil))

	if recorder.Header().Get("X-Probe") != "seen" {
		t.Error("HTTP middleware did not wrap the handler")
	}
}

// An unmatched path must reach the catch-all rather than the mux's bare 404,
// so error pages and the errMap behave in a test as they do in production.
func TestHandlerServesTheCatchAll(t *testing.T) {
	engine := Configure().WithConfig(testConfig())

	recorder := httptest.NewRecorder()
	engine.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/nothing-here", nil))

	if recorder.Code == 0 {
		t.Fatal("catch-all did not respond")
	}
	if recorder.Code == http.StatusOK {
		t.Errorf("unmatched path returned 200")
	}
}

// Handler must not publish. publish writes files into the project directory,
// and booting an application in a test must not have that side effect.
func TestHandlerDoesNotPublish(t *testing.T) {
	engine := Configure().WithConfig(testConfig()).
		WithProviders([]Provider{&probeProvider{
			publishables: []*Publishable{{
				FilePath: "handler_test_should_not_exist.go",
				Content:  []byte("package nope\n"),
				Tag:      "apptest-probe",
			}},
		}})

	engine.Handler()

	if _, err := os.Stat("handler_test_should_not_exist.go"); err == nil {
		t.Fatal("Handler published a file; booting in a test must not write to the project")
	}
}

type probeProvider struct {
	onProvide    func()
	publishables []*Publishable
}

func (p *probeProvider) Provide(a App) error {
	if p.onProvide != nil {
		p.onProvide()
	}
	return nil
}

func (p *probeProvider) AddPublishables() []*Publishable { return p.publishables }

// An application with no session.Provider is a legitimate configuration: a
// pure JSON API has no cookie jar. Both the boot path and the per-request
// path resolved the session with Get, which panics on an unregistered
// service — so such an application took the process down at boot, and then
// on every single request. Neither is recoverable by a middleware.
func TestHandlerServesAnApplicationWithoutASession(t *testing.T) {
	engine := Configure().WithConfig(testConfig()).WithRoutes([]RouteCallback{
		func(a App) {
			a.Router().Get("/api/ping", func(c Context) error {
				return c.JSON(M{"message": "pong"})
			})
		},
	})

	// No session.Provider, and no panic.
	handler := engine.Handler()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/ping", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
}
