package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lemmego/api/session"
	"github.com/lemmego/api/utils"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"

	"github.com/lemmego/api/config"
	"github.com/lemmego/api/req"
	"github.com/lemmego/api/shared"

	"github.com/lemmego/api/db"
	"github.com/lemmego/migration/cmd"

	"github.com/romsar/gonertia"
	"github.com/spf13/cobra"
)

type PluginID string

type PluginRegistry map[PluginID]Plugin

// Get a plugin
func (r PluginRegistry) Get(plugin Plugin) Plugin {
	//nameSpace := fmt.Sprintf("%T", plugin)
	nameSpace := utils.PkgName(plugin)

	if val, ok := r[PluginID(nameSpace)]; ok {
		return val
	}

	return nil
}

// Has returns if a plugin with the specified id exists
func (r PluginRegistry) Has(plugin Plugin) bool {
	//nameSpace := fmt.Sprintf("%T", plugin)
	nameSpace := utils.PkgName(plugin)

	_, ok := r[PluginID(nameSpace)]

	return ok
}

// Add a plugin
func (r PluginRegistry) Add(plugin Plugin) {
	if r.Has(plugin) {
		panic(fmt.Sprintf("plugin %v already registered", plugin))
	}

	//nameSpace := fmt.Sprintf("%T", plugin)
	nameSpace := utils.PkgName(plugin)

	r[PluginID(nameSpace)] = plugin
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

type Plugin interface {
	InstallCommand() *cobra.Command
	Commands() []*cobra.Command
	EventListeners() map[string]func()
	Publishables() []*Publishable
	Middlewares() []HTTPMiddleware
	Routes() []*Route
}

type Bootstrapper interface {
	WithPlugins(plugins map[PluginID]Plugin) Bootstrapper
	WithProviders(providers []Provider) Bootstrapper
	WithConfig(c config.M) Bootstrapper
	WithCommands(commands []Command) Bootstrapper
	WithRoutes(routeCallback func(r Router)) Bootstrapper
	HandleSignals()
	Run()
}

type ServiceRegistrar interface {
	Service(serviceType interface{}) error
	AddService(serviceType interface{})
}

type AppManager interface {
	//container.ServiceContainer
	ServiceRegistrar
	AppEngine
}

type AppEngine interface {
	Plugin(Plugin) Plugin
	Plugins() PluginRegistry
	Config() *config.Config
	Router() Router
	RunningInConsole() bool
}

type AppBootstrapper interface {
	Bootstrapper
	ServiceRegistrar
	AppEngine
}

type AppHooks struct {
	BeforeStart func()
	AfterStart  func()
}

// ServiceContainer holds all the application's dependencies
type ServiceContainer struct {
	services map[reflect.Type]interface{}
	mutex    sync.RWMutex
}

// NewServiceContainer creates a new ServiceContainer
func NewServiceContainer() *ServiceContainer {
	return &ServiceContainer{
		services: make(map[reflect.Type]interface{}),
	}
}

// Add adds a service to the container
func (sc *ServiceContainer) Add(service interface{}) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	t := reflect.TypeOf(service)
	// Store by pointer type if it's a pointer, otherwise by value type
	if t.Kind() == reflect.Ptr {
		sc.services[t] = service
	} else {
		sc.services[reflect.PointerTo(t)] = service
	}
}

// Get retrieves a service from the container and populates the provided pointer
func (sc *ServiceContainer) Get(service interface{}) error {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()

	// Get the type of the pointer passed in
	ptrType := reflect.TypeOf(service)
	if ptrType.Kind() != reflect.Ptr || ptrType.Elem().Kind() != reflect.Ptr {
		return fmt.Errorf("service must be a pointer to a pointer")
	}

	// Extract the element type (the actual service type)
	serviceType := ptrType.Elem()

	// Retrieve the service from the container
	if svc, exists := sc.services[serviceType]; exists {
		reflect.ValueOf(service).Elem().Set(reflect.ValueOf(svc))
		return nil
	}

	return fmt.Errorf("service of type %v not found", serviceType)
}

// App is the main application
type App struct {
	//*container.Container
	Services         *ServiceContainer
	mu               sync.Mutex
	config           *config.Config
	plugins          PluginRegistry
	serviceProviders []Provider
	hooks            *AppHooks
	router           *HTTPRouter
	routeCallback    func(r Router)
	commands         []Command
	runningInConsole bool
}

type Options struct {
	Config           config.M
	Commands         []*cobra.Command
	Plugins          map[PluginID]Plugin
	ServiceProviders []Provider
	Hooks            *AppHooks
}

type OptFunc func(opts *Options)

//func (app *App) Reset() {
//	if app.container != nil {
//		app.container.Clear()
//	}
//}

func (a *App) publishPackages() {
	if err := publishCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func (a *App) Plugin(plugin Plugin) Plugin {
	return a.plugins.Get(plugin)
}

func (a *App) Plugins() PluginRegistry {
	return a.plugins
}

func (a *App) Router() Router {
	return a.router
}

func (a *App) Config() *config.Config {
	return a.config
}

func WithPlugins(plugins map[PluginID]Plugin) OptFunc {
	return func(opts *Options) {
		opts.Plugins = plugins
	}
}

func WithProviders(providers []Provider) OptFunc {
	return func(opts *Options) {
		opts.ServiceProviders = providers
	}
}

func WithConfig(config config.M) OptFunc {
	return func(opts *Options) {
		opts.Config = config
	}
}

func New(optFuncs ...OptFunc) AppBootstrapper {
	opts := &Options{}

	for _, optFunc := range optFuncs {
		optFunc(opts)
	}

	container := NewServiceContainer()
	router := NewRouter()
	conf := config.NewConfig()

	if opts.Config != nil {
		conf.SetConfigMap(opts.Config)
	}

	app := &App{
		//Container:        container.NewContainer(),
		Services:         container,
		mu:               sync.Mutex{},
		config:           conf,
		plugins:          opts.Plugins,
		serviceProviders: opts.ServiceProviders,
		hooks:            opts.Hooks,
		router:           router,
		runningInConsole: len(os.Args) > 1,
	}

	return app
}

func (a *App) RunningInConsole() bool {
	return a.runningInConsole
}

// Service is a helper method to easily get a service
func (a *App) Service(serviceType interface{}) error {
	return a.Services.Get(serviceType)
}

// AddService is a helper method to easily add a service
func (a *App) AddService(serviceType interface{}) {
	a.Services.Add(serviceType)
}

// AddPlugin adds a plugin to the list of plugins
func (a *App) AddPlugin(plugin Plugin) {
	if a.plugins == nil {
		a.plugins = map[PluginID]Plugin{}
	}

	a.plugins.Add(plugin)
}

// WithPlugins appends the given map of plugins to the existing plugins
func (a *App) WithPlugins(plugins map[PluginID]Plugin) Bootstrapper {
	for _, plugin := range plugins {
		a.AddPlugin(plugin)
	}

	return a
}

// AddProvider adds a service provider to the list of providers
func (a *App) AddProvider(provider Provider) {
	a.serviceProviders = append(a.serviceProviders, provider)
}

// WithProviders appends the given slice to the existing providers
func (a *App) WithProviders(providers []Provider) Bootstrapper {
	a.serviceProviders = append(a.serviceProviders, providers...)
	return a
}

// WithConfig sets the config map to the current config instance
func (a *App) WithConfig(c config.M) Bootstrapper {
	a.config.SetConfigMap(c)
	return a
}

// WithRoutes calls the provided callback and registers the routes
func (a *App) WithRoutes(routeCallback func(r Router)) Bootstrapper {
	a.routeCallback = routeCallback
	return a
}

// WithCommands register the commands
func (a *App) WithCommands(commands []Command) Bootstrapper {
	a.commands = commands
	return a
}

// hasServiceProvider recursively checks if the inheritance tree has ServiceProvider or *ServiceProvider embedded.
func (a *App) hasServiceProvider(obj interface{}) bool {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem() // Dereference pointer to access underlying struct
	}

	// Check if the type is a struct
	if v.Kind() != reflect.Struct {
		return false
	}

	// Define types for ServiceProvider and *ServiceProvider for comparison
	serviceProviderType := reflect.TypeOf(ServiceProvider{})
	ptrServiceProviderType := reflect.TypeOf(&ServiceProvider{})

	// Traverse fields to check for direct or embedded initialized ServiceProvider
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := v.Type().Field(i)

		// Check if the field is of type ServiceProvider
		if field.Type() == serviceProviderType {
			return true // Struct ServiceProvider is embedded
		}

		// Check if the field is of type *ServiceProvider and is initialized (non-nil)
		if field.Type() == ptrServiceProviderType && !field.IsNil() {
			return true // Pointer ServiceProvider is embedded and initialized
		}

		// If the field is anonymous (embedded struct), recursively check it
		if fieldType.Anonymous && field.Kind() == reflect.Struct {
			if a.hasServiceProvider(field.Interface()) {
				return true
			}
		}
	}

	return false
}

// embedDefaultServiceProvider embeds &ServiceProvider{} if missing in the hierarchy.
func (a *App) embedDefaultServiceProvider(obj interface{}) error {
	v := reflect.ValueOf(obj).Elem()
	serviceProviderType := reflect.TypeOf(&ServiceProvider{})

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := v.Type().Field(i)

		// Embed default &ServiceProvider{} if field is of type *ServiceProvider and nil
		if field.Type() == serviceProviderType && field.IsNil() {
			field.Set(reflect.ValueOf(&ServiceProvider{App: a, publishables: make([]*Publishable, 0)}))
			return nil
		}

		// If the field is anonymous, recurse to check its fields
		if fieldType.Anonymous && field.Kind() == reflect.Ptr && field.IsValid() && !field.IsNil() {
			if err := a.embedDefaultServiceProvider(field.Interface()); err == nil {
				return nil
			}
		}
	}

	return errors.New(fmt.Sprintf("no suitable field to embed ServiceProvider into %T", obj))
}

func (a *App) registerCommands() {
	for _, command := range a.commands {
		rootCmd.AddCommand(command(a))
	}

	rootCmd.AddCommand(publishCmd)

	rootCmd.AddCommand(cmd.MigrateCmd)
	//cmd.Execute()

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

func (a *App) registerServiceProviders() {
	providers := []interface{}{
		&DatabaseProvider{&ServiceProvider{App: a}},
		&SessionProvider{&ServiceProvider{App: a}},
		&FilesystemProvider{&ServiceProvider{App: a}},
		&InertiaProvider{&ServiceProvider{App: a}},
	}

	for _, provider := range a.serviceProviders {
		if !a.hasServiceProvider(provider) {
			if err := a.embedDefaultServiceProvider(provider); err != nil {
				panic(err)
			}
		}
		providers = append(providers, provider)
	}

	for _, service := range providers {
		if val, ok := service.(Provider); ok {
			val.Register(a)
		}
	}

	for _, service := range providers {
		if val, ok := service.(Provider); ok {
			val.Boot(a)
		}

		if val, ok := service.(Publisher); ok {
			if val.RouteCallback() != nil {
				val.RouteCallback()(a.router)
			}

			if len(val.Commands()) > 0 {
				a.commands = append(a.commands, val.Commands()...)
			}

			if len(val.Publishables()) > 0 {
				if err := publish(a, val.Publishables()).Execute(); err != nil {
					panic(err)
				}
			}
		}
	}
}

func (a *App) registerMiddlewares() {
	for _, plugin := range a.plugins {
		for _, mw := range plugin.Middlewares() {
			a.router.Use(mw)
		}
	}
}

func (a *App) registerRoutes() {
	a.router.Handle("GET /static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	a.router.Handle("GET /public/*", http.StripPrefix("/public/", http.FileServer(http.Dir("public"))))

	a.routeCallback(a.router)

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
			return c.HTML(500, []byte("<html><body><code>"+err+"</code></body></html>"))
		})
	}
}

func makeHandlerFunc(app *App, route *Route) http.HandlerFunc {
	fn := func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("Handling request for route: %s %s", route.Method, route.Path)
		if route.router == nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		var sess *session.Session
		if err := app.Service(&sess); err != nil {
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
			slog.Error(err.Error())
			if errors.As(err, &shared.ValidationErrors{}) {
				ctx.ValidationError(err)
				return
			}

			var mfr *req.MalformedRequest
			if errors.As(err, &mfr) {
				ctx.Error(mfr.Status, mfr)
				return
			}

			ctx.Error(http.StatusInternalServerError, err)
			return
		}
	}

	var i *gonertia.Inertia

	if app.Service(&i) == nil {
		return i.Middleware(http.HandlerFunc(fn)).ServeHTTP
	}

	return fn
}

func (a *App) Run() {
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

	// Check if the filesystem configuration is nil
	if a.config.Get("filesystems") == nil {
		panic("filesystem configuration is missing")
	}

	a.config.Set("app.name", "foo")

	if !a.RunningInConsole() {
		slog.Info("app will start using the following config:\n", "config", a.config.GetAll())
	}

	a.registerServiceProviders()

	if a.RunningInConsole() {
		a.registerCommands()
	}

	a.registerMiddlewares()

	a.registerRoutes()

	if a.RunningInConsole() {
		a.shutDown()
		os.Exit(0)
	}

	var sess *session.Session
	if err := a.Service(&sess); &sess == nil || err != nil {
		panic(err)
	}

	slog.Info(fmt.Sprintf("%s is running on port %d...\n\nPress Ctrl+C to close the server", a.config.Get("app.name", "Lemmego"), a.config.Get("app.port", 3000)))
	if err := http.ListenAndServe(fmt.Sprintf(":%d", a.config.Get("app.port", 3000)), sess.LoadAndSave(a.router)); err != nil {
		panic(err)
	}
}

func (a *App) HandleSignals() {
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	sig := <-signalChannel
	switch sig {
	case syscall.SIGINT, syscall.SIGTERM:
		a.shutDown()
		os.Exit(0)
	}
}

func (a *App) shutDown() {
	slog.Debug("Shutting down application...")
	var dbm *db.DatabaseManager
	if err := a.Service(&dbm); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	for _, conn := range dbm.All() {
		err := conn.Close()
		if err != nil {
			log.Fatal(fmt.Sprintf("Error closing database connection: %s", conn.ConnName()), err)
		}
		slog.Debug(fmt.Sprintf("Closing database connection: %s, with connected database %s", conn.ConnName(), conn.DBName()))
	}
}
