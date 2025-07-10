package di

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// Lifetime defines the lifetime of a service
type Lifetime int

const (
	Transient Lifetime = iota // New instance for each request
	Singleton                 // Single instance for container lifetime
	Scoped                    // Single instance per scope
)

// ServiceDescriptor describes how to create a service
type ServiceDescriptor struct {
	ServiceType reflect.Type
	Factory     interface{}
	Lifetime    Lifetime
	instance    interface{}
	mu          sync.RWMutex
}

// Container is the main DI container
type Container struct {
	services    map[reflect.Type]*ServiceDescriptor
	mu          sync.RWMutex
	resolving   map[reflect.Type]bool // For circular dependency detection
	resolvingMu sync.Mutex
	parent      *Container // For scoped containers
}

// New creates a new DI container
func New() *Container {
	return &Container{
		services:  make(map[reflect.Type]*ServiceDescriptor),
		resolving: make(map[reflect.Type]bool),
	}
}

// CreateScope creates a scoped container
func (c *Container) CreateScope() *Container {
	return &Container{
		services:  make(map[reflect.Type]*ServiceDescriptor),
		resolving: make(map[reflect.Type]bool),
		parent:    c,
	}
}

// Register registers a service with explicit type
func Register[T any](c *Container, lifetime Lifetime, factory interface{}) error {
	var zero T
	serviceType := reflect.TypeOf(zero)

	// Validate factory function
	factoryType := reflect.TypeOf(factory)
	if factoryType.Kind() != reflect.Func {
		return errors.New("factory must be a function")
	}

	// Validate factory returns correct type
	if factoryType.NumOut() == 0 {
		return errors.New("factory must return at least one value")
	}

	returnType := factoryType.Out(0)
	if !returnType.AssignableTo(serviceType) {
		return fmt.Errorf("factory return type %v is not assignable to service type %v",
			returnType, serviceType)
	}

	// Check for error return
	if factoryType.NumOut() > 1 {
		if factoryType.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
			return errors.New("factory second return value must be error")
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.services[serviceType] = &ServiceDescriptor{
		ServiceType: serviceType,
		Factory:     factory,
		Lifetime:    lifetime,
	}

	return nil
}

// RegisterSingleton is a convenience method for singleton registration
func RegisterSingleton[T any](c *Container, factory interface{}) error {
	return Register[T](c, Singleton, factory)
}

// RegisterTransient is a convenience method for transient registration
func RegisterTransient[T any](c *Container, factory interface{}) error {
	return Register[T](c, Transient, factory)
}

// RegisterScoped is a convenience method for scoped registration
func RegisterScoped[T any](c *Container, factory interface{}) error {
	return Register[T](c, Scoped, factory)
}

// RegisterInstance registers an existing instance as a singleton
func RegisterInstance[T any](c *Container, instance T) error {
	var zero T
	serviceType := reflect.TypeOf(zero)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.services[serviceType] = &ServiceDescriptor{
		ServiceType: serviceType,
		Factory:     nil,
		Lifetime:    Singleton,
		instance:    instance,
	}

	return nil
}

// Resolve resolves a service by type
func Resolve[T any](c *Container) (T, error) {
	var zero T
	serviceType := reflect.TypeOf(zero)

	result, err := c.resolve(serviceType)
	if err != nil {
		return zero, err
	}

	return result.(T), nil
}

// resolve is the internal resolution logic
func (c *Container) resolve(serviceType reflect.Type) (interface{}, error) {
	// Check for circular dependencies
	c.resolvingMu.Lock()
	if c.resolving[serviceType] {
		c.resolvingMu.Unlock()
		return nil, fmt.Errorf("circular dependency detected for type %v", serviceType)
	}
	c.resolving[serviceType] = true
	c.resolvingMu.Unlock()

	defer func() {
		c.resolvingMu.Lock()
		delete(c.resolving, serviceType)
		c.resolvingMu.Unlock()
	}()

	// Look up service descriptor
	c.mu.RLock()
	descriptor, exists := c.services[serviceType]
	c.mu.RUnlock()

	if !exists {
		// Check parent container for scoped containers
		if c.parent != nil {
			return c.parent.resolve(serviceType)
		}
		return nil, fmt.Errorf("service of type %v not registered", serviceType)
	}

	// Handle pre-existing instance
	if descriptor.instance != nil && descriptor.Lifetime == Singleton {
		descriptor.mu.RLock()
		instance := descriptor.instance
		descriptor.mu.RUnlock()
		return instance, nil
	}

	// For scoped services in parent container, create new instance in scope
	if descriptor.Lifetime == Scoped && c.parent == nil {
		return nil, errors.New("scoped services can only be resolved from a scope")
	}

	// Check for cached scoped instance
	if descriptor.Lifetime == Scoped {
		descriptor.mu.RLock()
		if descriptor.instance != nil {
			instance := descriptor.instance
			descriptor.mu.RUnlock()
			return instance, nil
		}
		descriptor.mu.RUnlock()
	}

	// Create new instance using factory
	if descriptor.Factory == nil {
		return nil, fmt.Errorf("no factory for service type %v", serviceType)
	}

	factoryValue := reflect.ValueOf(descriptor.Factory)
	factoryType := factoryValue.Type()

	// Resolve dependencies
	args := make([]reflect.Value, factoryType.NumIn())
	for i := 0; i < factoryType.NumIn(); i++ {
		paramType := factoryType.In(i)

		// Special handling for *Container parameter
		if paramType == reflect.TypeOf((*Container)(nil)) {
			args[i] = reflect.ValueOf(c)
			continue
		}

		dep, err := c.resolve(paramType)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve dependency %v: %w", paramType, err)
		}
		args[i] = reflect.ValueOf(dep)
	}

	// Call factory
	results := factoryValue.Call(args)

	// Handle error return
	if len(results) > 1 && !results[1].IsNil() {
		return nil, results[1].Interface().(error)
	}

	instance := results[0].Interface()

	// Cache instance if singleton or scoped
	if descriptor.Lifetime == Singleton || descriptor.Lifetime == Scoped {
		descriptor.mu.Lock()
		descriptor.instance = instance
		descriptor.mu.Unlock()
	}

	return instance, nil
}

// MustResolve resolves a service or panics
func MustResolve[T any](c *Container) T {
	result, err := Resolve[T](c)
	if err != nil {
		panic(err)
	}
	return result
}

// Has checks if a service type is registered
func Has[T any](c *Container) bool {
	var zero T
	serviceType := reflect.TypeOf(zero)

	c.mu.RLock()
	defer c.mu.RUnlock()

	_, exists := c.services[serviceType]
	if !exists && c.parent != nil {
		return Has[T](c.parent)
	}
	return exists
}

// Clear removes all registered services
func (c *Container) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.services = make(map[reflect.Type]*ServiceDescriptor)
	c.resolving = make(map[reflect.Type]bool)
}

// ServiceRegistrar provides a fluent API for registration
type ServiceRegistrar[T any] struct {
	container *Container
	lifetime  Lifetime
}

// For creates a new registrar for type T
func For[T any](c *Container) *ServiceRegistrar[T] {
	return &ServiceRegistrar[T]{
		container: c,
		lifetime:  Transient,
	}
}

// AsTransient sets lifetime to transient
func (r *ServiceRegistrar[T]) AsTransient() *ServiceRegistrar[T] {
	r.lifetime = Transient
	return r
}

// AsSingleton sets lifetime to singleton
func (r *ServiceRegistrar[T]) AsSingleton() *ServiceRegistrar[T] {
	r.lifetime = Singleton
	return r
}

// AsScoped sets lifetime to scoped
func (r *ServiceRegistrar[T]) AsScoped() *ServiceRegistrar[T] {
	r.lifetime = Scoped
	return r
}

// Use registers the factory
func (r *ServiceRegistrar[T]) Use(factory interface{}) error {
	return Register[T](r.container, r.lifetime, factory)
}

// UseInstance registers an existing instance
func (r *ServiceRegistrar[T]) UseInstance(instance T) error {
	return RegisterInstance[T](r.container, instance)
}

// Example usage and tests
/*
// Service interfaces
type Logger interface {
	Log(message string)
}

type Database interface {
	Query(sql string) ([]map[string]interface{}, error)
}

type UserService interface {
	GetUser(id int) (User, error)
}

// Implementations
type ConsoleLogger struct{}

func (l *ConsoleLogger) Log(message string) {
	fmt.Println("[LOG]", message)
}

type MockDatabase struct {
	logger Logger
}

func NewMockDatabase(logger Logger) *MockDatabase {
	return &MockDatabase{logger: logger}
}

func (db *MockDatabase) Query(sql string) ([]map[string]interface{}, error) {
	db.logger.Log(fmt.Sprintf("Executing query: %s", sql))
	return []map[string]interface{}{}, nil
}

type DefaultUserService struct {
	db     Database
	logger Logger
}

func NewUserService(db Database, logger Logger) *DefaultUserService {
	return &DefaultUserService{db: db, logger: logger}
}

func (s *DefaultUserService) GetUser(id int) (User, error) {
	s.logger.Log(fmt.Sprintf("Getting user %d", id))
	// Implementation
	return User{}, nil
}

// Usage
func main() {
	container := New()

	// Register services using fluent API
	For[Logger](container).AsSingleton().Use(func() Logger {
		return &ConsoleLogger{}
	})

	For[Database](container).AsSingleton().Use(func(logger Logger) Database {
		return NewMockDatabase(logger)
	})

	For[UserService](container).AsTransient().Use(func(db Database, logger Logger) UserService {
		return NewUserService(db, logger)
	})

	// Resolve services
	userService, err := Resolve[UserService](container)
	if err != nil {
		panic(err)
	}

	// Use the service
	user, _ := userService.GetUser(123)
	fmt.Println(user)

	// Create a scope
	scope := container.CreateScope()

	// Register scoped service
	For[RequestContext](scope).AsScoped().Use(func() *RequestContext {
		return &RequestContext{ID: generateRequestID()}
	})

	// Resolve in scope
	ctx, _ := Resolve[RequestContext](scope)
	fmt.Println(ctx.ID)
}
*/
