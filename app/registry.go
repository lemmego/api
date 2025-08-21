package app

import (
	"reflect"
	"sync"
)

type ServiceRegistry struct {
	mu       sync.RWMutex
	services map[reflect.Type]any
}

func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		mu:       sync.RWMutex{},
		services: make(map[reflect.Type]any),
	}
}

func (r *ServiceRegistry) Register(p any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.services[reflect.TypeOf(p)]; ok {
		panic("service already registered")
	}
	r.services[reflect.TypeOf(p)] = p
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

func (r *ServiceRegistry) Get(p any) (any, bool) {
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

// GetTyped provides type-safe service retrieval
func GetTyped[T any](r *ServiceRegistry) (T, bool) {
	var zero T
	service, ok := r.GetByType(reflect.TypeOf(zero))
	if !ok {
		return zero, false
	}
	typed, ok := service.(T)
	return typed, ok
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
func (r *ServiceRegistry) Has(p any) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.services[reflect.TypeOf(p)]
	return exists
}
