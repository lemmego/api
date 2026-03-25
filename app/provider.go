package app

import "context"

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
	AddPublishables() []*Publishable
}

// ShutdownProvider is an optional interface that providers can implement
// to perform cleanup when the application shuts down. Implementations should
// be idempotent and safe to call multiple times.
type ShutdownProvider interface {
	// Shutdown performs cleanup operations when the application is shutting down.
	// It receives a context for timeout control and should return any error that
	// occurs during shutdown. The context may already be cancelled when Shutdown
	// is called if the shutdown timeout has expired.
	Shutdown(ctx context.Context) error
}

func Get[T any](a App) T {
	var zero T
	return a.Service(zero).(T)
}
