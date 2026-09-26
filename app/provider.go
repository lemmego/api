package app

import (
	"context"
	"fmt"
)

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

type ErrMapProvider interface {
	// AddErrMap appends the given errMap to the existing ones
	AddErrMap() ErrMap
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
	value, ok := Lookup[T](a)
	if !ok {
		var zero T
		panic(fmt.Sprintf("service %T is not registered", zero))
	}
	return value
}

// Lookup returns a typed service when it is registered. Unlike Get, it does
// not panic when an optional service is absent or has an unexpected type.
//
// T may be an interface. The lookup keys on the type parameter rather than on
// a value's dynamic type, which is why it goes through the registry instead of
// App.Service: a nil interface value carries no type, so reflect.TypeOf would
// see nothing to key on and every interface lookup would miss.
func Lookup[T any](a App) (T, bool) {
	return a.Services().Lookup[T]()
}

// RegisterIfAbsent stores service under T when nothing is registered there
// yet, and reports whether it did. T may be an interface, which is the point:
// it is how a package registers an implementation under a shared seam without
// the consumer having to know which concrete type provided it.
func RegisterIfAbsent[T any](a App, service T) bool {
	return a.Services().RegisterIfAbsent[T](service)
}
