package app

import "reflect"

type Provider interface {
	// Provide provides the services
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
	AddPublishables() []*Publishable
}

func Get[T any](a App) T {
	var zero T
	return a.Service(reflect.TypeOf(zero)).(T)
}
