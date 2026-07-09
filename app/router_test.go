package app

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelWarn)
}

func testRouter() *httpRouter {
	return newRouter()
}

func registerTestRoute(r *httpRouter, rt *route) {
	pattern := rt.Method + " " + rt.Path
	r.mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		allHandlers := append(append([]Handler{}, rt.BeforeMiddleware...), rt.Handlers...)
		allHandlers = append(allHandlers, rt.AfterMiddleware...)
		ctx := &ctx{writer: w, request: req, handlers: allHandlers, index: -1}
		if err := ctx.Next(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

func TestRouterGet(t *testing.T) {
	r := testRouter()
	rt := r.Get("/hello", func(c Context) error { return nil })
	registerTestRoute(r, rt)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/hello", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRouterPost(t *testing.T) {
	r := testRouter()
	rt := r.Post("/data", func(c Context) error { return nil })
	registerTestRoute(r, rt)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/data", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRouterGetRejectsPost(t *testing.T) {
	r := testRouter()
	rt := r.Get("/hello", func(c Context) error { return nil })
	registerTestRoute(r, rt)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/hello", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestRouterGroup(t *testing.T) {
	r := testRouter()
	g := r.Group("/api")
	rt := g.Get("/ping", func(c Context) error { return nil })
	registerTestRoute(r, rt)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/ping", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRouterGroupNested(t *testing.T) {
	r := testRouter()
	g := r.Group("/api").Group("/v1")
	rt := g.Get("/status", func(c Context) error { return nil })
	registerTestRoute(r, rt)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRouterUseBefore(t *testing.T) {
	r := testRouter()
	var order []string

	r.UseBefore(func(c Context) error {
		order = append(order, "before")
		return c.Next()
	})
	rt := r.Get("/test", func(c Context) error {
		order = append(order, "handler")
		return nil
	})
	registerTestRoute(r, rt)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if len(order) != 2 || order[0] != "before" || order[1] != "handler" {
		t.Errorf("expected [before handler], got %v", order)
	}
}

func TestRouterUseAfter(t *testing.T) {
	r := testRouter()
	var order []string

	r.UseAfter(func(c Context) error {
		order = append(order, "after")
		return c.Next()
	})
	rt := r.Get("/test", func(c Context) error {
		order = append(order, "handler")
		return c.Next()
	})
	registerTestRoute(r, rt)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if len(order) != 2 || order[0] != "handler" || order[1] != "after" {
		t.Errorf("expected [handler after], got %v", order)
	}
}

func TestRouterHasRoute(t *testing.T) {
	r := testRouter()
	r.Get("/exists", func(c Context) error { return nil })

	if !r.HasRoute("GET", "/exists") {
		t.Error("HasRoute should return true for registered route")
	}
	if r.HasRoute("GET", "/missing") {
		t.Error("HasRoute should return false for unregistered route")
	}
}

func TestRouterHandle(t *testing.T) {
	r := testRouter()
	r.Handle("/raw", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/raw", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRouterHandleFunc(t *testing.T) {
	r := testRouter()
	r.HandleFunc("/fn", func(w http.ResponseWriter, r *http.Request) {})
	req := httptest.NewRequest("GET", "/fn", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}

func TestRouterNotFound(t *testing.T) {
	r := testRouter()

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRouterMultipleHandlers(t *testing.T) {
	r := testRouter()
	var order []string

	rt := r.Get("/multi", func(c Context) error {
		order = append(order, "h1")
		return c.Next()
	}, func(c Context) error {
		order = append(order, "h2")
		return nil
	})
	registerTestRoute(r, rt)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/multi", nil)
	r.ServeHTTP(w, req)

	if len(order) != 2 || order[0] != "h1" || order[1] != "h2" {
		t.Errorf("expected [h1 h2], got %v", order)
	}
}

func TestRouterUtilMethods(t *testing.T) {
	names := []string{"PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	for _, name := range names {
		rt := testRouter()
		var route *route
		switch name {
		case "PUT":
			route = rt.Put("/test", func(c Context) error { return nil })
		case "PATCH":
			route = rt.Patch("/test", func(c Context) error { return nil })
		case "DELETE":
			route = rt.Delete("/test", func(c Context) error { return nil })
		case "HEAD":
			route = rt.Head("/test", func(c Context) error { return nil })
		case "OPTIONS":
			route = rt.Options("/test", func(c Context) error { return nil })
		}
		if route == nil {
			t.Errorf("%s route was nil", name)
		}
	}
}
