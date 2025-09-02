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

type Handler func(c Context) error

type Middleware func(next Handler) Handler

type HTTPMiddleware func(http.Handler) http.Handler

type RouteCallback func(a App)

type route struct {
	Method           string
	Path             string
	Handlers         []Handler
	BeforeMiddleware []Handler
	AfterMiddleware  []Handler
	router           *httpRouter
}

type httpRouter struct {
	routes           []*route
	httpMiddlewares  []HTTPMiddleware
	basePrefix       string
	mux              *http.ServeMux
	beforeMiddleware []Handler
	afterMiddleware  []Handler
}

type routeGroup struct {
	router           *httpRouter
	prefix           string
	beforeMiddleware []Handler
	afterMiddleware  []Handler
}

func (g *routeGroup) Group(prefix string) *routeGroup {
	return &routeGroup{
		router:           g.router,
		prefix:           path.Join(g.prefix, prefix),
		beforeMiddleware: append([]Handler{}, g.beforeMiddleware...),
		afterMiddleware:  append([]Handler{}, g.afterMiddleware...),
	}
}

func (g *routeGroup) UseBefore(handlers ...Handler) {
	g.beforeMiddleware = append(g.beforeMiddleware, handlers...)
}

func (g *routeGroup) UseAfter(handlers ...Handler) {
	g.afterMiddleware = append(handlers, g.afterMiddleware...)
}

func (g *routeGroup) addRoute(method, pattern string, handlers ...Handler) *route {
	fullPath := path.Join(g.prefix, pattern)
	route := &route{
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

func (g *routeGroup) Get(pattern string, handlers ...Handler) *route {
	return g.addRoute(http.MethodGet, pattern, handlers...)
}

func (g *routeGroup) Post(pattern string, handlers ...Handler) *route {
	return g.addRoute(http.MethodPost, pattern, handlers...)
}

func (g *routeGroup) Put(pattern string, handlers ...Handler) *route {
	return g.addRoute(http.MethodPut, pattern, handlers...)
}

func (g *routeGroup) Patch(pattern string, handlers ...Handler) *route {
	return g.addRoute(http.MethodPatch, pattern, handlers...)
}

func (g *routeGroup) Delete(pattern string, handlers ...Handler) *route {
	return g.addRoute(http.MethodDelete, pattern, handlers...)
}

// newRouter creates a new httpRouter-based router
func newRouter() *httpRouter {
	return &httpRouter{
		routes:           []*route{},
		httpMiddlewares:  []HTTPMiddleware{},
		mux:              http.NewServeMux(),
		beforeMiddleware: []Handler{},
		afterMiddleware:  []Handler{},
	}
}

func (r *httpRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var handler http.Handler = r.mux
	for i := len(r.httpMiddlewares) - 1; i >= 0; i-- {
		handler = r.httpMiddlewares[i](handler)
	}
	handler.ServeHTTP(w, req)
}

func (r *httpRouter) Group(prefix string) *routeGroup {
	return &routeGroup{
		router:           r,
		prefix:           prefix,
		beforeMiddleware: []Handler{},
		afterMiddleware:  []Handler{},
	}
}

func (r *httpRouter) UseBefore(handlers ...Handler) {
	r.beforeMiddleware = append(r.beforeMiddleware, handlers...)
}

func (r *httpRouter) UseAfter(handlers ...Handler) {
	r.afterMiddleware = append(handlers, r.afterMiddleware...)
}

func (r *httpRouter) HasRoute(method string, pattern string) bool {
	return slices.ContainsFunc(r.routes, func(route *route) bool {
		return route.Method == method && route.Path == pattern
	})
}

func (r *httpRouter) Handle(pattern string, handler http.Handler) {
	r.mux.Handle(pattern, handler)
}

func (r *httpRouter) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	r.mux.HandleFunc(pattern, handler)
}

func (r *httpRouter) Get(pattern string, handlers ...Handler) *route {
	return r.addRoute(http.MethodGet, pattern, handlers...)
}

func (r *httpRouter) Post(pattern string, handlers ...Handler) *route {
	return r.addRoute(http.MethodPost, pattern, handlers...)
}

func (r *httpRouter) Put(pattern string, handlers ...Handler) *route {
	return r.addRoute(http.MethodPut, pattern, handlers...)
}

func (r *httpRouter) Patch(pattern string, handlers ...Handler) *route {
	return r.addRoute(http.MethodPatch, pattern, handlers...)
}

func (r *httpRouter) Delete(pattern string, handlers ...Handler) *route {
	return r.addRoute(http.MethodDelete, pattern, handlers...)
}

func (r *httpRouter) Connect(pattern string, handlers ...Handler) *route {
	return r.addRoute(http.MethodConnect, pattern, handlers...)
}

func (r *httpRouter) Head(pattern string, handlers ...Handler) *route {
	return r.addRoute(http.MethodHead, pattern, handlers...)
}

func (r *httpRouter) Options(pattern string, handlers ...Handler) *route {
	return r.addRoute(http.MethodOptions, pattern, handlers...)
}

func (r *httpRouter) Trace(pattern string, handlers ...Handler) *route {
	return r.addRoute(http.MethodTrace, pattern, handlers...)
}

// Use adds one or more standard net/http middleware to the router
func (r *httpRouter) Use(middlewares ...HTTPMiddleware) {
	r.httpMiddlewares = append(r.httpMiddlewares, middlewares...)
}

func (r *httpRouter) addRoute(method, pattern string, handlers ...Handler) *route {
	fullPath := path.Join(r.basePrefix, pattern)
	route := &route{
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

func (r *route) UseBefore(handlers ...Handler) *route {
	r.BeforeMiddleware = append(r.BeforeMiddleware, handlers...)
	return r
}

func (r *route) UseAfter(handlers ...Handler) *route {
	r.AfterMiddleware = append(handlers, r.AfterMiddleware...)
	return r
}

func Input(inputStruct any, opts ...core.Option) Middleware {
	co, err := httpin.New(inputStruct, opts...)

	if err != nil {
		panic(err)
	}

	return func(next Handler) Handler {
		return func(ctx Context) error {
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
	Group(prefix string) *routeGroup
	UseBefore(handlers ...Handler)
	UseAfter(handlers ...Handler)
	HasRoute(method string, pattern string) bool
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
	Get(pattern string, handlers ...Handler) *route
	Post(pattern string, handlers ...Handler) *route
	Put(pattern string, handlers ...Handler) *route
	Patch(pattern string, handlers ...Handler) *route
	Delete(pattern string, handlers ...Handler) *route
	Connect(pattern string, handlers ...Handler) *route
	Head(pattern string, handlers ...Handler) *route
	Options(pattern string, handlers ...Handler) *route
	Trace(pattern string, handlers ...Handler) *route
	Use(middlewares ...HTTPMiddleware)
}
