package app

import (
	"context"
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
