// Package config provides configuration management for the Lemmego framework.
//
// It supports environment-based configuration with .env file loading,
// type-safe access methods, default values, and nested configuration access
// using dot notation. The configuration system automatically loads .env files
// and provides convenient methods for accessing configuration values.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/joho/godotenv/autoload"
)

// M is a type alias for a map of string to any that provides
// type-safe accessor methods for configuration values with default value support.
type M map[string]any

func (m M) String(key string, defaultVal ...string) string {
	if val, ok := m[key].(string); ok {
		return val
	} else {
		return defaultVal[0]
	}
}

func (m M) Int(key string, defaultVal ...int) int {
	if val, ok := m[key].(int); ok {
		return val
	} else {
		return defaultVal[0]
	}
}

func (m M) Int64(key string, defaultVal ...int64) int64 {
	if val, ok := m[key].(int64); ok {
		return val
	} else {
		return defaultVal[0]
	}
}

func (m M) Bool(key string, defaultVal ...bool) bool {
	if val, ok := m[key].(bool); ok {
		return val
	} else {
		return defaultVal[0]
	}
}

func (m M) Float64(key string, defaultVal ...float64) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	} else {
		return defaultVal[0]
	}
}

func (m M) Duration(key string, defaultVal ...time.Duration) time.Duration {
	if val, ok := m[key].(time.Duration); ok {
		return val
	} else {
		return defaultVal[0]
	}
}

func (m M) Time(key string, defaultVal ...time.Time) time.Time {
	if val, ok := m[key].(time.Time); ok {
		return val
	} else {
		return defaultVal[0]
	}
}

// config represents a nested configuration map with thread-safe operations
type config struct {
	mu sync.RWMutex
	m  M
}

// newConfig initializes and returns a new config instance (private)
func newConfig() *config {
	return &config{m: make(M)}
}

var (
	instance *config
	once     sync.Once
)

func init() {
	_ = GetInstance()
}

// GetInstance returns the singleton instance of config
func GetInstance() Configuration {
	once.Do(func() {
		instance = newConfig()
	})
	return instance
}

// SetConfigMap sets or replaces the entire configuration map
func (c *config) SetConfigMap(cm M) *config {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = cm
	return c
}

// Set sets a configuration value, supporting nested keys
func (c *config) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := strings.Split(key, ".")
	c.setRecursive(keys, value, 0)
}

func (c *config) setRecursive(keys []string, value any, depth int) {
	if len(keys) == 1 {
		c.m[keys[0]] = value
	} else {
		if _, exists := c.m[keys[0]]; !exists {
			c.m[keys[0]] = make(M)
		}
		subConfig := &config{m: c.m[keys[0]].(M)}
		subConfig.setRecursive(keys[1:], value, depth+1)
	}
}

// Get retrieves a configuration value with optional fallback
func (c *config) Get(key string, fallback ...any) any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	value, _ := c.getRecursive(strings.Split(key, "."), c.m)
	if value == nil && len(fallback) > 0 {
		return fallback[0]
	}
	return value
}

func (c *config) getRecursive(keys []string, current map[string]any) (any, bool) {
	if len(keys) == 1 {
		v, ok := current[keys[0]]
		return v, ok
	}
	if next, ok := current[keys[0]].(map[string]any); ok {
		return c.getRecursive(keys[1:], next)
	}
	if next, ok := current[keys[0]].(M); ok {
		// If it's of type M, we need to convert it to map[string]any
		return c.getRecursive(keys[1:], map[string]any(next))
	}
	return nil, false
}

// GetAll returns a deep copy of all configurations
func (c *config) GetAll() M {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return deepCopy(c.m)
}

// deepCopy creates a deep copy of the configuration map
func deepCopy(in M) M {
	out := make(M)
	for k, v := range in {
		switch v := v.(type) {
		case map[string]any:
			out[k] = deepCopy(v)
		case M:
			out[k] = deepCopy(v)
		default:
			out[k] = v
		}
	}
	return out
}

// MustEnv retrieves an environment variable and converts it to the specified type or panics on failure
func MustEnv[T any](key string, fallback T) T {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}

	var result T
	var err error

	switch any(fallback).(type) {
	case int:
		var i int
		i, err = strconv.Atoi(value)
		result = any(i).(T)
	case float64:
		var f float64
		f, err = strconv.ParseFloat(value, 64)
		result = any(f).(T)
	case bool:
		var b bool
		b, err = strconv.ParseBool(value)
		result = any(b).(T)
	case string:
		result = any(value).(T)
	default:
		panic(fmt.Sprintf("unsupported type for environment variable %s", key))
	}

	if err != nil {
		panic(err)
	}

	return result
}

// Set sets a configuration value in the singleton instance
func Set(key string, value any) {
	instance.Set(key, value)
}

// Get retrieves a configuration value from the singleton instance
func Get(key string, fallback ...any) any {
	return instance.Get(key, fallback...)
}

// GetAll returns all configurations from the singleton instance
func GetAll() M {
	return instance.GetAll()
}

type Configuration interface {
	SetConfigMap(cm M) *config
	Set(key string, value any)
	Get(key string, fallback ...any) any
	GetAll() M
}
