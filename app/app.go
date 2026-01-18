// Package app provides the core application framework for Lemmego.
//
// This package contains the main application container, dependency injection system,
// HTTP routing, middleware pipeline, and service registration mechanisms.
// It serves as the central orchestrator for all framework components.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lemmego/api/fs"
	"github.com/lemmego/api/session"

	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/lemmego/gpa"

	"github.com/lemmego/api/config"
	"github.com/lemmego/api/req"
	"github.com/lemmego/api/shared"

	"github.com/lemmego/migration/cmd"
)

// M is a convenience type for map[string]any, commonly used for JSON responses
// and error payloads. It implements the error interface for use in error handling.
type M map[string]any

// Error returns a string representation of the JSON-encoded map.
// This allows M to be used as an error type that can be serialized to JSON.
func (m M) Error() string {
	jsonEncoded, err := json.Marshal(m)
	if err != nil {
		return err.Error()
	}
	return string(jsonEncoded)
}

// Bootstrapper defines the interface for configuring and starting the application.
// It provides a fluent API for registering various application components before
// the application starts handling requests.
type Bootstrapper interface {
	// WithConfig sets the configuration map for the application
	WithConfig(c config.M) Bootstrapper

	// WithCommands registers CLI commands with the application
	WithCommands(commands []Command) Bootstrapper

	// WithMiddlewares registers global middleware that runs before route handlers
	WithMiddlewares(middlewares []Handler) Bootstrapper

	// WithHTTPMiddlewares registers HTTP-level middleware in the request pipeline
	WithHTTPMiddlewares(middlewares []HTTPMiddleware) Bootstrapper

	// WithRoutes registers route callback functions for defining application routes
	WithRoutes(routeCallbacks []RouteCallback) Bootstrapper

	// WithProviders registers service providers that configure application services
	WithProviders(providers []Provider) Bootstrapper

	// Run starts the application, either as a web server or CLI command processor
	Run()
}

// AppCore defines the core functionality available to the application.
// It provides access to configuration, routing, sessions, file systems,
// and service management capabilities.
type AppCore interface {
	// Config returns the application configuration instance
	Config() config.Configuration

	// Router returns the HTTP router for registering routes
	Router() Router

	// Session returns the session manager instance
	Session() *session.Session

	// FileSystem returns the file system abstraction instance
	FileSystem() *fs.FileSystem

	// RunningInConsole returns true if the application is running as a CLI command
	RunningInConsole() bool

	// Bootstrapped returns true if the application has completed bootstrap phase
	Bootstrapped() bool

	// InProduction returns true if the application is running in production environment
	InProduction() bool

	// Env checks if the application is running in the specified environment
	Env(environment string) bool

	// AddService registers a service instance in the service container
	AddService(service any)

	// Service retrieves a service instance from the service container by type
	Service(service any) any

	// EventEmitter provides event publishing and subscription capabilities
	EventEmitter
}

// App represents the main application interface that combines core functionality.
// It serves as the primary interface for application instances.
type App interface {
	AppCore
}

// AppEngine combines bootstrapping and core functionality.
// It represents the complete application engine that can be configured and run.
type AppEngine interface {
	Bootstrapper
	AppCore
}

// application is the main application implementation that manages the entire
// framework lifecycle including configuration, routing, middleware, services,
// and request handling.
type application struct {
	mu               sync.Mutex           // Mutex for thread-safe operations
	config           config.Configuration // Application configuration
	router           *httpRouter          // HTTP router for handling requests
	routeCallbacks   []RouteCallback      // Route registration callbacks
	commands         []Command            // CLI commands
	middleware       []Handler            // Application-level middleware
	httpMiddleware   []HTTPMiddleware     // HTTP-level middleware
	runningInConsole bool                 // True if running as CLI command
	bootstrapped     bool                 // True if bootstrap phase completed

	publishables    []*publishable   // Assets and files that can be published
	providers       []Provider       // Service providers for dependency injection
	serviceRegistry *serviceRegistry // Container for registered services
	eventRegistry   *eventRegistry   // Event system for application events
}

func (a *application) On(event string, listener EventListener) {
	a.eventRegistry.On(event, listener)
}

func (a *application) Dispatch(event string, payload ...any) {
	a.eventRegistry.Dispatch(event, payload)
}

func (a *application) WithProviders(providers []Provider) Bootstrapper {
	a.providers = append(a.providers, providers...)
	return a
}

// Options contains configuration options for creating a new application instance.
// It allows specifying configuration, commands, routes, and providers during
// application initialization.
type Options struct {
	Config    config.M        // Configuration map
	Commands  []Command       // CLI commands to register
	Routes    []RouteCallback // Route registration callbacks
	Providers []Provider      // Service providers to register
}

// OptFunc is a function that modifies Options during application configuration.
// It enables a functional options pattern for flexible application setup.
type OptFunc func(opts *Options)

func (a *application) Router() Router {
	return a.router
}

func (a *application) Session() *session.Session {
	return Get[*session.Session](a)
}

func (a *application) FileSystem() *fs.FileSystem {
	return Get[*fs.FileSystem](a)
}

func (a *application) Config() config.Configuration {
	return a.config
}

func (a *application) AddService(service any) {
	a.serviceRegistry.Register(service)
}

func (a *application) Service(service any) any {
	val, ok := a.serviceRegistry.GetByType(reflect.TypeOf(service))
	if !ok {
		return nil
	}
	return val
}

// WithConfig returns an OptFunc that sets the configuration map for the application.
func WithConfig(config config.M) OptFunc {
	return func(opts *Options) {
		opts.Config = config
	}
}

// WithCommands returns an OptFunc that registers CLI commands with the application.
func WithCommands(commands []Command) OptFunc {
	return func(opts *Options) {
		opts.Commands = commands
	}
}

// WithRoutes returns an OptFunc that registers route callbacks with the application.
func WithRoutes(routes []RouteCallback) OptFunc {
	return func(opts *Options) {
		opts.Routes = routes
	}
}

// WithProviders returns an OptFunc that registers service providers with the application.
func WithProviders(providers []Provider) OptFunc {
	return func(opts *Options) {
		opts.Providers = providers
	}
}

// Configure creates and configures a new application instance using functional options.
// It initializes the application with the provided configuration, commands, routes, and providers.
func Configure(optFuncs ...OptFunc) AppEngine {
	opts := &Options{}

	for _, optFunc := range optFuncs {
		optFunc(opts)
	}

	i := &application{
		mu:               sync.Mutex{},
		router:           newRouter(),
		config:           config.GetInstance(),
		runningInConsole: len(os.Args) > 1,
		serviceRegistry:  newServiceRegistry(),
		eventRegistry:    newEventRegistry(),
	}

	if opts.Config != nil {
		i.config.SetConfigMap(opts.Config)
	}

	if opts.Commands != nil && len(opts.Commands) > 0 {
		i.commands = append(i.commands, opts.Commands...)
	}

	if opts.Routes != nil {
		i.routeCallbacks = append(i.routeCallbacks, opts.Routes...)
	}

	return i
}

// InProduction returns true if the application is running in production environment.
// It checks the APP_ENV environment variable.
func InProduction() bool {
	return os.Getenv("APP_ENV") == "production"
}

// Env checks if the application is running in the specified environment.
// It compares the APP_ENV environment variable with the provided environment name.
func Env(environment string) bool {
	return os.Getenv("APP_ENV") == environment
}

func (a *application) InProduction() bool {
	return InProduction()
}

func (a *application) Env(environment string) bool {
	return Env(environment)
}

func (a *application) RunningInConsole() bool {
	return a.runningInConsole
}

func (a *application) Bootstrapped() bool {
	return a.bootstrapped
}

// WithConfig sets the config map to the current config instance
func (a *application) WithConfig(c config.M) Bootstrapper {
	a.config.SetConfigMap(c)
	return a
}

// WithRoutes calls the provided callback and registers the routes
func (a *application) WithRoutes(routeCallbacks []RouteCallback) Bootstrapper {
	a.routeCallbacks = append(a.routeCallbacks, routeCallbacks...)
	return a
}

// WithMiddlewares accepts a slice of global middleware
func (a *application) WithMiddlewares(middlewares []Handler) Bootstrapper {
	a.middleware = append(a.middleware, middlewares...)
	return a
}

// WithHTTPMiddlewares accepts a slice of global middleware
func (a *application) WithHTTPMiddlewares(httpMiddlewares []HTTPMiddleware) Bootstrapper {
	a.httpMiddleware = append(a.httpMiddleware, httpMiddlewares...)
	return a
}

// WithCommands register the commands
func (a *application) WithCommands(commands []Command) Bootstrapper {
	a.commands = commands
	return a
}

func (a *application) registerCommands() {
	for _, command := range a.commands {
		rootCmd.AddCommand(command(a))
	}

	rootCmd.AddCommand(publishCmd)

	rootCmd.AddCommand(cmd.MigrateCmd)

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

func (a *application) registerProviders() {
	for _, provider := range a.providers {
		if err := provider.Provide(a); err != nil {
			panic(err)
		}
	}

	//var wg sync.WaitGroup
	//errorsCh := make(chan error, len(a.providers))
	//
	//// Register service providers in parallel
	//for _, provider := range a.providers {
	//	wg.Add(1)
	//	go func() {
	//		wg.Done()
	//		if err := provider.Provide(a); err != nil {
	//			errorsCh <- err
	//		}
	//	}()
	//}
	//
	//// Wait for all service registrations to complete
	//wg.Wait()
	//
	//// Check for errors from service registration
	//close(errorsCh)
	//for err := range errorsCh {
	//	panic(err)
	//}
}

func (a *application) registerMiddlewares() {
	if a.router != nil {
		for _, middleware := range a.httpMiddleware {
			a.router.Use(middleware)
		}

		for _, middleware := range a.middleware {
			a.router.UseBefore(middleware)
		}
	}
}

func (a *application) registerRoutes() {
	for _, cb := range a.routeCallbacks {
		cb(a)
	}

	for _, route := range a.router.routes {
		slog.Debug(fmt.Sprintf("Registering route: %s %s", route.Method, route.Path))
		a.router.mux.HandleFunc(route.Method+" "+route.Path, func(w http.ResponseWriter, req *http.Request) {
			makeHandlerFunc(a, route)(w, req)
		})
	}

	// Register error endpoint if not overridden already
	if !a.router.HasRoute("GET", "/error") {
		a.router.Get("/error", func(c Context) error {
			err := c.PopSession("error").(string)
			return c.SetStatus(500).HTML([]byte("<html><body><code>" + err + "</code></body></html>"))
		})
	}

	a.router.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	a.router.mux.Handle("GET /public/", http.StripPrefix("/public/", http.FileServer(http.Dir("public"))))
}

func makeHandlerFunc(app *application, route *route) http.HandlerFunc {
	fn := func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("Handling request for route: %s %s", route.Method, route.Path)
		if route.router == nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		sess := Get[*session.Session](app)
		//sess := session.Get(app)
		//if err != nil {
		//	slog.Error(err.Error())
		//	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		//	return
		//}

		token := sess.Token(r.Context())
		if token != "" {
			r = r.WithContext(context.WithValue(r.Context(), "sessionID", token))
			slog.Debug("Current session ID: ", token)
		}

		allHandlers := append(append([]Handler{}, route.BeforeMiddleware...), route.Handlers...)
		allHandlers = append(allHandlers, route.AfterMiddleware...)

		ctx := &ctx{
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

	//if i, err := di.Resolve[*gonertia.Inertia](app.Container()); err == nil && i != nil {
	//	return i.Middleware(http.HandlerFunc(fn)).ServeHTTP
	//}

	return fn
}

func (a *application) Run() {
	// Check if the main config is nil
	if a.config == nil {
		panic("main configuration is missing")
	}

	// Check if the app configuration is nil
	if a.config.Get("app") == nil {
		panic("app configuration is missing")
	}

	// Check if the sql configuration is nil
	if a.config.Get("sql") == nil {
		panic("sql configuration is missing")
	}

	// Check if the keyvalue configuration is nil
	if a.config.Get("keyvalue") == nil {
		panic("keyvalue configuration is missing")
	}

	// Check if the session configuration is nil
	if a.config.Get("session") == nil {
		panic("session configuration is missing")
	}

	// Check if the filesystem configuration is nil
	if a.config.Get("filesystems") == nil {
		panic("filesystem configuration is missing")
	}

	if a.RunningInConsole() {
		for _, provider := range a.providers {
			if commandProvider, ok := provider.(CommandProvider); ok {
				a.commands = append(a.commands, commandProvider.AddCommands()...)
			}
		}
		for _, provider := range a.providers {
			if publishProvider, ok := provider.(PublishableProvider); ok {
				a.publishables = append(a.publishables, publishProvider.AddPublishables()...)
			}
		}
		publish(a.publishables)
		a.Dispatch(CommandsRegistering)
		a.registerCommands()
		a.Dispatch(CommandsRegistered)
	}

	// Register middlewares and routes sequentially to avoid race conditions
	// Middleware must be registered before routes
	func() {
		defer func() {
			if r := recover(); r != nil {
				panic(fmt.Errorf("middleware registration failed: %v", r))
			}
		}()
		for _, provider := range a.providers {
			if mwProvider, ok := provider.(MiddlewareProvider); ok {
				a.middleware = append(a.middleware, mwProvider.AddMiddlewares()...)
			}
		}
		a.Dispatch(MiddlewareRegistering)
		a.registerMiddlewares()
		a.Dispatch(MiddlewareRegistered)
	}()

	func() {
		defer func() {
			if r := recover(); r != nil {
				panic(fmt.Errorf("route registration failed: %v", r))
			}
		}()
		for _, provider := range a.providers {
			if routeProvider, ok := provider.(RouteProvider); ok {
				a.routeCallbacks = append(a.routeCallbacks, routeProvider.AddRoutes())
			}
		}
	}()

	// Register providers first so services are available for routes
	a.Dispatch(ServicesRegistering)
	a.registerProviders()
	a.Dispatch(ServicesRegistered)

	if a.RunningInConsole() {
		a.shutDown()
		os.Exit(0)
	}

	a.Dispatch(RoutesRegistering)
	a.registerRoutes()
	a.Dispatch(RoutesRegistered)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", a.config.Get("app.port", 3000)),
		Handler: a.Session().LoadAndSave(a.router),
	}

	// Start the server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %s\n", err)
		}
	}()
	slog.Info(fmt.Sprintf("%s is running on port %d, Press Ctrl+C to close the server...", a.config.Get("app.name", "Lemmego"), a.config.Get("app.port", 3000)))
	a.Dispatch(ServerStarted)
	a.HandleSignals(srv)
}

func (a *application) HandleSignals(srv *http.Server) {
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	sig := <-signalChannel
	switch sig {
	case syscall.SIGINT, syscall.SIGTERM:
		// In development, detect if this is likely from Air vs manual Ctrl+C
		// Air will send SIGTERM/SIGKILL shortly after SIGINT, so we can
		// detect this by checking if we receive another signal quickly
		isAirRestart := false
		if !a.InProduction() {
			// Set up a short-lived channel to detect follow-up signals from Air
			quickSignalCheck := make(chan os.Signal, 1)
			signal.Notify(quickSignalCheck, syscall.SIGTERM, syscall.SIGKILL)

			select {
			case <-quickSignalCheck:
				// Received SIGTERM/SIGKILL quickly after SIGINT - likely Air
				isAirRestart = true
			case <-time.After(500 * time.Millisecond):
				// No follow-up signal - likely manual Ctrl+C
				isAirRestart = false
			}
			signal.Stop(quickSignalCheck)
		}

		// Use very short timeout for Air restarts, longer for manual shutdown
		timeout := 30 * time.Second
		if !a.InProduction() {
			if isAirRestart {
				timeout = 100 * time.Millisecond // Very fast for Air
			} else {
				timeout = 2 * time.Second // Still fast for manual dev shutdown
			}
		}

		// Gracefully shutdown the server
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Server forced to shutdown: %v", err)
		}

		// Skip expensive DB cleanup for Air restarts in development
		if !isAirRestart {
			a.shutDown()
		}
		os.Exit(0)
	}
}

func (a *application) shutDown() {
	if !a.RunningInConsole() {
		slog.Info("Shutting down application...")
	}
	err := gpa.Registry().RemoveAll()
	if err != nil {
		slog.Error(err.Error())
	}
}
