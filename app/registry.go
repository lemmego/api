package app

import (
	"reflect"
	"sync"
)

// ServiceRegistry stores application services by their registered type.
// It is exposed through AppCore.Services for typed consumer access.
type ServiceRegistry struct {
	mu       sync.RWMutex
	services map[reflect.Type]any
}

func newServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		mu:       sync.RWMutex{},
		services: make(map[reflect.Type]any),
	}
}

// Register stores a service under the explicitly selected type T. T is
// inferred from the value for ordinary concrete registration, or can be
// specified to register an implementation under an interface type.
func (r *ServiceRegistry) Register[T any](service T) {
	typeOfService := reflect.TypeFor[T]()
	serviceType := reflect.TypeOf(service)
	if serviceType == nil {
		panic("service cannot be nil")
	}
	if !serviceType.AssignableTo(typeOfService) {
		panic("service does not implement registered type")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.services[typeOfService]; ok {
		panic("service already registered")
	}
	r.services[typeOfService] = service
}

// RegisterIfAbsent stores service under T when nothing is registered there
// yet, and reports whether it did.
//
// Unlike Register it never panics on a second registration. That is what a
// seam registered by whichever of several interchangeable providers happens
// to run first needs: two connectors in one application is a legitimate
// wiring, and it should resolve deterministically rather than fail to boot.
// The check and the write are under one lock, so concurrent registration
// cannot produce two winners.
func (r *ServiceRegistry) RegisterIfAbsent[T any](service T) bool {
	typeOfService := reflect.TypeFor[T]()
	serviceType := reflect.TypeOf(service)
	if serviceType == nil {
		panic("service cannot be nil")
	}
	if !serviceType.AssignableTo(typeOfService) {
		panic("service does not implement registered type")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.services[typeOfService]; ok {
		return false
	}
	r.services[typeOfService] = service
	return true
}

// RegisterValue preserves concrete-type registration for dynamic callers.
func (r *ServiceRegistry) RegisterValue(service any) {
	if service == nil {
		panic("service cannot be nil")
	}
	r.registerValue(reflect.TypeOf(service), service)
}

func (r *ServiceRegistry) registerValue(serviceType reflect.Type, service any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.services[serviceType]; ok {
		panic("service already registered")
	}
	r.services[serviceType] = service
}

func (r *ServiceRegistry) All() []any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]any, 0, len(r.services))
	for _, p := range r.services {
		out = append(out, p)
	}
	return out
}

func (r *ServiceRegistry) GetValue(p any) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	service, ok := r.services[reflect.TypeOf(p)]
	return service, ok
}

// GetByType is more efficient - no need to create instance
func (r *ServiceRegistry) GetByType(t reflect.Type) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	service, ok := r.services[t]
	return service, ok
}

// Lookup returns a typed service without panicking when it is absent or has
// the wrong registered type.
func (r *ServiceRegistry) Lookup[T any]() (T, bool) {
	var zero T
	service, ok := r.GetByType(reflect.TypeFor[T]())
	if !ok {
		return zero, false
	}
	typed, ok := service.(T)
	return typed, ok
}

// MustGet returns a typed service or panics when it is unavailable.
func (r *ServiceRegistry) MustGet[T any]() T {
	service, ok := r.Lookup[T]()
	if !ok {
		panic("service not registered")
	}
	return service
}

// Has reports whether a service is registered under T.
func (r *ServiceRegistry) Has[T any]() bool {
	_, ok := r.GetByType(reflect.TypeFor[T]())
	return ok
}

// GetTyped preserves the existing package-level typed lookup helper.
func GetTyped[T any](r *ServiceRegistry) (T, bool) {
	return r.Lookup[T]()
}

// Remove unregisters a service
func (r *ServiceRegistry) Remove(p Provider) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := reflect.TypeOf(p)
	if _, exists := r.services[t]; exists {
		delete(r.services, t)
		return true
	}
	return false
}

// Clear removes all providers
func (r *ServiceRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.services = make(map[reflect.Type]any)
}

// Count returns the number of registered providers
func (r *ServiceRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.services)
}

// Has checks if a service type is registered
func (r *ServiceRegistry) HasValue(p any) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.services[reflect.TypeOf(p)]
	return exists
}
