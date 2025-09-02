package app

import (
	"fmt"
	"reflect"
	"sync"
)

// serviceContainer holds all the application's dependencies
type serviceContainer struct {
	services map[reflect.Type]interface{}
	mutex    sync.RWMutex
}

// newServiceContainer creates a new serviceContainer
func newServiceContainer() *serviceContainer {
	return &serviceContainer{
		services: make(map[reflect.Type]interface{}),
	}
}

// Add adds a service to the container
func (sc *serviceContainer) Add(service interface{}) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	t := reflect.TypeOf(service)
	// Store by pointer type if it's a pointer, otherwise by value type
	if t.Kind() == reflect.Ptr {
		sc.services[t] = service
	} else {
		sc.services[reflect.PointerTo(t)] = service
	}
}

// Get retrieves a service from the container and populates the provided pointer or pointer to pointer
func (sc *serviceContainer) Get(service interface{}) error {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()

	// Check if the service is a pointer or pointer to pointer
	ptrType := reflect.TypeOf(service)
	if ptrType.Kind() != reflect.Ptr {
		return fmt.Errorf("service must be a pointer or pointer to pointer")
	}

	// If we have a pointer to a pointer, get the element type
	serviceType := ptrType
	if ptrType.Elem().Kind() == reflect.Ptr {
		serviceType = ptrType.Elem()
	}

	// Retrieve the service from the container
	if svc, exists := sc.services[serviceType]; exists {
		// If we're dealing with a pointer to pointer, set the value directly
		if ptrType.Elem().Kind() == reflect.Ptr {
			reflect.ValueOf(service).Elem().Set(reflect.ValueOf(svc))
		} else {
			// If we're dealing with a pointer, set the value it points to
			reflect.ValueOf(service).Elem().Set(reflect.ValueOf(svc).Elem())
		}
		return nil
	}

	return ErrServiceNotFound
}
