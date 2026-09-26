package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// A health check runs in its own goroutine, so a panic in one is never reached
// by the HTTP recovery middleware and takes the whole process down — an
// endpoint whose job is to report that something is wrong killing the server
// instead.
func TestAPanickingCheckIsReportedNotFatal(t *testing.T) {
	checker := NewHealthChecker(nil)
	checker.RegisterCheck("explodes", HealthCheck{
		Name:    "explodes",
		Timeout: time.Second,
		CheckFunc: func(context.Context) error {
			panic("something went badly wrong")
		},
	})

	status := checker.Check(context.Background())

	result, ok := status.Checks["explodes"]
	if !ok {
		t.Fatal("the check produced no result")
	}
	if result.Status != "fail" {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if !strings.Contains(result.Error, "something went badly wrong") {
		t.Errorf("the panic value was not reported: %q", result.Error)
	}
}

// The built-in checks resolved a service and then tested whether it was nil.
// Resolving panics when the service is absent, so the nil test could never
// run — and this is exactly the case it was written for.
func TestDefaultChecksReportAbsentServices(t *testing.T) {
	a := Configure()
	checker := NewHealthChecker(a)
	checker.RegisterDefaultChecks()

	status := checker.Check(context.Background())

	for _, name := range []string{"filesystem", "session"} {
		result, ok := status.Checks[name]
		if !ok {
			t.Fatalf("the %s check produced no result", name)
		}
		if result.Status != "fail" {
			t.Errorf("%s status = %q, want fail on an application with no such service", name, result.Status)
		}
		if !strings.Contains(result.Error, "registered") {
			t.Errorf("%s error = %q, want it to say the service is not registered", name, result.Error)
		}
	}
}

// An ordinary failing check still reports normally.
func TestAFailingCheckIsReported(t *testing.T) {
	checker := NewHealthChecker(nil)
	wantErr := errors.New("the disk is full")
	checker.RegisterCheck("disk", HealthCheck{
		Name:      "disk",
		Timeout:   time.Second,
		CheckFunc: func(context.Context) error { return wantErr },
	})

	status := checker.Check(context.Background())
	if got := status.Checks["disk"].Error; got != wantErr.Error() {
		t.Errorf("error = %q, want %q", got, wantErr)
	}
}
