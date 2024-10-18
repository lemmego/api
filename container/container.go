package container

import (
	"context"
	"fmt"
	"reflect"
	"sync"
)

// ScopeIDKey is the key used to store the scope ID in the context
var ScopeIDKey = struct{}{}

type Container struct {
	bindings   map[reflect.Type]bindingInfo
	instances  map[reflect.Type]interface{}
	scopes     map[string]map[reflect.Type]interface{}
	scopeMutex sync.RWMutex
}

type ServiceContainer interface {
	Bind(abstract interface{}, concrete interface{})
	Singleton(abstract interface{}, concrete interface{})
	Scoped(abstract interface{}, concrete interface{})
	Resolve(out interface{}) error
	ResolveCtx(ctx context.Context, out interface{}) error
	BeginScope(scopeID string)
	EndScope(scopeID string)
}

type bindingInfo struct {
	resolver  interface{}
	singleton bool
	scoped    bool
}

func NewContainer() *Container {
	return &Container{
		bindings:  make(map[reflect.Type]bindingInfo),
		instances: make(map[reflect.Type]interface{}),
		scopes:    make(map[string]map[reflect.Type]interface{}),
	}
}

func (c *Container) Bind(abstract interface{}, concrete interface{}) {
	c.bind(abstract, concrete, false, false)
}

func (c *Container) Singleton(abstract interface{}, concrete interface{}) {
	c.bind(abstract, concrete, true, false)
}

func (c *Container) Scoped(abstract interface{}, concrete interface{}) {
	c.bind(abstract, concrete, false, true)
}

func (c *Container) Resolve(out interface{}) error {
	return c.resolveInScope(out, "")
}

func (c *Container) ResolveCtx(ctx context.Context, out interface{}) error {
	scopeID, _ := ctx.Value(ScopeIDKey).(string)
	return c.resolveInScope(out, scopeID)
}

func (c *Container) bind(abstract interface{}, concrete interface{}, singleton, scoped bool) {
	abstractType := reflect.TypeOf(abstract)

	// Handle both pointer and non-pointer types for interfaces
	if abstractType == nil {
		// This case handles (InterfaceName)(nil)
		concreteType := reflect.TypeOf(concrete)
		if concreteType.Kind() == reflect.Func {
			returnType := concreteType.Out(0)
			if returnType.Kind() == reflect.Interface {
				abstractType = returnType
			} else if returnType.Kind() == reflect.Ptr {
				abstractType = returnType.Elem()
			} else {
				panic(fmt.Sprintf("Invalid concrete type for nil abstract: %v", concreteType))
			}
		} else {
			panic(fmt.Sprintf("Invalid concrete type for nil abstract: %v", concreteType))
		}
	} else if abstractType.Kind() == reflect.Ptr {
		// This case handles (*InterfaceName)(nil) and (*StructName)(nil)
		abstractType = abstractType.Elem()
	}

	fmt.Printf("Binding: %v\n", abstractType) // Debug log
	c.bindings[abstractType] = bindingInfo{
		resolver:  concrete,
		singleton: singleton,
		scoped:    scoped,
	}
}

func (c *Container) resolveInScope(out interface{}, scopeID string) error {
	outValue := reflect.ValueOf(out)
	if outValue.Kind() != reflect.Ptr {
		return fmt.Errorf("out parameter must be a pointer")
	}

	abstractType := outValue.Type().Elem()
	fmt.Printf("Resolving: %v\n", abstractType) // Debug log

	binding, exists := c.bindings[abstractType]
	if !exists {
		// If the abstractType is a pointer, try to find a binding for its element type
		if abstractType.Kind() == reflect.Ptr {
			binding, exists = c.bindings[abstractType.Elem()]
		}

		// If still not found and it's an interface, look for implementations
		if !exists && abstractType.Kind() == reflect.Interface {
			fmt.Printf("Direct binding not found, searching for implementations...\n") // Debug log
			for boundType, boundBinding := range c.bindings {
				fmt.Printf("Checking: %v\n", boundType) // Debug log
				if boundType.Implements(abstractType) || reflect.PtrTo(boundType).Implements(abstractType) {
					binding = boundBinding
					exists = true
					fmt.Printf("Found implementation: %v\n", boundType) // Debug log
					break
				}
			}
		}
	}

	if !exists {
		return fmt.Errorf("no binding found for %v", abstractType)
	}

	if binding.singleton {
		if instance, ok := c.instances[abstractType]; ok {
			outValue.Elem().Set(reflect.ValueOf(instance))
			return nil
		}
	}

	if binding.scoped {
		c.scopeMutex.RLock()
		scopedInstances, exists := c.scopes[scopeID]
		if exists {
			if instance, ok := scopedInstances[abstractType]; ok {
				c.scopeMutex.RUnlock()
				outValue.Elem().Set(reflect.ValueOf(instance))
				return nil
			}
		}
		c.scopeMutex.RUnlock()
	}

	var instance interface{}

	concreteValue := reflect.ValueOf(binding.resolver)
	if concreteValue.Kind() == reflect.Func {
		results := concreteValue.Call(nil)
		if len(results) != 1 {
			return fmt.Errorf("factory function must return exactly one value")
		}
		instance = results[0].Interface()
	} else if concreteValue.Kind() == reflect.Ptr {
		instance = reflect.New(concreteValue.Type().Elem()).Interface()
	} else {
		return fmt.Errorf("invalid binding for %v", abstractType)
	}

	if binding.singleton {
		c.instances[abstractType] = instance
	} else if binding.scoped {
		c.scopeMutex.Lock()
		if _, exists := c.scopes[scopeID]; !exists {
			c.scopes[scopeID] = make(map[reflect.Type]interface{})
		}
		c.scopes[scopeID][abstractType] = instance
		c.scopeMutex.Unlock()
	}

	instanceValue := reflect.ValueOf(instance)
	if abstractType.Kind() == reflect.Ptr && instanceValue.Kind() != reflect.Ptr {
		// If we're expecting a pointer but instance is not a pointer, get its address
		instancePtr := reflect.New(instanceValue.Type())
		instancePtr.Elem().Set(instanceValue)
		outValue.Elem().Set(instancePtr)
	} else {
		outValue.Elem().Set(instanceValue)
	}

	return nil
}

func (c *Container) BeginScope(scopeID string) {
	c.scopeMutex.Lock()
	defer c.scopeMutex.Unlock()
	c.scopes[scopeID] = make(map[reflect.Type]interface{})
}

func (c *Container) EndScope(scopeID string) {
	c.scopeMutex.Lock()
	defer c.scopeMutex.Unlock()
	delete(c.scopes, scopeID)
}
