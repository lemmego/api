package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// namedRenderer implements ViewNamer, as res.Template and inertia's response
// do.
type namedRenderer struct {
	name string
	err  error
}

func (r namedRenderer) ViewName() string { return r.name }

func (r namedRenderer) Render(w io.Writer) error {
	_, _ = io.WriteString(w, "rendered")
	return r.err
}

// anonymousRenderer does not, which is the fallback case.
type anonymousRenderer struct{}

func (anonymousRenderer) Render(w io.Writer) error {
	_, _ = io.WriteString(w, "rendered")
	return nil
}

type templComponent struct{ err error }

func (c templComponent) Render(context.Context, io.Writer) error { return c.err }

// renderCtx builds a context over a recorder, with a value on the request so
// the correlation assertion has something to find.
func renderCtx(t *testing.T, requestKey, requestValue any) (*ctx, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if requestKey != nil {
		request = request.WithContext(context.WithValue(request.Context(), requestKey, requestValue))
	}
	return &ctx{request: request, writer: recorder}, recorder
}

func watchRenders(t *testing.T) *[]Rendered {
	t.Helper()
	var seen []Rendered
	ObserveRenders(func(_ context.Context, r Rendered) { seen = append(seen, r) })
	t.Cleanup(func() { SetRenderObservers(nil) })
	return &seen
}

func TestARenderIsObservedByItsViewName(t *testing.T) {
	seen := watchRenders(t)
	c, _ := renderCtx(t, nil, nil)
	c.status = http.StatusOK

	if err := c.Render(namedRenderer{name: "Profile/Show"}); err != nil {
		t.Fatal(err)
	}

	if len(*seen) != 1 {
		t.Fatalf("observed %d renders, want 1", len(*seen))
	}
	got := (*seen)[0]
	if got.View != "Profile/Show" {
		t.Errorf("View = %q, want the name the renderer gave", got.View)
	}
	if got.Status != http.StatusOK {
		t.Errorf("Status = %d", got.Status)
	}
	if got.Err != nil {
		t.Errorf("Err = %v", got.Err)
	}
}

// A renderer that will not name itself is reported by its type. Honest, and
// much less useful — which is the argument for ViewNamer existing rather than
// for reflection being enough.
func TestARendererThatWillNotNameItselfIsReportedByItsType(t *testing.T) {
	seen := watchRenders(t)
	c, _ := renderCtx(t, nil, nil)

	if err := c.Render(anonymousRenderer{}); err != nil {
		t.Fatal(err)
	}

	if len(*seen) != 1 {
		t.Fatalf("observed %d renders, want 1", len(*seen))
	}
	if (*seen)[0].View != "app.anonymousRenderer" {
		t.Errorf("View = %q, want the type", (*seen)[0].View)
	}
}

// Context.Templ is a second view path: it takes a component rather than a
// Renderer and never reaches Render. A view panel built on Render alone would
// be silently blind to every page an application renders this way, which is
// the same gap shape the tasker dispatch hook had to avoid.
func TestTemplIsObservedToo(t *testing.T) {
	seen := watchRenders(t)
	c, _ := renderCtx(t, nil, nil)

	if err := c.Templ(templComponent{}); err != nil {
		t.Fatal(err)
	}

	if len(*seen) != 1 {
		t.Fatalf("observed %d renders, want 1 — Templ renders a view too", len(*seen))
	}
	got := (*seen)[0]
	if got.View != "app.templComponent" {
		t.Errorf("View = %q", got.View)
	}
	if got.Renderer != nil {
		t.Error("Templ has no Renderer, so Renderer must be nil rather than something invented")
	}
}

// A render that failed is the one most worth seeing, and by the time it is
// reported a partial body has already been written — which is the shape of
// failure that reaches a browser as a truncated page rather than an error.
func TestAFailedRenderIsObservedWithItsError(t *testing.T) {
	seen := watchRenders(t)
	c, recorder := renderCtx(t, nil, nil)

	broken := errors.New("the template does not exist")
	if err := c.Render(namedRenderer{name: "Profile/Show", err: broken}); !errors.Is(err, broken) {
		t.Fatalf("Render returned %v; the observer swallowed the error", err)
	}

	if len(*seen) != 1 {
		t.Fatalf("observed %d renders, want 1", len(*seen))
	}
	if !errors.Is((*seen)[0].Err, broken) {
		t.Errorf("Err = %v, want the renderer's", (*seen)[0].Err)
	}
	if recorder.Body.Len() == 0 {
		t.Error("nothing was written, so this is not the partial-body case it claims to be")
	}
}

func TestTheObserverIsHandedTheRequestsContext(t *testing.T) {
	type key struct{}

	var got context.Context
	ObserveRenders(func(ctx context.Context, _ Rendered) { got = ctx })
	t.Cleanup(func() { SetRenderObservers(nil) })

	c, _ := renderCtx(t, key{}, "this request")
	if err := c.Render(namedRenderer{name: "Profile/Show"}); err != nil {
		t.Fatal(err)
	}

	if got == nil {
		t.Fatal("the observer was not called")
	}
	if got.Value(key{}) != "this request" {
		t.Errorf("the observer was handed a context carrying %v, not the request's", got.Value(key{}))
	}
}

// Rendering a page must not be breakable by something watching it.
func TestAPanickingRenderObserverDoesNotBreakThePage(t *testing.T) {
	ran := false
	ObserveRenders(func(context.Context, Rendered) { panic("the observer is broken") })
	ObserveRenders(func(context.Context, Rendered) { ran = true })
	t.Cleanup(func() { SetRenderObservers(nil) })

	c, recorder := renderCtx(t, nil, nil)
	if err := c.Render(namedRenderer{name: "Profile/Show"}); err != nil {
		t.Fatalf("a panic in an observer reached the caller: %v", err)
	}

	if recorder.Body.String() != "rendered" {
		t.Errorf("body = %q", recorder.Body.String())
	}
	if !ran {
		t.Error("a panic in the first observer cost the second its render")
	}
}

// Nobody watching is the normal case, and the guard on the render path is one
// atomic load rather than a lock.
func TestARenderWithNoObserversIsUnchanged(t *testing.T) {
	SetRenderObservers(nil)
	c, recorder := renderCtx(t, nil, nil)

	if err := c.Render(namedRenderer{name: "Profile/Show"}); err != nil {
		t.Fatal(err)
	}
	if recorder.Body.String() != "rendered" {
		t.Errorf("body = %q", recorder.Body.String())
	}
}
