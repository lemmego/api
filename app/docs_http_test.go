package app_test

import (
	"net/http"
	"testing"

	"github.com/lemmego/api/app"
)

// The HTTP section of the docs site, compiled.
//
// Every Context method, router shape, error helper and middleware form the
// pages at /docs/http use, exercised with the arity they are documented
// with. Nothing here asserts behaviour — the point is narrower and is the
// one the docs kept getting wrong: that the call exists, on the interface a
// handler is actually handed, taking the arguments the page shows.
//
// It was written after an audit found two that did not. The pages had
// `router.Input(&CreateUserInput{})`, where there is no router package and
// the function is app.Input; and `c.Input().(*CreateUserInput)`, where Input
// takes the struct to decode into and the no-argument form does not compile.
// Both read perfectly and neither would have survived a reader.

// documentedContextCalls is every c.X(...) the http pages make.
//
// A compile-time list rather than a runtime one: calling them needs a live
// request, and what is being checked is the signature, which the compiler
// settles on its own.
func documentedContextCalls(c app.Context) {
	// request-handling.md
	_ = c.Param("id")
	_ = c.Query("page")
	_ = c.Input(&struct{}{})
	_ = c.Get(app.HTTPInKey)
	_ = c.ParseInput(&struct{}{})
	_, _, _ = c.FormFile("avatar")
	_, _ = c.Upload("avatar", "avatars")
	_ = c.HasFile("avatar")
	_ = c.HasJSONRequest()
	_ = c.HasMultiPartRequest()
	_ = c.HasFormDataRequest()
	_ = c.HasFormURLEncodedRequest()
	_ = c.Header("X-Request-Id")
	_ = c.Referer()
	_ = c.Request()
	_ = c.RequestContext()

	// response-handling.md
	_ = c.JSON(app.M{"id": 1})
	_ = c.Text([]byte("hello"))
	_ = c.XML(struct{ ID int }{1})
	_ = c.NoContent()
	_ = c.Redirect("/login")
	_ = c.Back()
	_ = c.SetStatus(http.StatusCreated)
	_ = c.SetHeader("X-Thing", "1")
	_ = c.WantsJSON()
	_ = c.WantsHTML()
	_ = c.ResponseWriter()

	// error-handling.md — all variadic, so both documented forms compile.
	_ = c.NotFound()
	_ = c.Unauthorized()
	_ = c.Forbidden()
	_ = c.BadRequest()
	_ = c.InternalServerError()
	_ = c.PageExpired()
	_ = c.MethodNotAllowed()
	_ = c.UnprocessableEntity()

	// context.md
	c.Set("key", "value")
	_ = c.App()
	_ = c.Validator()
	_ = c.SetCookie(&http.Cookie{Name: "pref", Value: "dark"})
	_ = c.Cookie("pref")
	_ = c.PutSession("user_id", 1)
	_ = c.Session("user_id")
	_ = c.SessionString("flash")
	_ = c.PopSession("flash")
	_ = c.PopSessionString("flash")
}

// The sentinel errors error-handling.md lists, with the codes it gives them.
func TestDocsErrorSentinelsExist(t *testing.T) {
	for _, err := range []error{
		app.ErrUnauthorized,
		app.ErrForbidden,
		app.ErrNotFound,
		app.ErrBadRequest,
		app.ErrMethodNotAllowed,
		app.ErrPageExpired,
		app.ErrUnprocessableEntity,
		app.ErrInternalServerError,
	} {
		if err == nil {
			t.Fatal("a sentinel the docs list is nil")
		}
	}
	_ = documentedContextCalls
}

// routing.md: a RouteCallback is func(app.App), and the Input middleware is
// app.Input rather than router.Input — the page said the latter, and there is
// no router package to hold it.
func TestDocsRoutingShapes(t *testing.T) {
	var _ app.RouteCallback = func(a app.App) {
		r := a.Router()

		r.Get("/hello", func(c app.Context) error {
			return c.Text([]byte("World"))
		})
		// Middleware on a route is an ordinary Handler, which is what the
		// pages now show. app.Input is deliberately not here: it returns an
		// app.Middleware, and nothing in the router accepts one — routes,
		// groups and the router all take app.Handler. The docs used to show
		// it wired to a route, which is what this line failing to compile
		// first revealed.
		r.Post("/users", requireAuth, func(c app.Context) error {
			return c.JSON(app.M{"ok": true})
		})

		api := r.Group("/api")
		api.Get("/users", func(c app.Context) error { return nil })

		v1 := api.Group("/v1")
		v1.Get("/products", func(c app.Context) error { return nil })

		admin := r.Group("/admin")
		admin.UseBefore(func(c app.Context) error { return nil })
	}
}

// requireAuth stands in for auth.Protected, which is func(app.Context) error
// — the shape every route middleware has.
func requireAuth(c app.Context) error { return c.Next() }

// middleware.md's custom-middleware shape.
func TestDocsMiddlewareShape(t *testing.T) {
	var _ app.Middleware = func(next app.Handler) app.Handler {
		return func(c app.Context) error {
			return next(c)
		}
	}
}
