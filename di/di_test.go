package di

import (
	"testing"
)

type testService struct {
	Value string
}

type testRepo struct {
	Svc *testService `di:"inject"`
}

type testInterface interface {
	Name() string
}

type testImplementation struct{}

func (*testImplementation) Name() string { return "implementation" }

func TestRegisterAndResolveSingleton(t *testing.T) {
	c := New()
	err := RegisterSingleton[*testService](c, func() *testService {
		return &testService{Value: "singleton"}
	})
	if err != nil {
		t.Fatal(err)
	}

	svc, err := Resolve[*testService](c)
	if err != nil {
		t.Fatal(err)
	}
	if svc.Value != "singleton" {
		t.Errorf("expected 'singleton', got %s", svc.Value)
	}
}

func TestSingletonReturnsSameInstance(t *testing.T) {
	c := New()
	RegisterSingleton[*testService](c, func() *testService {
		return &testService{Value: "shared"}
	})

	a, _ := Resolve[*testService](c)
	b, _ := Resolve[*testService](c)

	if a != b {
		t.Error("singleton should return the same instance")
	}
}

func TestTransientReturnsNewInstance(t *testing.T) {
	c := New()
	err := RegisterTransient[*testService](c, func() *testService {
		return &testService{Value: "transient"}
	})
	if err != nil {
		t.Fatal(err)
	}

	a, _ := Resolve[*testService](c)
	b, _ := Resolve[*testService](c)

	if a == b {
		t.Error("transient should return a new instance each time")
	}
}

func TestHas(t *testing.T) {
	c := New()
	if Has[*testService](c) {
		t.Error("Has should return false before registration")
	}

	RegisterSingleton[*testService](c, func() *testService {
		return &testService{}
	})
	if !Has[*testService](c) {
		t.Error("Has should return true after registration")
	}
}

func TestResolveUnregistered(t *testing.T) {
	c := New()
	_, err := Resolve[*testService](c)
	if err == nil {
		t.Error("expected error for unregistered type")
	}
}

func TestMustResolvePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for missing type")
		}
	}()
	c := New()
	MustResolve[*testService](c)
}

func TestRegisterInstance(t *testing.T) {
	c := New()
	svc := &testService{Value: "instance"}
	err := RegisterInstance(c, svc)
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := Resolve[*testService](c)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != svc {
		t.Error("RegisterInstance should return the same instance")
	}
}

func TestCreateScope(t *testing.T) {
	parent := New()
	RegisterSingleton[*testService](parent, func() *testService {
		return &testService{Value: "parent"}
	})

	child := parent.CreateScope()
	if child == parent {
		t.Error("scoped container should be different from parent")
	}

	svc, err := Resolve[*testService](child)
	if err != nil {
		t.Fatal(err)
	}
	if svc.Value != "parent" {
		t.Errorf("expected 'parent', got %s", svc.Value)
	}
}

func TestFluentRegistrar(t *testing.T) {
	c := New()
	err := For[*testService](c).AsSingleton().Use(func() *testService {
		return &testService{Value: "fluent"}
	})
	if err != nil {
		t.Fatal(err)
	}

	svc, _ := Resolve[*testService](c)
	if svc.Value != "fluent" {
		t.Errorf("expected 'fluent', got %s", svc.Value)
	}
}

func TestFluentUseInstance(t *testing.T) {
	c := New()
	svc := &testService{Value: "direct"}
	err := For[*testService](c).AsSingleton().UseInstance(svc)
	if err != nil {
		t.Fatal(err)
	}

	resolved, _ := Resolve[*testService](c)
	if resolved != svc {
		t.Error("UseInstance should return the same instance")
	}
}

func TestClear(t *testing.T) {
	c := New()
	RegisterSingleton[*testService](c, func() *testService { return &testService{} })
	c.Clear()
	if Has[*testService](c) {
		t.Error("after Clear, Has should return false")
	}
}

func TestGenericContainerMethods(t *testing.T) {
	c := New()
	if err := c.RegisterInstance("value"); err != nil {
		t.Fatalf("register instance: %v", err)
	}

	value, err := c.Resolve[string]()
	if err != nil || value != "value" {
		t.Fatalf("resolve via method = %q, %v", value, err)
	}
	if !c.Has[string]() {
		t.Fatal("Has via method returned false")
	}
	if c.MustResolve[string]() != "value" {
		t.Fatal("MustResolve via method returned the wrong value")
	}
	if err := c.For[int]().AsSingleton().UseInstance(42); err != nil {
		t.Fatalf("register via method: %v", err)
	}
	if c.MustResolve[int]() != 42 {
		t.Fatal("For via method did not register the instance")
	}
	if err := c.RegisterSingleton[*testService](func() *testService {
		return &testService{Value: "registered"}
	}); err != nil {
		t.Fatalf("RegisterSingleton via method: %v", err)
	}
	if c.MustResolve[*testService]().Value != "registered" {
		t.Fatal("RegisterSingleton via method did not register the service")
	}
}

func TestGenericMethodsSupportInterfaceTypes(t *testing.T) {
	c := New()
	if err := c.RegisterInstance[testInterface](&testImplementation{}); err != nil {
		t.Fatalf("register interface: %v", err)
	}
	value, err := c.Resolve[testInterface]()
	if err != nil {
		t.Fatalf("resolve interface: %v", err)
	}
	if value.Name() != "implementation" {
		t.Fatalf("unexpected interface value: %s", value.Name())
	}
}

func TestScopedServicesAreIsolatedPerScope(t *testing.T) {
	root := New()
	if err := root.RegisterScoped[*testService](func() *testService {
		return &testService{}
	}); err != nil {
		t.Fatal(err)
	}

	first := root.CreateScope()
	second := root.CreateScope()
	firstValue := first.MustResolve[*testService]()
	if firstValue != first.MustResolve[*testService]() {
		t.Fatal("scoped service should be reused within a scope")
	}
	if firstValue == second.MustResolve[*testService]() {
		t.Fatal("scoped service should not be shared across scopes")
	}
}
