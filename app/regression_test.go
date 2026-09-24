package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/lemmego/api/config"
	"github.com/lemmego/api/session"
)

type regressionProvider struct{}

func (regressionProvider) Provide(App) error { return nil }

func TestConfigureRetainsProvidersAndErrMap(t *testing.T) {
	provider := regressionProvider{}
	errMap := ErrMap{ErrNotFound: func(Context) error { return nil }}

	configured := Configure(WithProviders([]Provider{provider}), WithErrMap(errMap)).(*application)

	if len(configured.providers) != 1 || configured.providers[0] != provider {
		t.Fatalf("expected configured provider to be retained")
	}
	if len(configured.errMap) != 1 || configured.errMap[ErrNotFound] == nil {
		t.Fatalf("expected configured error map to be retained")
	}
}

func TestConfigureUsesIsolatedConfiguration(t *testing.T) {
	first := Configure(WithConfig(config.M{"app": config.M{"name": "first"}}))
	second := Configure(WithConfig(config.M{"app": config.M{"name": "second"}}))

	if first.Config().Get("app.name") != "first" {
		t.Fatalf("first application configuration was overwritten: %v", first.Config().Get("app.name"))
	}
	if second.Config().Get("app.name") != "second" {
		t.Fatalf("second application configuration was overwritten: %v", second.Config().Get("app.name"))
	}
}

func TestSpanSetError(t *testing.T) {
	span := &Span{}
	done := make(chan struct{})

	go func() {
		span.SetError(errors.New("request failed"))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SetError deadlocked")
	}

	if span.Status != SpanStatusError || span.Tags["error"] != "request failed" {
		t.Fatalf("expected error status and tag, got status %q and tags %#v", span.Status, span.Tags)
	}
}

func TestErrorJSONUsesRequestedStatus(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Accept", "application/json")
	writer := httptest.NewRecorder()
	ctx := &ctx{request: request, writer: writer}

	if err := ctx.Error(http.StatusTeapot, errors.New("short and stout")); err != nil {
		t.Fatal(err)
	}
	if writer.Code != http.StatusTeapot {
		t.Fatalf("expected %d, got %d", http.StatusTeapot, writer.Code)
	}
}

func testErrorHandlerApp(t *testing.T, production bool) *application {
	t.Helper()
	if production {
		t.Setenv("APP_ENV", "production")
	} else {
		t.Setenv("APP_ENV", "development")
	}

	a := Configure().(*application)
	manager := scs.New()
	a.AddService(session.New(manager.Store, manager.Cookie))
	return a
}

func serveErrorRoute(a *application, route *route, w http.ResponseWriter, r *http.Request) {
	a.Session().LoadAndSave(http.HandlerFunc(makeHandlerFunc(a, route))).ServeHTTP(w, r)
}

func TestUnhandledErrorHidesDetailsInProductionJSON(t *testing.T) {
	a := testErrorHandlerApp(t, true)
	route := &route{
		Method:   http.MethodGet,
		Path:     "/failure",
		Handlers: []Handler{func(Context) error { return errors.New("database password leaked") }},
		router:   a.router,
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/failure", nil)
	r.Header.Set("Accept", "application/json")
	serveErrorRoute(a, route, w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if got := w.Body.String(); got != `{"message":"Internal Server Error"}` {
		t.Fatalf("unexpected response: %s", got)
	}
}

func TestUnhandledErrorHidesDetailsInProductionHTML(t *testing.T) {
	a := testErrorHandlerApp(t, true)
	route := &route{
		Method:   http.MethodGet,
		Path:     "/failure",
		Handlers: []Handler{func(Context) error { return errors.New("database password leaked") }},
		router:   a.router,
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/failure", nil)
	r.Header.Set("Accept", "text/html")
	serveErrorRoute(a, route, w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if got := w.Body.String(); got != "Internal Server Error" {
		t.Fatalf("unexpected response: %s", got)
	}
}

func TestUnhandledErrorRetainsDetailsInDevelopment(t *testing.T) {
	a := testErrorHandlerApp(t, false)
	route := &route{
		Method:   http.MethodGet,
		Path:     "/failure",
		Handlers: []Handler{func(Context) error { return errors.New("useful development detail") }},
		router:   a.router,
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/failure", nil)
	r.Header.Set("Accept", "application/json")
	serveErrorRoute(a, route, w, r)

	if !strings.Contains(w.Body.String(), "useful development detail") {
		t.Fatalf("development response omitted error detail: %s", w.Body.String())
	}
}

func TestExplicitHttpErrorMessageRemainsPublicInProduction(t *testing.T) {
	a := testErrorHandlerApp(t, true)
	route := &route{
		Method:   http.MethodGet,
		Path:     "/failure",
		Handlers: []Handler{func(Context) error { return &BadRequestError{HttpMessage{http.StatusBadRequest, "email is invalid"}} }},
		router:   a.router,
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/failure", nil)
	r.Header.Set("Accept", "application/json")
	serveErrorRoute(a, route, w, r)

	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "email is invalid") {
		t.Fatalf("explicit public error was not preserved: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDefaultErrorRouteIsRegisteredAndEscapesMessage(t *testing.T) {
	a := testErrorHandlerApp(t, false)
	a.registerRoutes()

	request := httptest.NewRequest(http.MethodGet, "/error", nil)
	w := httptest.NewRecorder()
	a.Session().LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.Session().Put(r.Context(), "error", "<script>alert(1)</script>")
		a.router.ServeHTTP(w, r)
	})).ServeHTTP(w, request)

	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "<script>") {
		t.Fatalf("default error route was not safely rendered: status=%d body=%s", w.Code, w.Body.String())
	}
}
