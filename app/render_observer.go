package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// ViewNamer lets a Renderer say what it rendered.
//
// Optional: a renderer that does not implement it is reported by its type,
// which is honest but much less useful — "*inertia.InertiaResponse" on every
// page is a count, not a view panel. The ones that can name themselves do.
type ViewNamer interface {
	ViewName() string
}

// Rendered reports one view render.
type Rendered struct {
	// View is what was rendered: a component path, a template file, or the
	// renderer's type when it will not say which.
	View string

	// Renderer is the value that was rendered, for an observer that knows
	// more about a particular one than this package does. It is nil for a
	// render that had no Renderer — Context.Templ takes a component
	// directly.
	Renderer Renderer

	Status   int
	Duration time.Duration

	// Err is the renderer's error. A render that failed halfway has already
	// written a partial body by the time it is reported, which is worth
	// knowing: it is the shape of failure that reaches a browser as a page
	// that looks truncated rather than as an error.
	Err error
}

// RenderObserver is told about every view render.
//
// It runs synchronously on the request's goroutine after the body has been
// written, is handed the request's context so an entry can be attributed, and
// a panic in it is contained — rendering a page must not be breakable by
// something watching it.
type RenderObserver func(ctx context.Context, r Rendered)

var (
	renderObserversMu sync.Mutex
	renderObservers   atomic.Pointer[[]RenderObserver]
)

// ObserveRenders registers an observer for every view render. Additive.
//
// Process-wide rather than per-application, matching tasker's middleware
// registries: a process runs one application, and a watcher installed at boot
// has no application to reach through.
//
// The read on the render path is one atomic load, so a process with no
// observer pays nothing it can measure.
func ObserveRenders(o RenderObserver) {
	if o == nil {
		return
	}
	renderObserversMu.Lock()
	defer renderObserversMu.Unlock()

	var next []RenderObserver
	if current := renderObservers.Load(); current != nil {
		next = append(next, *current...)
	}
	next = append(next, o)
	renderObservers.Store(&next)
}

// SetRenderObservers replaces the registered set. Passing nil clears it,
// which is how a test stops leaking into the next one.
func SetRenderObservers(observers []RenderObserver) {
	renderObserversMu.Lock()
	defer renderObserversMu.Unlock()
	renderObservers.Store(&observers)
}

// renderObserved reports a render, and is the one place that decides what a
// view is called.
func renderObserved(ctx context.Context, r Renderer, component any, status int, took time.Duration, err error) {
	observers := renderObservers.Load()
	if observers == nil || len(*observers) == 0 {
		return
	}

	view := ""
	switch {
	case r != nil:
		if named, ok := r.(ViewNamer); ok {
			view = named.ViewName()
		}
		if view == "" {
			view = fmt.Sprintf("%T", r)
		}
	default:
		view = fmt.Sprintf("%T", component)
	}

	rendered := Rendered{
		View:     view,
		Renderer: r,
		Status:   status,
		Duration: took,
		Err:      err,
	}
	for _, observe := range *observers {
		deliverRender(ctx, observe, rendered)
	}
}

// deliverRender isolates an observer's panic. Rendering a page must not be
// breakable by something watching it — the same rule every other listener
// seam in this framework follows.
func deliverRender(ctx context.Context, observe RenderObserver, r Rendered) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.ErrorContext(ctx, "a render observer panicked",
				"view", r.View, "panic", recovered)
		}
	}()
	observe(ctx, r)
}
