package config

import (
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

type ConfigMap map[string]interface{}

var (
	Conf ConfigMap = make(ConfigMap)
	mu   sync.RWMutex
)

func init() {
	if err := godotenv.Load(); err != nil {
		panic(err)
	}
}

func Set(key string, value interface{}) {
	mu.Lock()
	defer mu.Unlock()

	keys := strings.Split(key, ".")
	setRecursive(Conf, keys, value)
}

func setRecursive(current ConfigMap, keys []string, value interface{}) {
	if len(keys) == 1 {
		if nestedMap, ok := value.(map[string]interface{}); ok {
			// If the value is a map, convert it to ConfigMap
			configMap := make(ConfigMap)
			for k, v := range nestedMap {
				setRecursive(configMap, []string{k}, v)
			}
			current[keys[0]] = configMap
		} else {
			current[keys[0]] = value
		}
	} else {
		if _, ok := current[keys[0]]; !ok {
			current[keys[0]] = make(ConfigMap)
		}
		setRecursive(current[keys[0]].(ConfigMap), keys[1:], value)
	}
}

func Get[T any](key string, fallback ...T) T {
	mu.RLock()
	defer mu.RUnlock()

	keys := strings.Split(key, ".")
	current := interface{}(Conf)

	for _, k := range keys {
		if m, ok := current.(ConfigMap); ok {
			if val, exists := m[k]; exists {
				current = val
			} else {
				return getFallbackOrZero(fallback)
			}
		} else {
			return getFallbackOrZero(fallback)
		}
	}

	// Try to convert the final value to type T
	if result, ok := current.(T); ok {
		return result
	}

	// Handle the case where T is map[string]any
	if reflect.TypeOf(*(new(T))) == reflect.TypeOf(map[string]any{}) {
		if m, ok := current.(ConfigMap); ok {
			result := make(map[string]any)
			for k, v := range m {
				result[k] = v
			}
			return any(result).(T)
		}
	}

	// If T is ConfigMap and current is map[string]interface{}, convert it
	if _, ok := interface{}(*(new(T))).(ConfigMap); ok {
		if m, ok := current.(map[string]interface{}); ok {
			return interface{}(ConfigMap(m)).(T)
		}
	}

	return getFallbackOrZero(fallback)
}

func getFallbackOrZero[T any](fallback []T) T {
	if len(fallback) > 0 {
		return fallback[0]
	}
	var zero T
	return zero
}

func GetAll() ConfigMap {
	mu.RLock()
	defer mu.RUnlock()

	return Conf
}

func Reset() {
	mu.Lock()
	defer mu.Unlock()

	Conf = make(ConfigMap)
}

// MustEnv returns the value of the environment variable or panics if the variable is not set or if the type is unsupported.
func MustEnv[T any](key string, fallback T) T {
	value, ok := os.LookupEnv(key)
	if !ok {
		slog.Info(fmt.Sprintf("Using fallback value for key: %s", key), key, fallback)
		return fallback
	}

	fallbackType := reflect.TypeOf(fallback)

	// Check if the fallback type is supported
	switch fallbackType.Kind() {
	case reflect.Int, reflect.Int64:
		result, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			panic(err)
		}
		return reflect.ValueOf(result).Convert(fallbackType).Interface().(T)

	case reflect.Float64:
		result, err := strconv.ParseFloat(value, 64)
		if err != nil {
			panic(err)
		}
		return reflect.ValueOf(result).Convert(fallbackType).Interface().(T)

	case reflect.Bool:
		result, err := strconv.ParseBool(value)
		if err != nil {
			panic(err)
		}
		return reflect.ValueOf(result).Convert(fallbackType).Interface().(T)

	case reflect.String:
		return reflect.ValueOf(value).Convert(fallbackType).Interface().(T)

	default:
		panic(fmt.Sprintf("unsupported type: %v", fallbackType))
	}
}
