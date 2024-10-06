package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lemmego/api/container"
	"github.com/lemmego/api/session"
	"github.com/lemmego/api/utils"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
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
	FileName string
	DirPath  string
	Content  []byte
}

func (p *Publishable) Publish() error {
	filePath := filepath.Join(p.DirPath, p.FileName)
	if _, err := os.Stat(filePath); err != nil {
		err := os.WriteFile(filePath, []byte(p.Content), 0644)
		if err != nil {
			return err
		}
		slog.Info("Copied file %s to %s\n", p.FileName, filePath)
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

type AppManager interface {
	container.ServiceContainer
	Plugin(Plugin) Plugin
	Plugins() PluginRegistry
	Router() *Router
	Session() *session.Session
	Inertia() *gonertia.Inertia
	DB() *db.DB
	DbFunc(c context.Context, config *db.Config) (*db.DB, error)
	FS() fsys.FS
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

// Register adds a service to the container
func (sc *ServiceContainer) Register(service interface{}) {
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

// Get retrieves a service from the container
func (sc *ServiceContainer) Get(serviceType reflect.Type) (interface{}, error) {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()

	// If serviceType is not a pointer, convert it to a pointer type
	if serviceType.Kind() != reflect.Ptr {
		serviceType = reflect.PointerTo(serviceType)
	}

	if service, exists := sc.services[serviceType]; exists {
		return service, nil
	}
	return nil, fmt.Errorf("service of type %v not found", serviceType)
}

// App is the main application
type App struct {
	*container.Container
	Services         *ServiceContainer
	mu               sync.Mutex
	config           config.ConfigMap
	plugins          PluginRegistry
	serviceProviders []ServiceProvider
	hooks            *AppHooks
	router           *Router
	db               *db.DB
	dbFunc           func(c context.Context, config *db.Config) (*db.DB, error)
	i                *gonertia.Inertia
	routeRegistrar   RouteRegistrarFunc
	//fs               fsys.FS
}

type Options struct {
	Config           config.ConfigMap
	Plugins          map[PluginID]Plugin
	ServiceProviders []ServiceProvider
	Hooks            *AppHooks
	inertia          *gonertia.Inertia
	//fs               fsys.FS
}

type OptFunc func(opts *Options)

//func (app *App) Reset() {
//	if app.container != nil {
//		app.container.Clear()
//	}
//}

func (app *App) Plugin(plugin Plugin) Plugin {
	return app.plugins.Get(plugin)
}

func (app *App) Plugins() PluginRegistry {
	return app.plugins
}

func (app *App) RegisterRoutes(fn RouteRegistrarFunc) {
	app.routeRegistrar = fn
}

func (app *App) Router() *Router {
	return app.router
}

func (app *App) Session() *session.Session {
	var sess *session.Session
	if err := app.Resolve(&sess); err != nil {
		panic(err)
	}
	return sess
}

//func (app *App) SetSession(session *session.Session) {
//	app.session = session
//}

func (app *App) Inertia() *gonertia.Inertia {
	return app.i
}

func (app *App) DB() *db.DB {
	return app.db
}

func (app *App) DbFunc(c context.Context, config *db.Config) (*db.DB, error) {
	return app.dbFunc(c, config)
}

func (app *App) SetDB(db *db.DB) {
	app.db = db
}

func (app *App) SetDbFunc(dbFunc func(c context.Context, config *db.Config) (*db.DB, error)) {
	app.dbFunc = dbFunc
}

func (app *App) FS() fsys.FS {
	var fs fsys.FS
	if err := app.Resolve(&fs); err != nil {
		panic(err)
	}
	return fs
}

func getDefaultConfig() config.ConfigMap {
	return config.GetAll()
}

func defaultOptions() *Options {
	return &Options{
		getDefaultConfig(),
		nil,
		nil,
		nil,
		nil,
	}
}

func WithPlugins(plugins map[PluginID]Plugin) OptFunc {
	return func(opts *Options) {
		opts.Plugins = plugins
	}
}

func WithProviders(providers []ServiceProvider) OptFunc {
	return func(opts *Options) {
		opts.ServiceProviders = providers
	}
}

func WithInertia(i *gonertia.Inertia) OptFunc {
	if i == nil {
		i = initInertia()
	}
	return func(opts *Options) {
		opts.inertia = i
	}
}

func New(optFuncs ...OptFunc) *App {
	opts := defaultOptions()

	for _, optFunc := range optFuncs {
		optFunc(opts)
	}

	for _, plugin := range opts.Plugins {
		for _, p := range plugin.Publishables() {
			if err := p.Publish(); err != nil {
				panic(err)
			}
		}
	}

	router := NewRouter()

	inertia := opts.inertia
	app := &App{
		Container: container.NewContainer(),
		//Services:         NewServiceContainer(),
		mu:               sync.Mutex{},
		config:           opts.Config,
		plugins:          opts.Plugins,
		serviceProviders: opts.ServiceProviders,
		hooks:            opts.Hooks,
		router:           router,
		i:                inertia,
	}

	return app
}

// Helper method to easily get a service
func (a *App) Service(serviceType interface{}) (interface{}, error) {
	return a.Services.Get(reflect.TypeOf(serviceType).Elem())
}

func (app *App) registerServiceProviders(serviceProviders []ServiceProvider) {
	serviceProviders = append(serviceProviders, app.serviceProviders...)
	for _, svc := range serviceProviders {
		extendsBase := false
		if reflect.TypeOf(svc).Kind() != reflect.Ptr {
			panic("Service must be a pointer")
		}
		if reflect.TypeOf(svc).Elem().Kind() != reflect.Struct {
			panic("Service must be a struct")
		}

		// Iterate over all the fields of the struct and see if it extends *BaseServiceProvider
		for i := 0; i < reflect.TypeOf(svc).Elem().NumField(); i++ {
			if reflect.TypeOf(svc).Elem().Field(i).Type == reflect.PointerTo(reflect.TypeOf(BaseServiceProvider{})) {
				extendsBase = true
				break
			}
		}

		if !extendsBase {
			panic("Service must extend BaseServiceProvider")
		}

		// Check if service implements ServiceProvider interface, not necessary if type hinted
		if reflect.TypeOf(svc).Implements(reflect.TypeOf((*ServiceProvider)(nil)).Elem()) {
			slog.Info("Registering service: " + reflect.TypeOf(svc).Elem().Name())
			svc.Register(app)
			app.serviceProviders = append(app.serviceProviders, svc)
		} else {
			panic("Service must implement ServiceProvider interface")
		}
	}

	for _, service := range serviceProviders {
		if reflect.TypeOf(service).Implements(reflect.TypeOf((*ServiceProvider)(nil)).Elem()) {
			service.Boot()
		}
	}
}

func (app *App) registerMiddlewares() {
	for _, plugin := range app.plugins {
		for _, mw := range plugin.Middlewares() {
			app.router.Use(mw)
		}
	}
}

func (app *App) registerRoutes() {
	if app.routeRegistrar != nil {
		app.routeRegistrar(app.router)
	}

	for pluginID, plugin := range app.plugins {
		for _, route := range plugin.Routes() {
			if !app.router.HasRoute(route.Method, route.Path) {
				log.Println("Adding route for the", pluginID, "plugin:", route.Method, route.Path)
				app.router.addRoute(route.Method, route.Path, route.Handlers...)
				//r.routes = append(r.routes, route)
			}
		}
	}

	for _, route := range app.router.routes {
		log.Printf("Registering route: %s %s, router: %p", route.Method, route.Path, route.router)
		app.router.mux.HandleFunc(route.Method+" "+route.Path, func(w http.ResponseWriter, req *http.Request) {
			makeHandlerFunc(app, route)(w, req)
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
		if err := app.Resolve(&sess); err != nil {
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

	if app.i != nil {
		return app.Inertia().Middleware(http.HandlerFunc(fn)).ServeHTTP
	}

	return fn
}

func (app *App) Run() {
	app.registerServiceProviders([]ServiceProvider{
		&DatabaseServiceProvider{},
		&SessionServiceProvider{},
		&AuthServiceProvider{},
		&FSServiceProvider{},
	})

	app.registerMiddlewares()

	app.registerRoutes()

	app.router.Handle("GET /static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	app.router.Handle("GET /public/*", http.StripPrefix("/public/", http.FileServer(http.Dir("public"))))

	for _, plugin := range app.plugins {
		if err := plugin.Boot(app); err != nil {
			panic(err)
		}
	}

	var sess *session.Session
	if err := app.Resolve(&sess); &sess == nil || err != nil {
		panic(err)
	}

	slog.Info(fmt.Sprintf("%s is running on port %d...", config.Get[string]("app.name"), config.Get[int]("app.port")))
	if err := http.ListenAndServe(fmt.Sprintf(":%d", config.Get[int]("app.port")), sess.LoadAndSave(app.router)); err != nil {
		panic(err)
	}
}

func (app *App) HandleSignals() {
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	sig := <-signalChannel
	switch sig {
	case syscall.SIGINT, syscall.SIGTERM:
		app.shutDown()
		os.Exit(0)
	}
}

func (app *App) shutDown() {
	log.Println("Shutting down application...")
	dbName := app.db.Name()
	err := app.db.Close()
	if err != nil {
		log.Fatal("Error closing database connection:", err)
	}
	log.Println("Database connection", dbName, "closed.")
}
