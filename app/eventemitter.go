package app

import (
	"context"
	"log/slog"
	"sync"
)

const (
	MiddlewareRegistering = "middleware.registering"
	MiddlewareRegistered  = "middleware.registered"
	RoutesRegistering     = "routes.registering"
	RoutesRegistered      = "routes.registered"
	CommandsRegistering   = "commands.registering"
	CommandsRegistered    = "commands.registered"
	ServicesRegistering   = "services.registering"
	ServicesRegistered    = "services.registered"
	ServerStarted         = "server.started"
)

type EventListener func(payload any) error

type EventEmitter interface {
	On(event string, listener EventListener)
	Dispatch(event string, payload ...any)
}

type eventRegistry struct {
	mu         sync.RWMutex
	events     map[string][]EventListener
	shutdown   bool
	shutdownMu sync.Mutex
}

func newEventRegistry() *eventRegistry {
	return &eventRegistry{
		mu:     sync.RWMutex{},
		events: make(map[string][]EventListener),
	}
}

func (r *eventRegistry) Dispatch(event string, payload any) {
	r.shutdownMu.Lock()
	shutdown := r.shutdown
	r.shutdownMu.Unlock()

	if shutdown {
		slog.Warn("attempted to dispatch event after shutdown", "event", event)
		return
	}

	if r.Has(event) {
		for _, listener := range r.events[event] {
			if err := listener(payload); err != nil {
				slog.Error(err.Error())
			}
		}
	}
}

func (r *eventRegistry) On(event string, listener EventListener) {
	r.shutdownMu.Lock()
	shutdown := r.shutdown
	r.shutdownMu.Unlock()

	if shutdown {
		panic("cannot register event listener after shutdown")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[event] = append(r.events[event], listener)
}

func (r *eventRegistry) All() []any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]any, 0, len(r.events))
	for _, p := range r.events {
		out = append(out, p)
	}
	return out
}

func (r *eventRegistry) Get(event string) ([]EventListener, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	service, ok := r.events[event]
	return service, ok
}

// Remove unregisters the listeners of an event
func (r *eventRegistry) Remove(event string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.events[event]; exists {
		delete(r.events, event)
		return true
	}
	return false
}

// Clear removes all events
func (r *eventRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = make(map[string][]EventListener)
}

// Count returns the number of registered events
func (r *eventRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.events)
}

// Has checks if a service type is registered
func (r *eventRegistry) Has(event string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.events[event]
	return exists
}

// Shutdown gracefully shuts down the event emitter
// It prevents new events from being dispatched and listeners from being registered
func (r *eventRegistry) Shutdown(ctx context.Context) error {
	r.shutdownMu.Lock()
	if r.shutdown {
		r.shutdownMu.Unlock()
		return nil
	}
	r.shutdown = true
	r.shutdownMu.Unlock()

	// Wait for context cancellation or timeout
	select {
	case <-ctx.Done():
		slog.Warn("event emitter shutdown context cancelled")
		return ctx.Err()
	default:
		// Shutdown complete
		slog.Info("event emitter shut down successfully")
		return nil
	}
}

// IsShutdown returns true if the event emitter has been shut down
func (r *eventRegistry) IsShutdown() bool {
	r.shutdownMu.Lock()
	defer r.shutdownMu.Unlock()
	return r.shutdown
}
