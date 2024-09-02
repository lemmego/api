package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/lemmego/api/session"
	"github.com/lemmego/api/shared"

	"github.com/lemmego/api/db"

	inertia "github.com/romsar/gonertia"
	"github.com/spf13/cobra"
)

type PluginID string

type PluginRegistry map[PluginID]Plugin

// Get a plugin
func (r PluginRegistry) Get(plugin Plugin) Plugin {
	nameSpace := fmt.Sprintf("%T", plugin)

	if val, ok := r[PluginID(nameSpace)]; ok {
		return val
	}

	return nil
}

// Add a plugin
func (r PluginRegistry) Add(plugin Plugin) {
	nameSpace := fmt.Sprintf("%T", plugin)
	r[PluginID(nameSpace)] = plugin
}

type M map[string]any

func (m M) Error() string {
	jsonEncoded, err := json.Marshal(m)
	if err != nil {
		return err.Error()
	}
	return string(jsonEncoded)
}

type Plugin interface {
	Boot(a AppManager) error
	InstallCommand() *cobra.Command
	Commands() []*cobra.Command
	EventListeners() map[string]func()
	PublishableMigrations() map[string][]byte
	PublishableModels() map[string][]byte
	PublishableTemplates() map[string][]byte
	Middlewares() []HTTPMiddleware
	Routes() []*Route
	Webhooks() []string
}

type AppManager interface {
	Plugin(Plugin) Plugin
	Plugins() PluginRegistry
	Router() *Router
	Session() *session.Session
	Inertia() *inertia.Inertia
	DB() *db.DB
	DbFunc(c context.Context, config *db.Config) (*db.DB, error)
	FS() fsys.FS
}

type AppHooks struct {
	BeforeStart func()
	AfterStart  func()
}

type App struct {
	//*container.Container
	isContextReady   bool
	mu               sync.Mutex
	session          *session.Session
	config           config.ConfigMap
	plugins          PluginRegistry
	serviceProviders []ServiceProvider
	hooks            *AppHooks
	router           *Router
	db               *db.DB
	dbFunc           func(c context.Context, config *db.Config) (*db.DB, error)
	i                *inertia.Inertia
	routeRegistrar   RouteRegistrarFunc
	fs               fsys.FS
}

type Options struct {
	//*container.Container
	*session.Session
	Config           config.ConfigMap
	Plugins          map[PluginID]Plugin
	ServiceProviders []ServiceProvider
	routeMiddlewares map[string]Middleware
	Hooks            *AppHooks
	inertia          *inertia.Inertia
	fs               fsys.FS
}

type OptFunc func(opts *Options)

//func (app *App) Reset() {
//	if app.Container != nil {
//		app.Container.Clear()
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
	return app.session
}

func (app *App) SetSession(session *session.Session) {
	app.session = session
}

func (app *App) Inertia() *inertia.Inertia {
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
	return app.fs
}

func getDefaultConfig() config.ConfigMap {
	return config.GetAll()
}

func defaultOptions() *Options {
	return &Options{
		//container.New(),
		nil,
		getDefaultConfig(),
		nil,
		nil,
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

func WithHooks(hooks *AppHooks) OptFunc {
	return func(opts *Options) {
		opts.Hooks = hooks
	}
}

func WithInertia(i *inertia.Inertia) OptFunc {
	if i == nil {
		i = initInertia()
	}
	return func(opts *Options) {
		opts.inertia = i
	}
}

func WithFS(fs fsys.FS) OptFunc {
	if fs == nil {
		fs = fsys.NewLocalStorage("./storage")
	}
	return func(opts *Options) {
		opts.fs = fs
	}
}

//func WithRouter(router HTTPRouter) OptFunc {
//	return func(opts *Options) {
//		opts.HTTPRouter = router
//	}
//}

//func WithContainer(container *container.Container) OptFunc {
//	return func(opts *Options) {
//		opts.Container = container
//	}
//}

func WithSession(sm *session.Session) OptFunc {
	return func(opts *Options) {
		opts.Session = sm
	}
}

func New(optFuncs ...OptFunc) *App {
	opts := defaultOptions()

	for _, optFunc := range optFuncs {
		optFunc(opts)
	}

	for _, plugin := range opts.Plugins {
		// Copy template files listed in the Views() method to the app's template directory
		for name, content := range plugin.PublishableTemplates() {
			filePath := filepath.Join(config.Get[string]("app.templateDir"), name)
			if _, err := os.Stat(filePath); err != nil {
				err := os.WriteFile(filePath, []byte(content), 0644)
				if err != nil {
					panic(err)
				}
				slog.Info("Copied template %s to %s\n", name, filePath)
			}
		}
	}

	router := NewRouter()

	inertia := opts.inertia
	app := &App{
		//Container:        opts.Container,
		isContextReady:   false,
		mu:               sync.Mutex{},
		config:           opts.Config,
		plugins:          opts.Plugins,
		serviceProviders: opts.ServiceProviders,
		hooks:            opts.Hooks,
		router:           router,
		i:                inertia,
		fs:               opts.fs,
	}
	return app
}

// func (app *App) Container() container.Container {
// 	return app.container
// }

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
	for namespace, plugin := range app.plugins {
		for _, route := range plugin.Routes() {
			if !app.router.HasRoute(route.Method, route.Path) {
				log.Println("Adding route for the", namespace, "plugin:", route.Method, route.Path)
				app.router.addRoute(route.Method, route.Path, route.Handlers...)
				//r.routes = append(r.routes, route)
			}
		}
	}

	for _, route := range app.router.routes {
		log.Printf("Registering route: %s %s, router: %p", route.Method, route.Path, route.router)
		app.router.mux.HandleFunc(route.Method+" "+route.Path, func(w http.ResponseWriter, req *http.Request) {
			makeHandlerFunc(app, route, app.router)(w, req)
		})
	}
}

func makeHandlerFunc(app *App, route *Route, router *Router) http.HandlerFunc {
	fn := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Handling request for route: %s %s, router: %p", route.Method, route.Path, router)
		if route.router == nil {
			log.Printf("WARNING: route.router is nil for %s %s", route.Method, route.Path)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		token := app.Session().Token(r.Context())
		if token != "" {
			r = r.WithContext(context.WithValue(r.Context(), "sessionID", token))
			log.Println("Current SessionID: ", token)
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
	})

	app.registerMiddlewares()

	// Call the route registrar function here
	if app.routeRegistrar != nil {
		app.routeRegistrar(app.router)
	}

	app.registerRoutes()

	app.router.Handle("GET /static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	app.router.Handle("GET /public/*", http.StripPrefix("/public/", http.FileServer(http.Dir("public"))))

	for _, plugin := range app.plugins {
		if err := plugin.Boot(app); err != nil {
			panic(err)
		}
	}

	slog.Info(fmt.Sprintf("%s is running on port %d...", config.Get[string]("app.name"), config.Get[int]("app.port")))
	if err := http.ListenAndServe(fmt.Sprintf(":%d", config.Get[int]("app.port")), app.session.LoadAndSave(app.router)); err != nil {
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
