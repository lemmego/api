package app

import (
	"context"
	"errors"
	"testing"
)

func TestNewHealthCheckerInitializesDefaults(t *testing.T) {
	h := NewHealthChecker(nil)
	h.RegisterCheck("ok", HealthCheck{CheckFunc: func(context.Context) error { return nil }})
	status := h.Check(nil)
	if status.Status != "healthy" {
		t.Fatalf("status = %q, want healthy", status.Status)
	}
}

func TestZeroValueHealthCheckerIsUsable(t *testing.T) {
	var h HealthChecker
	h.RegisterCheck("missing", HealthCheck{})
	status := h.Check(context.Background())
	if status.Status != "degraded" {
		t.Fatalf("status = %q, want degraded", status.Status)
	}
}

// TestTheHealthCheckerIsOneObject is the regression for a subsystem that was
// inert.
//
// GetHealthChecker built a new HealthChecker on every call, so a provider
// registering a check wrote into a throwaway and the HTTP handler served a
// different, empty one. /health answered "healthy" having checked nothing,
// which is worse than not having the endpoint — it is a monitoring signal
// that cannot fail.
func TestTheHealthCheckerIsOneObject(t *testing.T) {
	application := &application{serviceRegistry: newServiceRegistry()}

	first := application.GetHealthChecker()
	second := application.GetHealthChecker()
	if first != second {
		t.Fatal("GetHealthChecker returned two different checkers, so a registered check is invisible to the handler")
	}

	// A check registered through one accessor is run by another.
	application.GetHealthChecker().RegisterCheck("canary", HealthCheck{
		Name:      "canary",
		Critical:  true,
		CheckFunc: func(context.Context) error { return errors.New("down") },
	})

	status := application.GetHealthChecker().Check(context.Background())
	if _, ok := status.Checks["canary"]; !ok {
		t.Fatalf("the registered check was not run: %+v", status.Checks)
	}
	if status.Status != "unhealthy" {
		t.Errorf("status = %q, want unhealthy", status.Status)
	}
}

// TestGetHealthCheckerIsOnTheInterface: it was only on the concrete type, so
// no provider and no route callback — which both receive App — could reach
// the health subsystem at all.
func TestGetHealthCheckerIsOnTheInterface(t *testing.T) {
	var reachable interface {
		GetHealthChecker() *HealthChecker
	} = &application{serviceRegistry: newServiceRegistry()}

	if reachable.GetHealthChecker() == nil {
		t.Fatal("no checker")
	}

	// And through App itself, which is what a provider is handed.
	var _ interface{ GetHealthChecker() *HealthChecker } = App(nil)
}

// TestCheckSurvivesAnApplicationWithNoConfiguration.
//
// Check reads app.name and app.env for the response envelope. Config()
// returns an interface, so an application built without one returns a nil
// interface and Get panicked — inside Check, which is outside the protection
// runCheck gives a failing check, so the panic reached the HTTP handler. A
// monitoring endpoint that can take the process down is a worse liability
// than no monitoring endpoint.
func TestCheckSurvivesAnApplicationWithNoConfiguration(t *testing.T) {
	application := &application{serviceRegistry: newServiceRegistry()}
	checker := application.GetHealthChecker()
	checker.RegisterCheck("ok", HealthCheck{
		Name:      "ok",
		CheckFunc: func(context.Context) error { return nil },
	})

	status := checker.Check(context.Background())
	if status == nil || status.Status != "healthy" {
		t.Fatalf("status = %+v", status)
	}
}
