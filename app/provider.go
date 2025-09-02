package app

type Provider interface {
	// Provide provides the events
	Provide(a App) error
}

type CommandProvider interface {
	// AddCommands appends the given commands to the existing ones
	AddCommands() []Command
}

type RouteProvider interface {
	// AddRoutes appends the given routes to the existing ones
	AddRoutes() RouteCallback
}

type MiddlewareProvider interface {
	// AddMiddlewares appends the given middleware to the existing ones
	AddMiddlewares() []Handler
}

type PublishableProvider interface {
	// AddPublishables publishes the publishable assets
	AddPublishables() []*publishable
}

func Get[T any](a App) T {
	var zero T
	return a.Service(zero).(T)
}
