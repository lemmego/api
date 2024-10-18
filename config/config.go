package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	_ "github.com/joho/godotenv/autoload"
	"log/slog"
)

type M map[string]interface{}

// Config represents a configuration map that can be nested
type Config struct {
	mu sync.RWMutex
	m  M
}

// NewConfig returns a new instance of Config
func NewConfig() *Config {
	return &Config{
		m: make(M),
	}
}

// SetConfig sets the config map if none available, replaces otherwise.
func (c *Config) SetConfigMap(cm M) *Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	slog.Info("Setting config map", "config", cm)
	c.m = cm
	return c
}

// Set sets a configuration value
func (c *Config) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := strings.Split(key, ".")
	c.setRecursive(keys, value)
}

func (c *Config) setRecursive(keys []string, value interface{}) {
	if len(keys) == 1 {
		// Handle the final key setting or merging
		existing, exists := c.m[keys[0]]
		if exists {
			if existingMap, ok := existing.(map[string]interface{}); ok {
				if newValueMap, ok := value.(map[string]interface{}); ok {
					// Merge new values into existing map
					for k, v := range newValueMap {
						existingMap[k] = v
					}
					return // We've merged, no need to set again
				}
			}
			if configM, ok := existing.(M); ok {
				if newValueMap, ok := value.(M); ok {
					for k, v := range newValueMap {
						configM[k] = v
					}
					return
				}
			}
		}
		// If we're here, either it didn't exist, wasn't a map, or couldn't merge, so set directly
		c.m[keys[0]] = value
	} else {
		var current map[string]interface{}
		next, exists := c.m[keys[0]]
		if !exists || next == nil {
			current = make(map[string]interface{})
			c.m[keys[0]] = current
		} else {
			var ok bool
			if current, ok = next.(map[string]interface{}); !ok {
				if current, ok = next.(M); !ok {
					// If it's neither, we'll overwrite it with a new map
					current = make(map[string]interface{})
					c.m[keys[0]] = current
				}
			}
		}
		subConfig := &Config{m: current}
		subConfig.setRecursive(keys[1:], value)
	}
}

// Get retrieves a configuration value with type assertion. The fallback is optional.
func (c *Config) Get(key string, fallback ...interface{}) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := strings.Split(key, ".")
	var current interface{} = c.m
	for _, k := range keys {
		switch v := current.(type) {
		case map[string]interface{}:
			if val, exists := v[k]; exists {
				current = val
			} else {
				if len(fallback) == 0 {
					return nil
				}
				return fallback[0]
			}
		case M:
			if val, exists := v[k]; exists {
				current = val
			} else {
				if len(fallback) == 0 {
					return nil
				}
				return fallback[0]
			}
		default:
			if len(fallback) == 0 {
				return nil
			}
			return fallback[0]
		}
	}
	return current
}

// GetAll returns all configurations
func (c *Config) GetAll() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return deepCopy(c.m)
}

// deepCopy creates a deep copy of the map to prevent external modifications
func deepCopy(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range in {
		if vm, ok := v.(map[string]interface{}); ok {
			out[k] = deepCopy(vm)
		} else {
			out[k] = v
		}
	}
	return out
}

// MustEnv is similar to the previous implementation but adjusted for no generics
func MustEnv[T any](key string, fallback T) T {
	value, exists := os.LookupEnv(key)
	if !exists {
		slog.Info(fmt.Sprintf("Using fallback value for key: %s", key), "fallback", fallback)
		return fallback
	}

	var result T
	var err error

	// Type switch for conversion
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
