package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lemmego/api/di"
	"github.com/lemmego/api/session"
	"github.com/romsar/gonertia"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/lemmego/api/config"
	"github.com/lemmego/api/req"
	"github.com/lemmego/api/shared"

	"github.com/lemmego/db"
	"github.com/lemmego/migration/cmd"
)

// Global variable to hold the singleton instance
var instance *Application

// Once ensures that the initialization function is called only once
var once sync.Once

// Get returns the single instance of the app
func Get() App {
	once.Do(func() {
		instance = &Application{
			mu:                        sync.Mutex{},
			container:                 di.New(),
			router:                    newRouter(),
			config:                    config.GetInstance(),
			serviceRegistrarCallbacks: []func(a App) error{},
			bootStrapperCallbacks:     []func(a App) error{},
			runningInConsole:          len(os.Args) > 1,
		}
	})
	return instance
}

func init() {
	_ = Get()
}

type M map[string]any

// Error returns a string representation of the JSON-encoded map.
func (m M) Error() string {
	jsonEncoded, err := json.Marshal(m)
	if err != nil {
		return err.Error()
	}
	return string(jsonEncoded)
}

type Bootstrapper interface {
	WithConfig(c config.M) Bootstrapper
	WithCommands(commands []Command) Bootstrapper
	WithRoutes(routeCallback RouteCallback) Bootstrapper
	Run()
}

type AppCore interface {
	Config() config.Configuration
	Router() Router
	Container() *di.Container
	RunningInConsole() bool
	Bootstrapped() bool
	InProduction() bool
	Env(environment string) bool
	AddCommands(commands []Command)
	AddPublishables(publishables []*Publishable)
}

type App interface {
	AppCore
}

type AppEngine interface {
	Bootstrapper
	AppCore
}

// Application is the main application
type Application struct {
	container                 *di.Container
	mu                        sync.Mutex
	config                    config.Configuration
	router                    *HTTPRouter
	routeCallbacks            []RouteCallback
	serviceRegistrarCallbacks []func(a App) error
	bootStrapperCallbacks     []func(a App) error
	commands                  []Command
	middleware                []Handler
	httpMiddleware            []HTTPMiddleware
	runningInConsole          bool
	bootstrapped              bool

	publishables []*Publishable
}

type Options struct {
	Config   config.M
	Commands []Command
	Routes   RouteCallback
}

type OptFunc func(opts *Options)

func (a *Application) Container() *di.Container {
	return a.container
}

func (a *Application) Router() Router {
	return a.router
}

func (a *Application) Config() config.Configuration {
	return a.config
}

func (a *Application) AddCommands(commands []Command) {
	a.commands = append(a.commands, commands...)
}

func (a *Application) AddPublishables(publishables []*Publishable) {
	if a.Bootstrapped() {
		panic("cannot publish after app has been bootstrapped")
	}

	a.publishables = append(a.publishables, publishables...)
}

func WithConfig(config config.M) OptFunc {
	return func(opts *Options) {
		opts.Config = config
	}
}

func WithCommands(commands []Command) OptFunc {
	return func(opts *Options) {
		opts.Commands = commands
	}
}

func WithRoutes(routes RouteCallback) OptFunc {
	return func(opts *Options) {
		opts.Routes = routes
	}
}

func Configure(optFuncs ...OptFunc) AppEngine {
	opts := &Options{}

	for _, optFunc := range optFuncs {
		optFunc(opts)
	}

	i := Get().(*Application)

	if opts.Config != nil {
		i.config.SetConfigMap(opts.Config)
	}

	if opts.Commands != nil && len(opts.Commands) > 0 {
		i.commands = append(i.commands, opts.Commands...)
	}

	if opts.Routes != nil {
		i.routeCallbacks = append(i.routeCallbacks, opts.Routes)
	}

	return i
}

func InProduction() bool {
	return os.Getenv("APP_ENV") == "production"
}

func Env(environment string) bool {
	return os.Getenv("APP_ENV") == environment
}

func (a *Application) InProduction() bool {
	return InProduction()
}

func (a *Application) Env(environment string) bool {
	return Env(environment)
}

func (a *Application) RunningInConsole() bool {
	return a.runningInConsole
}

func (a *Application) Bootstrapped() bool {
	return a.bootstrapped
}

// WithConfig sets the config map to the current config instance
func (a *Application) WithConfig(c config.M) Bootstrapper {
	a.config.SetConfigMap(c)
	return a
}

// WithRoutes calls the provided callback and registers the routes
func (a *Application) WithRoutes(routeCallback RouteCallback) Bootstrapper {
	a.routeCallbacks = append(a.routeCallbacks, routeCallback)
	return a
}

// WithCommands register the commands
func (a *Application) WithCommands(commands []Command) Bootstrapper {
	a.commands = commands
	return a
}

func (a *Application) registerCommands() {
	for _, command := range a.commands {
		rootCmd.AddCommand(command(a))
	}

	rootCmd.AddCommand(publishCmd)

	rootCmd.AddCommand(cmd.MigrateCmd)

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

func (a *Application) registerServiceProviders() {
	for _, callback := range a.serviceRegistrarCallbacks {
		if err := callback(a); err != nil {
			panic(err)
		}
	}

	for _, callback := range a.bootStrapperCallbacks {
		if err := callback(a); err != nil {
			panic(err)
		}
	}

	a.bootstrapped = true

	return
}

func (a *Application) registerMiddlewares() {
	if a.router != nil {
		for _, middleware := range a.httpMiddleware {
			a.router.Use(middleware)
		}

		for _, middleware := range a.middleware {
			a.router.UseBefore(middleware)
		}
	}
}

func (a *Application) registerRoutes() {
	for _, cb := range a.routeCallbacks {
		cb(a.router)
	}

	for _, route := range a.router.routes {
		slog.Debug(fmt.Sprintf("Registering route: %s %s", route.Method, route.Path))
		a.router.mux.HandleFunc(route.Method+" "+route.Path, func(w http.ResponseWriter, req *http.Request) {
			makeHandlerFunc(a, route)(w, req)
		})
	}

	// Register error endpoint if not overridden already
	if !a.router.HasRoute("GET", "/error") {
		a.router.Get("/error", func(c *Context) error {
			err := c.PopSession("error").(string)
			return c.Status(500).HTML([]byte("<html><body><code>" + err + "</code></body></html>"))
		})
	}

	a.router.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	a.router.mux.Handle("GET /public/", http.StripPrefix("/public/", http.FileServer(http.Dir("public"))))
}

func makeHandlerFunc(app *Application, route *Route) http.HandlerFunc {
	fn := func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("Handling request for route: %s %s", route.Method, route.Path)
		if route.router == nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		sess, err := di.Resolve[*session.Session](app.Container())
		if err != nil {
			slog.Error(err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		token := sess.Token(r.Context())
		if token != "" {
			r = r.WithContext(context.WithValue(r.Context(), "sessionID", token))
			slog.Debug("Current session ID: ", token)
		}

		allHandlers := append(append([]Handler{}, route.BeforeMiddleware...), route.Handlers...)
		allHandlers = append(allHandlers, route.AfterMiddleware...)

		ctx := &Context{
			Mutex:    sync.Mutex{},
			app:      app,
			request:  r,
			writer:   w,
			handlers: allHandlers,
			index:    -1,
		}

		if err := ctx.Next(); err != nil {
			if errors.As(err, &shared.ValidationErrors{}) {
				ctx.ValidationError(err)
				return
			}

			var mfr *req.MalformedRequest
			if errors.As(err, &mfr) {
				ctx.Error(mfr.Status, mfr)
				return
			}

			if errors.As(err, &M{}) {
				ctx.JSON(err.(M))
				return
			}

			ctx.Error(http.StatusInternalServerError, err)
			return
		}
	}

	if i, err := di.Resolve[*gonertia.Inertia](app.Container()); err == nil && i != nil {
		return i.Middleware(http.HandlerFunc(fn)).ServeHTTP
	}

	return fn
}

func (a *Application) Run() {
	// Check if the main config is nil
	if a.config == nil {
		panic("main configuration is missing")
	}

	// Check if the app configuration is nil
	if a.config.Get("app") == nil {
		panic("app configuration is missing")
	}

	// Check if the database configuration is nil
	if a.config.Get("database") == nil {
		panic("database configuration is missing")
	}

	// Check if the redis configuration is nil
	if a.config.Get("redis") == nil {
		panic("redis configuration is missing")
	}

	// Check if the session configuration is nil
	if a.config.Get("session") == nil {
		panic("session configuration is missing")
	}

	// Check if the filesystem configuration is nil
	if a.config.Get("filesystems") == nil {
		panic("filesystem configuration is missing")
	}

	a.registerServiceProviders()

	if a.RunningInConsole() {
		publish(a.publishables)
		a.registerCommands()
	}

	a.registerMiddlewares()

	a.registerRoutes()

	if a.RunningInConsole() {
		a.shutDown()
		os.Exit(0)
	}
	sess, err := di.Resolve[*session.Session](a.Container())

	if err != nil {
		panic(err)
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", a.config.Get("app.port", 3000)),
		Handler: sess.LoadAndSave(a.router),
	}

	// Start the server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %s\n", err)
		}
	}()
	slog.Info(fmt.Sprintf("%s is running on port %d, Press Ctrl+C to close the server...", a.config.Get("app.name", "Lemmego"), a.config.Get("app.port", 3000)))
	a.HandleSignals(srv)
}

func (a *Application) HandleSignals(srv *http.Server) {
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	sig := <-signalChannel
	switch sig {
	case syscall.SIGINT, syscall.SIGTERM:
		// Gracefully shutdown the server
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Server forced to shutdown: %v", err)
		}

		// Close database connections
		a.shutDown()
		os.Exit(0)
	}
}

func (a *Application) shutDown() {
	if !a.RunningInConsole() {
		slog.Info("Shutting down application...")
	}

	for name, conn := range db.DM().All() {
		err := conn.Close()
		if err != nil {
			log.Fatal(fmt.Sprintf("Error closing database connection: %s", name), err)
		}
		if !a.RunningInConsole() {
			slog.Info(fmt.Sprintf("Closing database connection: %s", name))
		}
	}
}
