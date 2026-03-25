// Package app provides health check endpoints for monitoring application status
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// HealthChecker provides health check functionality for the application
type HealthChecker struct {
	app          *application
	checks       map[string]HealthCheck
	mu           sync.RWMutex
	timeout      time.Duration
	cachedStatus *HealthStatus
	cacheExpiry  time.Time
	cacheMu      sync.RWMutex
	cacheDuration time.Duration
}

// HealthCheck defines a health check that can be performed
type HealthCheck struct {
	Name        string
	Description string
	CheckFunc   HealthCheckFunc
	Critical    bool // If true, application is considered unhealthy if this check fails
	Timeout     time.Duration
}

// HealthCheckFunc is a function that performs a health check
type HealthCheckFunc func(ctx context.Context) error

// HealthStatus represents the overall health status of the application
type HealthStatus struct {
	Status    string                 `json:"status"` // healthy, degraded, unhealthy
	Timestamp string                 `json:"timestamp"`
	Checks    map[string]CheckResult `json:"checks"`
	Version   string                 `json:"version,omitempty"`
	Env       string                 `json:"env,omitempty"`
}

// CheckResult represents the result of a single health check
type CheckResult struct {
	Status      string `json:"status"`      // pass, fail, warn
	Message     string `json:"message"`     // Human-readable message
	Error       string `json:"error,omitempty"` // Error message if check failed
	DurationMs  int64  `json:"duration_ms"` // Time taken to execute check
	Critical    bool   `json:"critical"`    // Whether this is a critical check
	Description string `json:"description,omitempty"` // Description of what this check tests
}

// NewHealthChecker creates a new health checker for the application
func NewHealthChecker(app *application) *HealthChecker {
	return &HealthChecker{
		app:           app,
		checks:        make(map[string]HealthCheck),
		timeout:       10 * time.Second,
		cacheDuration: 5 * time.Second,
	}
}

// RegisterCheck registers a health check with the given name
func (h *HealthChecker) RegisterCheck(name string, check HealthCheck) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Set default timeout if not specified
	if check.Timeout == 0 {
		check.Timeout = h.timeout
	}

	h.checks[name] = check
}

// UnregisterCheck removes a health check by name
func (h *HealthChecker) UnregisterCheck(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.checks, name)
}

// Check performs all registered health checks and returns the overall status
func (h *HealthChecker) Check(ctx context.Context) *HealthStatus {
	// Check cache first
	h.cacheMu.RLock()
	if h.cachedStatus != nil && time.Now().Before(h.cacheExpiry) {
		status := h.cachedStatus
		h.cacheMu.RUnlock()
		return status
	}
	h.cacheMu.RUnlock()

	h.mu.RLock()
	defer h.mu.RUnlock()

	// Create context with timeout for all checks
	checkCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	results := make(map[string]CheckResult)
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Run all checks in parallel
	for name, check := range h.checks {
		wg.Add(1)
		go func(name string, check HealthCheck) {
			defer wg.Done()

			start := time.Now()
			result := CheckResult{
				Status:      "pass",
				Critical:    check.Critical,
				Description: check.Description,
			}

			// Create context with individual timeout for this check
			checkCtx, cancel := context.WithTimeout(checkCtx, check.Timeout)
			defer cancel()

			err := check.CheckFunc(checkCtx)
			result.DurationMs = time.Since(start).Milliseconds()

			if err != nil {
				result.Status = "fail"
				result.Error = err.Error()
				result.Message = fmt.Sprintf("Health check failed: %v", err)
			} else {
				result.Message = "Health check passed"
			}

			mu.Lock()
			results[name] = result
			mu.Unlock()
		}(name, check)
	}

	wg.Wait()

	// Determine overall status
	status := h.determineStatus(results)

	healthStatus := &HealthStatus{
		Status:    status,
		Timestamp: time.Now().Format(time.RFC3339),
		Checks:    results,
	}

	// Add version and environment if available
	if h.app != nil {
		if name := h.app.config.Get("app.name"); name != nil {
			healthStatus.Version = fmt.Sprintf("%v", name)
		}
		if env := h.app.config.Get("app.env"); env != nil {
			healthStatus.Env = fmt.Sprintf("%v", env)
		}
	}

	// Cache the result
	h.cacheMu.Lock()
	h.cachedStatus = healthStatus
	h.cacheExpiry = time.Now().Add(h.cacheDuration)
	h.cacheMu.Unlock()

	return healthStatus
}

// determineStatus determines the overall health status based on check results
func (h *HealthChecker) determineStatus(results map[string]CheckResult) string {
	hasFailedCritical := false
	hasFailedNonCritical := false
	hasWarning := false

	for _, result := range results {
		switch result.Status {
		case "fail":
			if result.Critical {
				hasFailedCritical = true
			} else {
				hasFailedNonCritical = true
			}
		case "warn":
			hasWarning = true
		}
	}

	if hasFailedCritical {
		return "unhealthy"
	} else if hasFailedNonCritical {
		return "degraded"
	} else if hasWarning {
		return "degraded"
	}
	return "healthy"
}

// SetCacheDuration sets how long to cache health check results
func (h *HealthChecker) SetCacheDuration(duration time.Duration) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	h.cacheDuration = duration
}

// SetTimeout sets the timeout for health checks
func (h *HealthChecker) SetTimeout(timeout time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.timeout = timeout
}

// InvalidateCache clears the cached health status
func (h *HealthChecker) InvalidateCache() {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	h.cachedStatus = nil
}

// HealthCheckHandler returns an HTTP handler for the health check endpoint
func (h *HealthChecker) HealthCheckHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Parse query parameters
		var timeout time.Duration
		if timeoutStr := r.URL.Query().Get("timeout"); timeoutStr != "" {
			var err error
			timeout, err = time.ParseDuration(timeoutStr)
			if err != nil {
				http.Error(w, fmt.Sprintf("Invalid timeout: %v", err), http.StatusBadRequest)
				return
			}
		}

		// Apply custom timeout if specified
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		status := h.Check(ctx)

		// Determine HTTP status code based on health status
		var httpStatus int
		switch status.Status {
		case "healthy":
			httpStatus = http.StatusOK
		case "degraded":
			httpStatus = http.StatusOK // Still OK but with degraded functionality
		case "unhealthy":
			httpStatus = http.StatusServiceUnavailable
		default:
			httpStatus = http.StatusInternalServerError
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)

		if err := json.NewEncoder(w).Encode(status); err != nil {
			slog.Error("failed to encode health status", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}
}

// LivenessHandler returns an HTTP handler for the liveness endpoint
// This endpoint should return quickly and indicates whether the application is running
func (h *HealthChecker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok"}`)
	}
}

// ReadinessHandler returns an HTTP handler for the readiness endpoint
// This endpoint indicates whether the application is ready to handle requests
func (h *HealthChecker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// For readiness, we can use the cached health check for speed
		h.cacheMu.RLock()
		cachedStatus := h.cachedStatus
		isCacheValid := cachedStatus != nil && time.Now().Before(h.cacheExpiry)
		h.cacheMu.RUnlock()

		var isReady bool
		if isCacheValid {
			isReady = cachedStatus.Status == "healthy" || cachedStatus.Status == "degraded"
		} else {
			// Run a quick health check
			status := h.Check(r.Context())
			isReady = status.Status == "healthy" || status.Status == "degraded"
		}

		if isReady {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"status":"ready"}`)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"not_ready"}`)
		}
	}
}

// RegisterDefaultChecks registers default health checks for the application
func (h *HealthChecker) RegisterDefaultChecks() {
	// File system health check
	h.RegisterCheck("filesystem", HealthCheck{
		Name:        "filesystem",
		Description: "Check if the file system is accessible",
		CheckFunc: func(ctx context.Context) error {
			if h.app == nil {
				return fmt.Errorf("application not initialized")
			}

			fs := h.app.FileSystem()
			if fs == nil {
				return fmt.Errorf("file system not initialized")
			}

			// Try to access the file system
			// This is a basic check - in production you might want to check specific disks/buckets
			return nil
		},
		Critical: false,
		Timeout: 2 * time.Second,
	})

	// Session health check
	h.RegisterCheck("session", HealthCheck{
		Name:        "session",
		Description: "Check if the session store is accessible",
		CheckFunc: func(ctx context.Context) error {
			if h.app == nil {
				return fmt.Errorf("application not initialized")
			}

			sess := h.app.Session()
			if sess == nil {
				return fmt.Errorf("session not initialized")
			}

			// Basic session store check
			return nil
		},
		Critical: true,
		Timeout:  2 * time.Second,
	})
}

// GetHealthChecker returns the health checker for the application
func (a *application) GetHealthChecker() *HealthChecker {
	return &HealthChecker{app: a}
}
