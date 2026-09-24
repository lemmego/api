// Package config provides configuration management for the Lemmego framework.
//
// It supports environment-based configuration with .env file loading,
// type-safe access methods, default values, and nested configuration access
// using dot notation. The configuration system automatically loads .env files
// and provides convenient methods for accessing configuration values.
package config

import (
	"fmt"
	"net/http"
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
	}
	return defaultVal[0]
}

func (m M) Int(key string, defaultVal ...int) int {
	if val, ok := m[key].(int); ok {
		return val
	}
	return defaultVal[0]
}

func (m M) Int64(key string, defaultVal ...int64) int64 {
	if val, ok := m[key].(int64); ok {
		return val
	}
	return defaultVal[0]
}

func (m M) Bool(key string, defaultVal ...bool) bool {
	if val, ok := m[key].(bool); ok {
		return val
	}
	return defaultVal[0]
}

func (m M) Float64(key string, defaultVal ...float64) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	}
	return defaultVal[0]
}

func (m M) Duration(key string, defaultVal ...time.Duration) time.Duration {
	if val, ok := m[key].(time.Duration); ok {
		return val
	}
	return defaultVal[0]
}

func (m M) Time(key string, defaultVal ...time.Time) time.Time {
	if val, ok := m[key].(time.Time); ok {
		return val
	}
	return defaultVal[0]
}

// Lookup returns a value at key, traversing nested M and map[string]any values.
// It never panics when a path is missing or encounters a non-map value.
func (m M) Lookup(key string) (any, bool) {
	return lookup(m, strings.Split(key, "."))
}

// Lookup returns a typed value at key without panicking on missing or mismatched values.
func Lookup[T any](m M, key string) (T, bool) {
	value, ok := m.Lookup(key)
	if !ok {
		var zero T
		return zero, false
	}
	result, ok := value.(T)
	return result, ok
}

// config represents a nested configuration map with thread-safe operations
type config struct {
	mu sync.RWMutex
	m  M
}

// newConfig initializes and returns a new config instance.
func newConfig() *config {
	return &config{m: make(M)}
}

// New returns an isolated configuration instance.
func New() Configuration {
	return newConfig()
}

var (
	instance *config
	once     sync.Once
)

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
	c.m = deepCopy(cm)
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
		var subConfig M
		switch current := c.m[keys[0]].(type) {
		case M:
			subConfig = current
		case map[string]any:
			subConfig = M(current)
			c.m[keys[0]] = subConfig
		default:
			c.m[keys[0]] = make(M)
			subConfig = c.m[keys[0]].(M)
		}
		(&config{m: subConfig}).setRecursive(keys[1:], value, depth+1)
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
	return lookup(current, keys)
}

func lookup(current map[string]any, keys []string) (any, bool) {
	if len(keys) == 0 {
		return nil, false
	}
	value, ok := current[keys[0]]
	if !ok || len(keys) == 1 {
		return value, ok
	}
	switch next := value.(type) {
	case M:
		return lookup(next, keys[1:])
	case map[string]any:
		return lookup(next, keys[1:])
	default:
		return nil, false
	}
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
		out[k] = deepCopyValue(v)
	}
	return out
}

func deepCopyValue(value any) any {
	switch value := value.(type) {
	case M:
		return deepCopy(value)
	case map[string]any:
		return deepCopy(M(value))
	case []any:
		copy := make([]any, len(value))
		for i, item := range value {
			copy[i] = deepCopyValue(item)
		}
		return copy
	default:
		return value
	}
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
	case time.Duration:
		var d time.Duration
		d, err = time.ParseDuration(value)
		result = any(d).(T)
	case http.SameSite:
		var sameSite http.SameSite
		sameSite, err = parseSameSite(value)
		result = any(sameSite).(T)
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

func parseSameSite(value string) (http.SameSite, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "default", "0", "1":
		return http.SameSiteDefaultMode, nil
	case "lax", "2":
		return http.SameSiteLaxMode, nil
	case "strict", "3":
		return http.SameSiteStrictMode, nil
	case "none", "4":
		return http.SameSiteNoneMode, nil
	default:
		return http.SameSiteDefaultMode, fmt.Errorf("invalid SameSite value %q", value)
	}
}

// Set sets a configuration value in the singleton instance
func Set(key string, value any) {
	GetInstance().Set(key, value)
}

// Get retrieves a configuration value from the singleton instance
func Get(key string, fallback ...any) any {
	return GetInstance().Get(key, fallback...)
}

// GetAll returns all configurations from the singleton instance
func GetAll() M {
	return GetInstance().GetAll()
}

type Configuration interface {
	SetConfigMap(cm M) *config
	Set(key string, value any)
	Get(key string, fallback ...any) any
	GetAll() M
}
