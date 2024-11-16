package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"slices"

	"github.com/ggicci/httpin"
	"github.com/ggicci/httpin/core"
)

const HTTPInKey = "input"

type Handler func(c *Context) error

type Middleware func(next Handler) Handler

type HTTPMiddleware func(http.Handler) http.Handler

type RouteCallback func(r Router)

type Route struct {
	Method           string
	Path             string
	Handlers         []Handler
	BeforeMiddleware []Handler
	AfterMiddleware  []Handler
	router           *HTTPRouter
}

type HTTPRouter struct {
	routes           []*Route
	httpMiddlewares  []HTTPMiddleware
	basePrefix       string
	mux              *http.ServeMux
	beforeMiddleware []Handler
	afterMiddleware  []Handler
}

type Group struct {
	router           *HTTPRouter
	prefix           string
	beforeMiddleware []Handler
	afterMiddleware  []Handler
}

func (g *Group) Group(prefix string) *Group {
	return &Group{
		router:           g.router,
		prefix:           path.Join(g.prefix, prefix),
		beforeMiddleware: append([]Handler{}, g.beforeMiddleware...),
		afterMiddleware:  append([]Handler{}, g.afterMiddleware...),
	}
}

func (g *Group) UseBefore(handlers ...Handler) {
	g.beforeMiddleware = append(g.beforeMiddleware, handlers...)
}

func (g *Group) UseAfter(handlers ...Handler) {
	g.afterMiddleware = append(handlers, g.afterMiddleware...)
}

func (g *Group) addRoute(method, pattern string, handlers ...Handler) *Route {
	fullPath := path.Join(g.prefix, pattern)
	route := &Route{
		Method:           method,
		Path:             fullPath,
		Handlers:         handlers,
		BeforeMiddleware: append(append([]Handler{}, g.router.beforeMiddleware...), g.beforeMiddleware...),
		AfterMiddleware:  append(append([]Handler{}, g.afterMiddleware...), g.router.afterMiddleware...),
		router:           g.router,
	}
	g.router.routes = append(g.router.routes, route)
	return route
}

func (g *Group) Get(pattern string, handlers ...Handler) *Route {
	return g.addRoute(http.MethodGet, pattern, handlers...)
}

func (g *Group) Post(pattern string, handlers ...Handler) *Route {
	return g.addRoute(http.MethodPost, pattern, handlers...)
}

func (g *Group) Put(pattern string, handlers ...Handler) *Route {
	return g.addRoute(http.MethodPut, pattern, handlers...)
}

func (g *Group) Patch(pattern string, handlers ...Handler) *Route {
	return g.addRoute(http.MethodPatch, pattern, handlers...)
}

func (g *Group) Delete(pattern string, handlers ...Handler) *Route {
	return g.addRoute(http.MethodDelete, pattern, handlers...)
}

// newRouter creates a new HTTPRouter-based router
func newRouter() *HTTPRouter {
	return &HTTPRouter{
		routes:           []*Route{},
		httpMiddlewares:  []HTTPMiddleware{},
		mux:              http.NewServeMux(),
		beforeMiddleware: []Handler{},
		afterMiddleware:  []Handler{},
	}
}

func (r *HTTPRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var handler http.Handler = r.mux
	for i := len(r.httpMiddlewares) - 1; i >= 0; i-- {
		handler = r.httpMiddlewares[i](handler)
	}
	handler.ServeHTTP(w, req)
}

func (r *HTTPRouter) Group(prefix string) *Group {
	return &Group{
		router:           r,
		prefix:           prefix,
		beforeMiddleware: []Handler{},
		afterMiddleware:  []Handler{},
	}
}

func (r *HTTPRouter) UseBefore(handlers ...Handler) {
	r.beforeMiddleware = append(r.beforeMiddleware, handlers...)
}

func (r *HTTPRouter) UseAfter(handlers ...Handler) {
	r.afterMiddleware = append(handlers, r.afterMiddleware...)
}

func (r *HTTPRouter) HasRoute(method string, pattern string) bool {
	return slices.ContainsFunc(r.routes, func(route *Route) bool {
		return route.Method == method && route.Path == pattern
	})
}

func (r *HTTPRouter) Handle(pattern string, handler http.Handler) {
	r.mux.Handle(pattern, handler)
}

func (r *HTTPRouter) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	r.mux.HandleFunc(pattern, handler)
}

func (r *HTTPRouter) Get(pattern string, handlers ...Handler) *Route {
	return r.addRoute(http.MethodGet, pattern, handlers...)
}

func (r *HTTPRouter) Post(pattern string, handlers ...Handler) *Route {
	return r.addRoute(http.MethodPost, pattern, handlers...)
}

func (r *HTTPRouter) Put(pattern string, handlers ...Handler) *Route {
	return r.addRoute(http.MethodPut, pattern, handlers...)
}

func (r *HTTPRouter) Patch(pattern string, handlers ...Handler) *Route {
	return r.addRoute(http.MethodPatch, pattern, handlers...)
}

func (r *HTTPRouter) Delete(pattern string, handlers ...Handler) *Route {
	return r.addRoute(http.MethodDelete, pattern, handlers...)
}

func (r *HTTPRouter) Connect(pattern string, handlers ...Handler) *Route {
	return r.addRoute(http.MethodConnect, pattern, handlers...)
}

func (r *HTTPRouter) Head(pattern string, handlers ...Handler) *Route {
	return r.addRoute(http.MethodHead, pattern, handlers...)
}

func (r *HTTPRouter) Options(pattern string, handlers ...Handler) *Route {
	return r.addRoute(http.MethodOptions, pattern, handlers...)
}

func (r *HTTPRouter) Trace(pattern string, handlers ...Handler) *Route {
	return r.addRoute(http.MethodTrace, pattern, handlers...)
}

// Use adds one or more standard net/http middleware to the router
func (r *HTTPRouter) Use(middlewares ...HTTPMiddleware) {
	r.httpMiddlewares = append(r.httpMiddlewares, middlewares...)
}

func (r *HTTPRouter) addRoute(method, pattern string, handlers ...Handler) *Route {
	fullPath := path.Join(r.basePrefix, pattern)
	route := &Route{
		Method:           method,
		Path:             fullPath,
		Handlers:         handlers,
		BeforeMiddleware: r.beforeMiddleware,
		AfterMiddleware:  r.afterMiddleware,
		router:           r,
	}
	r.routes = append(r.routes, route)
	slog.Debug(fmt.Sprintf("Added route: %s %s", method, fullPath))
	return route
}

func (r *Route) UseBefore(handlers ...Handler) *Route {
	r.BeforeMiddleware = append(r.BeforeMiddleware, handlers...)
	return r
}

func (r *Route) UseAfter(handlers ...Handler) *Route {
	r.AfterMiddleware = append(handlers, r.AfterMiddleware...)
	return r
}

func Input(inputStruct any, opts ...core.Option) Middleware {
	co, err := httpin.New(inputStruct, opts...)

	if err != nil {
		panic(err)
	}

	return func(next Handler) Handler {
		return func(ctx *Context) error {
			input, err := co.Decode(ctx.Request())
			if err != nil {
				co.GetErrorHandler()(ctx.ResponseWriter(), ctx.Request(), err)
				return nil
			}

			ctx.Set(HTTPInKey, input)
			return next(ctx)
		}
	}
}

type Router interface {
	Group(prefix string) *Group
	UseBefore(handlers ...Handler)
	UseAfter(handlers ...Handler)
	HasRoute(method string, pattern string) bool
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
	Get(pattern string, handlers ...Handler) *Route
	Post(pattern string, handlers ...Handler) *Route
	Put(pattern string, handlers ...Handler) *Route
	Patch(pattern string, handlers ...Handler) *Route
	Delete(pattern string, handlers ...Handler) *Route
	Connect(pattern string, handlers ...Handler) *Route
	Head(pattern string, handlers ...Handler) *Route
	Options(pattern string, handlers ...Handler) *Route
	Trace(pattern string, handlers ...Handler) *Route
	Use(middlewares ...HTTPMiddleware)
}
