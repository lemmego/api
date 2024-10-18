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
	"strings"
	"sync"
	"syscall"

	"github.com/lemmego/api/config"
	"github.com/lemmego/api/fsys"
	"github.com/lemmego/api/logger"
	"github.com/lemmego/api/req"
	"github.com/lemmego/api/shared"

	"github.com/lemmego/api/db"

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

type Publishable struct {
	FilePath string
	Content  []byte
	Tag      string
}

func (p *Publishable) Publish() error {
	filePath := p.FilePath

	if !strings.HasSuffix(filePath, ".go") {
		filePath = filePath + ".go"
	}

	if _, err := os.Stat(filePath); err != nil {
		err := os.WriteFile(filePath, []byte(p.Content), 0644)
		if err != nil {
			return err
		}
		slog.Info("Copied file to %s\n", filePath)
	} else {
		return err
	}
	return nil
}

type Plugin interface {
	Boot(a AppManager) error
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
	WithRoutes(routeCallback func(r Router)) Bootstrapper
	Run()
	HandleSignals()
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
	Session() *session.Session
	Inertia() *gonertia.Inertia
	DB() *db.DB
	DbFunc(c context.Context, config *db.Config) (*db.DB, error)
	FS() fsys.FS
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
func (sc *ServiceContainer) Get(output interface{}) error {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()

	// Get the type of the pointer passed in
	ptrType := reflect.TypeOf(output)
	if ptrType.Kind() != reflect.Ptr || ptrType.Elem().Kind() != reflect.Ptr {
		return fmt.Errorf("output must be a pointer to a pointer")
	}

	// Extract the element type (the actual service type)
	serviceType := ptrType.Elem()

	// Retrieve the service from the container
	if service, exists := sc.services[serviceType]; exists {
		reflect.ValueOf(output).Elem().Set(reflect.ValueOf(service))
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
	db               *db.DB
	dbFunc           func(c context.Context, config *db.Config) (*db.DB, error)
	routeCallback    func(r Router)
}

type Options struct {
	Config           config.M
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

func (a *App) Session() *session.Session {
	var sess *session.Session
	if err := a.Service(&sess); err != nil {
		panic(err)
	}
	return sess
}

//func (app *App) SetSession(session *session.Session) {
//	app.session = session
//}

func (a *App) Inertia() *gonertia.Inertia {
	var i *gonertia.Inertia
	if err := a.Service(&i); err != nil {
		panic(err)
	}
	return i
}

func (a *App) DB() *db.DB {
	return a.db
}

func (a *App) DbFunc(c context.Context, config *db.Config) (*db.DB, error) {
	return a.dbFunc(c, config)
}

func (a *App) SetDB(db *db.DB) {
	a.db = db
}

func (a *App) SetDbFunc(dbFunc func(c context.Context, config *db.Config) (*db.DB, error)) {
	a.dbFunc = dbFunc
}

func (a *App) FS() fsys.FS {
	var fs fsys.FS
	if err := a.Service(&fs); err != nil {
		panic(err)
	}
	return fs
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
	config := config.NewConfig()

	if opts.Config != nil {
		config.SetConfigMap(opts.Config)
	}

	app := &App{
		//Container:        container.NewContainer(),
		Services:         container,
		mu:               sync.Mutex{},
		config:           config,
		plugins:          opts.Plugins,
		serviceProviders: opts.ServiceProviders,
		hooks:            opts.Hooks,
		router:           router,
	}

	return app
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

func (a *App) registerServiceProviders() {
	providers := []Provider{
		&DatabaseProvider{&ServiceProvider{App: a}},
		&SessionProvider{&ServiceProvider{App: a}},
		&AuthServiceProvider{&ServiceProvider{App: a}},
		&FSProvider{&ServiceProvider{App: a}},
		&InertiaProvider{&ServiceProvider{App: a}},
	}

	providers = append(providers, a.serviceProviders...)

	for _, svc := range providers {
		svc.Register(a)
	}

	for _, service := range providers {
		service.Boot(a)
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

	for pluginID, plugin := range a.plugins {
		for _, route := range plugin.Routes() {
			if !a.router.HasRoute(route.Method, route.Path) {
				log.Println("Adding route for the", pluginID, "plugin:", route.Method, route.Path)
				a.router.addRoute(route.Method, route.Path, route.Handlers...)
				//r.routes = append(r.routes, route)
			}
		}
	}

	for _, route := range a.router.routes {
		log.Printf("Registering route: %s %s, router: %p", route.Method, route.Path, route.router)
		a.router.mux.HandleFunc(route.Method+" "+route.Path, func(w http.ResponseWriter, req *http.Request) {
			makeHandlerFunc(a, route)(w, req)
		})
	}
}

func makeHandlerFunc(app *App, route *Route) http.HandlerFunc {
	fn := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Handling request for route: %s %s", route.Method, route.Path)
		if route.router == nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		var sess *session.Session
		if err := app.Service(&sess); err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		token := sess.Token(r.Context())
		if token != "" {
			r = r.WithContext(context.WithValue(r.Context(), "sessionID", token))
			log.Println("Current session ID: ", token)
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
			logger.V().Error(err.Error())
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

	if app.Inertia() != nil {
		return app.Inertia().Middleware(http.HandlerFunc(fn)).ServeHTTP
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

	slog.Info("app will start using the following config:\n", "config", a.config.GetAll())

	a.registerServiceProviders()

	a.registerMiddlewares()

	a.registerRoutes()

	for _, plugin := range a.plugins {
		if err := plugin.Boot(a); err != nil {
			panic(err)
		}
	}

	var sess *session.Session
	if err := a.Service(&sess); &sess == nil || err != nil {
		panic(err)
	}

	slog.Info(fmt.Sprintf("%s is running on port %d...", a.config.Get("app.name", "Lemmego"), a.config.Get("app.port", 3000)))
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
	log.Println("Shutting down application...")
	dbName := a.db.Name()
	err := a.db.Close()
	if err != nil {
		log.Fatal("Error closing database connection:", err)
	}
	log.Println("Database connection", dbName, "closed.")
}
