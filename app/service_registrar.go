package app

func RegisterService(registrar ...func(a App) error) {
	if instance == nil {
		Get()
	}

	if instance.Bootstrapped() {
		panic("cannot register service after app has been bootstrapped")
	}

	instance.serviceRegistrarCallbacks = append(instance.serviceRegistrarCallbacks, registrar...)
}

func BootService(bootstrapper ...func(a App) error) {
	if instance == nil {
		Get()
	}

	if instance.Bootstrapped() {
		panic("cannot boot service after app has been bootstrapped")
	}

	instance.bootStrapperCallbacks = append(instance.bootStrapperCallbacks, bootstrapper...)
}

func RegisterCommands(commands ...Command) {
	if instance == nil {
		Get()
	}

	if instance.Bootstrapped() {
		panic("cannot register commands after app has been bootstrapped")
	}

	instance.commands = append(instance.commands, commands...)
}

func RegisterRoutes(routes ...RouteCallback) {
	if instance == nil {
		Get()
	}

	if instance.Bootstrapped() {
		panic("cannot register routes after app has been bootstrapped")
	}

	instance.routeCallbacks = append(instance.routeCallbacks, routes...)
}

func RegisterHTTPMiddleware(middleware ...HTTPMiddleware) {
	if instance == nil {
		Get()
	}

	if instance.Bootstrapped() {
		panic("cannot register http middleware after app has been bootstrapped")
	}

	instance.httpMiddleware = append(instance.httpMiddleware, middleware...)
}

func RegisterMiddleware(middleware ...Handler) {
	if instance == nil {
		Get()
	}

	if instance.Bootstrapped() {
		panic("cannot register middleware after app has been bootstrapped")
	}

	instance.middleware = append(instance.middleware, middleware...)
}
