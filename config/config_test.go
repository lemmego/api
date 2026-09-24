package config

import (
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

func TestBasicSetAndGet(t *testing.T) {
	c := newConfig()

	c.Set("key1", "value1")
	result := c.Get("key1")

	if result != "value1" {
		t.Errorf("Expected 'value1', got %v", result)
	}
}

func TestNestedSetAndGet(t *testing.T) {
	c := newConfig()

	c.Set("database.host", "localhost")
	c.Set("database.port", 5432)

	host := c.Get("database.host")
	port := c.Get("database.port")

	if host != "localhost" {
		t.Errorf("Expected 'localhost', got %v", host)
	}
	if port != 5432 {
		t.Errorf("Expected 5432, got %v", port)
	}
}

func TestGetWithFallback(t *testing.T) {
	c := newConfig()

	result := c.Get("nonexistent", "default")

	if result != "default" {
		t.Errorf("Expected 'default', got %v", result)
	}
}

func TestSetConfigMap(t *testing.T) {
	c := newConfig()

	configMap := M{
		"app": M{
			"name": "test-app",
			"port": 8080,
		},
		"debug": true,
	}

	c.SetConfigMap(configMap)

	appName := c.Get("app.name")
	appPort := c.Get("app.port")
	debug := c.Get("debug")

	if appName != "test-app" {
		t.Errorf("Expected 'test-app', got %v", appName)
	}
	if appPort != 8080 {
		t.Errorf("Expected 8080, got %v", appPort)
	}
	if debug != true {
		t.Errorf("Expected true, got %v", debug)
	}
}

func TestSetConfigMapDoesNotAliasInput(t *testing.T) {
	c := newConfig()
	input := M{"nested": M{"value": "original"}}

	c.SetConfigMap(input)
	input["nested"].(M)["value"] = "changed"

	if got := c.Get("nested.value"); got != "original" {
		t.Fatalf("SetConfigMap aliased nested input: got %v", got)
	}
}

func TestNestedSetReplacesNonMapIntermediate(t *testing.T) {
	c := newConfig()
	c.Set("database", "not a map")

	c.Set("database.host", "localhost")

	if got := c.Get("database.host"); got != "localhost" {
		t.Fatalf("expected nested Set to replace non-map intermediate, got %v", got)
	}
}

func TestLookupIsSafeAndTyped(t *testing.T) {
	m := M{"database": M{"port": 5432}, "value": "text"}

	port, ok := Lookup[int](m, "database.port")
	if !ok || port != 5432 {
		t.Fatalf("expected typed lookup to find port, got %d, %v", port, ok)
	}
	if _, ok := Lookup[int](m, "value"); ok {
		t.Fatal("typed lookup should reject a mismatched value")
	}
	if _, ok := m.Lookup("value.child"); ok {
		t.Fatal("lookup should reject traversal through a non-map")
	}
	if port, ok := m.LookupAs[int]("database.port"); !ok || port != 5432 {
		t.Fatalf("expected receiver lookup to find port, got %d, %v", port, ok)
	}
}

func TestGetAll(t *testing.T) {
	c := newConfig()

	c.Set("key1", "value1")
	c.Set("nested.key", "nested_value")

	all := c.GetAll()

	if all["key1"] != "value1" {
		t.Errorf("Expected 'value1', got %v", all["key1"])
	}

	nested, ok := all["nested"].(M)
	if !ok {
		t.Errorf("Expected nested to be of type M")
	}
	if nested["key"] != "nested_value" {
		t.Errorf("Expected 'nested_value', got %v", nested["key"])
	}
}

func TestMString(t *testing.T) {
	m := M{"key": "value"}
	if m.String("key", "") != "value" {
		t.Errorf("expected value, got %s", m.String("key", ""))
	}
	if m.String("missing", "fallback") != "fallback" {
		t.Errorf("expected fallback, got %s", m.String("missing", "fallback"))
	}
}

func TestMInt(t *testing.T) {
	m := M{"count": 42}
	if m.Int("count", 0) != 42 {
		t.Errorf("expected 42, got %d", m.Int("count", 0))
	}
	if m.Int("missing", 99) != 99 {
		t.Errorf("expected 99, got %d", m.Int("missing", 99))
	}
}

func TestMBool(t *testing.T) {
	m := M{"enabled": true}
	if !m.Bool("enabled", false) {
		t.Errorf("expected true")
	}
	if m.Bool("missing", true) != true {
		t.Errorf("expected true fallback")
	}
}

func TestMFloat64(t *testing.T) {
	m := M{"pi": 3.14}
	if m.Float64("pi", 0) != 3.14 {
		t.Errorf("expected 3.14, got %f", m.Float64("pi", 0))
	}
}

func TestMDuration(t *testing.T) {
	m := M{"timeout": 5 * time.Second}
	d := m.Duration("timeout", 0)
	if d != 5*time.Second {
		t.Errorf("expected 5s, got %v", d)
	}
	if m.Duration("missing", 10*time.Second) != 10*time.Second {
		t.Errorf("expected 10s fallback")
	}
}

func TestGetAllReturnsDeepCopy(t *testing.T) {
	c := newConfig()
	c.Set("test", "original")

	all1 := c.GetAll()
	all2 := c.GetAll()

	all1["test"] = "modified"

	if all2["test"] != "original" {
		t.Errorf("Deep copy failed: all2 was modified when all1 was changed")
	}

	original := c.Get("test")
	if original != "original" {
		t.Errorf("Original config was modified when copy was changed")
	}
}

func TestSingletonPattern(t *testing.T) {
	instance1 := GetInstance()
	instance2 := GetInstance()

	if instance1 != instance2 {
		t.Errorf("GetInstance() should return the same instance")
	}
}

func TestNewReturnsIsolatedConfiguration(t *testing.T) {
	first := New()
	second := New()
	first.Set("app.name", "first")

	if second.Get("app.name") != nil {
		t.Fatal("New configurations should not share values")
	}
}

func TestGlobalFunctions(t *testing.T) {
	Set("global.test", "global_value")
	result := Get("global.test")

	if result != "global_value" {
		t.Errorf("Expected 'global_value', got %v", result)
	}

	all := GetAll()
	if all["global"].(M)["test"] != "global_value" {
		t.Errorf("GetAll() should include globally set values")
	}
}

func TestConcurrentReads(t *testing.T) {
	c := newConfig()
	c.Set("concurrent.test", "test_value")

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				result := c.Get("concurrent.test")
				if result != "test_value" {
					errors <- nil
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	if len(errors) > 0 {
		t.Errorf("Concurrent reads failed")
	}
}

func TestConcurrentWrites(t *testing.T) {
	c := newConfig()

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				c.Set("concurrent.write", id*10+j)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	wg.Wait()

	result := c.Get("concurrent.write")
	if result == nil {
		t.Errorf("Concurrent writes should result in some value being set")
	}
}

func TestConcurrentReadWrites(t *testing.T) {
	c := newConfig()
	c.Set("readwrite.test", "initial")

	var wg sync.WaitGroup
	errors := make(chan error, 50)

	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				c.Set("readwrite.test", id)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				result := c.Get("readwrite.test")
				if result == nil {
					errors <- nil
				}
				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait()
	close(errors)

	if len(errors) > 0 {
		t.Errorf("Concurrent read/write operations failed")
	}
}

func TestConcurrentSetConfigMapAndGetAll(t *testing.T) {
	c := newConfig()
	var wg sync.WaitGroup

	for i := 0; i < 25; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			c.SetConfigMap(M{"worker": M{"id": i}})
		}(i)
		go func() {
			defer wg.Done()
			_ = c.GetAll()
		}()
	}

	wg.Wait()
	if _, ok := c.Get("worker.id").(int); !ok {
		t.Fatal("concurrent SetConfigMap/GetAll lost the configured value")
	}
}

func TestMustEnvString(t *testing.T) {
	os.Setenv("TEST_STRING", "test_value")
	defer os.Unsetenv("TEST_STRING")

	result := MustEnv("TEST_STRING", "default")
	if result != "test_value" {
		t.Errorf("Expected 'test_value', got %v", result)
	}

	defaultResult := MustEnv("NONEXISTENT_STRING", "default")
	if defaultResult != "default" {
		t.Errorf("Expected 'default', got %v", defaultResult)
	}
}

func TestMustEnvInt(t *testing.T) {
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")

	result := MustEnv("TEST_INT", 0)
	if result != 42 {
		t.Errorf("Expected 42, got %v", result)
	}

	defaultResult := MustEnv("NONEXISTENT_INT", 99)
	if defaultResult != 99 {
		t.Errorf("Expected 99, got %v", defaultResult)
	}
}

func TestMustEnvFloat(t *testing.T) {
	os.Setenv("TEST_FLOAT", "3.14")
	defer os.Unsetenv("TEST_FLOAT")

	result := MustEnv("TEST_FLOAT", 0.0)
	if result != 3.14 {
		t.Errorf("Expected 3.14, got %v", result)
	}
}

func TestMustEnvBool(t *testing.T) {
	os.Setenv("TEST_BOOL", "true")
	defer os.Unsetenv("TEST_BOOL")

	result := MustEnv("TEST_BOOL", false)
	if result != true {
		t.Errorf("Expected true, got %v", result)
	}
}

func TestMustEnvDuration(t *testing.T) {
	t.Setenv("TEST_DURATION", "90m")

	if got := MustEnv("TEST_DURATION", 0*time.Second); got != 90*time.Minute {
		t.Fatalf("expected 90m, got %v", got)
	}
}

func TestMustEnvSameSite(t *testing.T) {
	t.Setenv("TEST_SAME_SITE", "strict")

	if got := MustEnv("TEST_SAME_SITE", http.SameSiteLaxMode); got != http.SameSiteStrictMode {
		t.Fatalf("expected SameSiteStrictMode, got %v", got)
	}
}

func TestMustEnvPanicOnInvalidInt(t *testing.T) {
	os.Setenv("INVALID_INT", "not_a_number")
	defer os.Unsetenv("INVALID_INT")

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for invalid int conversion")
		}
	}()

	MustEnv("INVALID_INT", 0)
}

func TestMustEnvPanicOnInvalidFloat(t *testing.T) {
	os.Setenv("INVALID_FLOAT", "not_a_float")
	defer os.Unsetenv("INVALID_FLOAT")

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for invalid float conversion")
		}
	}()

	MustEnv("INVALID_FLOAT", 0.0)
}

func TestMustEnvPanicOnInvalidBool(t *testing.T) {
	os.Setenv("INVALID_BOOL", "not_a_bool")
	defer os.Unsetenv("INVALID_BOOL")

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for invalid bool conversion")
		}
	}()

	MustEnv("INVALID_BOOL", false)
}

func TestSingletonConcurrency(t *testing.T) {
	var wg sync.WaitGroup
	instances := make(chan Configuration, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			instances <- GetInstance()
		}()
	}

	wg.Wait()
	close(instances)

	first := <-instances
	for instance := range instances {
		if instance != first {
			t.Errorf("Singleton pattern failed under concurrent access")
			break
		}
	}
}

func BenchmarkGet(b *testing.B) {
	c := newConfig()
	c.Set("benchmark.key", "value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get("benchmark.key")
	}
}

func BenchmarkSet(b *testing.B) {
	c := newConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set("benchmark.key", i)
	}
}

func BenchmarkConcurrentGet(b *testing.B) {
	c := newConfig()
	c.Set("benchmark.concurrent", "value")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Get("benchmark.concurrent")
		}
	})
}
