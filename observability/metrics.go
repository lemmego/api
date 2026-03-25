// Package observability provides metrics collection capabilities for the Lemmego framework
package observability

import (
	"net/http"
	"sync"
	"time"
)

// MetricsCollector collects and aggregates application metrics
type MetricsCollector struct {
	mu                sync.RWMutex
	httpRequests      *HTTPMetrics
	counters          map[string]int64
	gauges            map[string]int64
	histograms        map[string]*Histogram
	enabled           bool
}

// HTTPMetrics tracks HTTP request metrics
type HTTPMetrics struct {
	TotalRequests   int64
	ActiveRequests  int64
	TotalErrors     int64
	StatusCodes     map[int]int64
	PathMetrics     map[string]*PathMetrics
	MethodMetrics   map[string]int64
	mu              sync.RWMutex
}

// PathMetrics tracks metrics for specific paths
type PathMetrics struct {
	TotalRequests   int64
	TotalErrors     int64
	AverageLatency  time.Duration
	MaxLatency      time.Duration
	StatusCodes     map[int]int64
	mu              sync.RWMutex
}

// Histogram tracks distribution of values
type Histogram struct {
	Count    int64
	Sum      int64
	Min      int64
	Max      int64
	Buckets  map[int64]int64
	mu       sync.RWMutex
}

// MetricsConfig holds configuration for the metrics collector
type MetricsConfig struct {
	Enabled           bool
	HistogramBuckets  []int64
	TrackPaths        bool
	TrackStatusCodes  bool
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(config MetricsConfig) *MetricsCollector {
	mc := &MetricsCollector{
		httpRequests: &HTTPMetrics{
			StatusCodes:   make(map[int]int64),
			PathMetrics:   make(map[string]*PathMetrics),
			MethodMetrics: make(map[string]int64),
		},
		counters:   make(map[string]int64),
		gauges:     make(map[string]int64),
		histograms: make(map[string]*Histogram),
		enabled:    config.Enabled,
	}

	// Initialize default histogram buckets if not provided
	if config.HistogramBuckets == nil {
		config.HistogramBuckets = []int64{10, 50, 100, 500, 1000, 5000, 10000}
	}

	return mc
}

// RecordHTTPRequest records an HTTP request
func (mc *MetricsCollector) RecordHTTPRequest(method, path string, statusCode int, duration time.Duration) {
	if !mc.enabled {
		return
	}

	mc.httpRequests.mu.Lock()
	defer mc.httpRequests.mu.Unlock()

	mc.httpRequests.TotalRequests++
	if statusCode >= 400 {
		mc.httpRequests.TotalErrors++
	}
	mc.httpRequests.StatusCodes[statusCode]++
	mc.httpRequests.MethodMetrics[method]++

	if pathMetrics, ok := mc.httpRequests.PathMetrics[path]; ok {
		pathMetrics.mu.Lock()
		pathMetrics.TotalRequests++
		if statusCode >= 400 {
			pathMetrics.TotalErrors++
		}
		pathMetrics.StatusCodes[statusCode]++
		pathMetrics.mu.Unlock()
	} else {
		mc.httpRequests.PathMetrics[path] = &PathMetrics{
			TotalRequests: 1,
			StatusCodes:   map[int]int64{statusCode: 1},
			TotalErrors:   boolToInt(statusCode >= 400),
		}
	}
}

// RecordRequestStart records the start of an HTTP request
func (mc *MetricsCollector) RecordRequestStart() {
	if !mc.enabled {
		return
	}

	mc.httpRequests.mu.Lock()
	defer mc.httpRequests.mu.Unlock()
	mc.httpRequests.ActiveRequests++
}

// RecordRequestEnd records the end of an HTTP request
func (mc *MetricsCollector) RecordRequestEnd() {
	if !mc.enabled {
		return
	}

	mc.httpRequests.mu.Lock()
	defer mc.httpRequests.mu.Unlock()
	mc.httpRequests.ActiveRequests--
}

// IncrementCounter increments a counter metric
func (mc *MetricsCollector) IncrementCounter(name string, value int64) {
	if !mc.enabled {
		return
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.counters[name] += value
}

// SetGauge sets a gauge metric value
func (mc *MetricsCollector) SetGauge(name string, value int64) {
	if !mc.enabled {
		return
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.gauges[name] = value
}

// RecordHistogram records a value in a histogram
func (mc *MetricsCollector) RecordHistogram(name string, value int64) {
	if !mc.enabled {
		return
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	hist, ok := mc.histograms[name]
	if !ok {
		hist = &Histogram{
			Min:     value,
			Max:     value,
			Buckets: make(map[int64]int64),
		}
		mc.histograms[name] = hist
	}

	hist.mu.Lock()
	defer hist.mu.Unlock()

	hist.Count++
	hist.Sum += value
	if value < hist.Min {
		hist.Min = value
	}
	if value > hist.Max {
		hist.Max = value
	}

	// Find appropriate bucket
	bucket := hist.findBucket(value)
	hist.Buckets[bucket]++
}

// GetCounter returns the current value of a counter
func (mc *MetricsCollector) GetCounter(name string) int64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.counters[name]
}

// GetGauge returns the current value of a gauge
func (mc *MetricsCollector) GetGauge(name string) int64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.gauges[name]
}

// GetHTTPMetrics returns a snapshot of HTTP metrics
func (mc *MetricsCollector) GetHTTPMetrics() *HTTPMetricsSnapshot {
	mc.httpRequests.mu.RLock()
	defer mc.httpRequests.mu.RUnlock()

	snapshot := &HTTPMetricsSnapshot{
		TotalRequests:  mc.httpRequests.TotalRequests,
		ActiveRequests: mc.httpRequests.ActiveRequests,
		TotalErrors:    mc.httpRequests.TotalErrors,
		StatusCodes:    make(map[int]int64),
		PathMetrics:    make(map[string]*PathMetricsSnapshot),
		MethodMetrics:  make(map[string]int64),
	}

	// Copy status codes
	for k, v := range mc.httpRequests.StatusCodes {
		snapshot.StatusCodes[k] = v
	}

	// Copy method metrics
	for k, v := range mc.httpRequests.MethodMetrics {
		snapshot.MethodMetrics[k] = v
	}

	// Copy path metrics
	for path, pm := range mc.httpRequests.PathMetrics {
		pm.mu.RLock()
		pathSnapshot := &PathMetricsSnapshot{
			TotalRequests:  pm.TotalRequests,
			TotalErrors:    pm.TotalErrors,
			AverageLatency: pm.AverageLatency,
			MaxLatency:     pm.MaxLatency,
			StatusCodes:    make(map[int]int64),
		}
		for k, v := range pm.StatusCodes {
			pathSnapshot.StatusCodes[k] = v
		}
		pm.mu.RUnlock()
		snapshot.PathMetrics[path] = pathSnapshot
	}

	return snapshot
}

// GetHistogram returns a snapshot of a histogram
func (mc *MetricsCollector) GetHistogram(name string) *HistogramSnapshot {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	hist, ok := mc.histograms[name]
	if !ok {
		return nil
	}

	hist.mu.RLock()
	defer hist.mu.RUnlock()

	snapshot := &HistogramSnapshot{
		Count:   hist.Count,
		Sum:     hist.Sum,
		Average: float64(hist.Sum) / float64(hist.Count),
		Min:     hist.Min,
		Max:     hist.Max,
		Buckets: make(map[int64]int64),
	}

	for k, v := range hist.Buckets {
		snapshot.Buckets[k] = v
	}

	return snapshot
}

// GetAllMetrics returns all metrics as a map
func (mc *MetricsCollector) GetAllMetrics() map[string]interface{} {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	metrics := make(map[string]interface{})

	// Add counters
	counters := make(map[string]int64)
	for k, v := range mc.counters {
		counters[k] = v
	}
	metrics["counters"] = counters

	// Add gauges
	gauges := make(map[string]int64)
	for k, v := range mc.gauges {
		gauges[k] = v
	}
	metrics["gauges"] = gauges

	// Add histograms
	histograms := make(map[string]*HistogramSnapshot)
	for k := range mc.histograms {
		if snap := mc.GetHistogram(k); snap != nil {
			histograms[k] = snap
		}
	}
	metrics["histograms"] = histograms

	// Add HTTP metrics
	metrics["http"] = mc.GetHTTPMetrics()

	return metrics
}

// Reset resets all metrics
func (mc *MetricsCollector) Reset() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.counters = make(map[string]int64)
	mc.gauges = make(map[string]int64)
	mc.histograms = make(map[string]*Histogram)

	mc.httpRequests.mu.Lock()
	mc.httpRequests.TotalRequests = 0
	mc.httpRequests.ActiveRequests = 0
	mc.httpRequests.TotalErrors = 0
	mc.httpRequests.StatusCodes = make(map[int]int64)
	mc.httpRequests.PathMetrics = make(map[string]*PathMetrics)
	mc.httpRequests.MethodMetrics = make(map[string]int64)
	mc.httpRequests.mu.Unlock()
}

// findBucket finds the appropriate bucket for a value
func (h *Histogram) findBucket(value int64) int64 {
	// Default buckets: 10, 50, 100, 500, 1000, 5000, 10000
	buckets := []int64{10, 50, 100, 500, 1000, 5000, 10000}

	for _, bucket := range buckets {
		if value <= bucket {
			return bucket
		}
	}
	return 10000 // Max bucket
}

// HTTPMetricsSnapshot represents an immutable snapshot of HTTP metrics
type HTTPMetricsSnapshot struct {
	TotalRequests  int64                        `json:"total_requests"`
	ActiveRequests int64                        `json:"active_requests"`
	TotalErrors    int64                        `json:"total_errors"`
	StatusCodes    map[int]int64                `json:"status_codes"`
	PathMetrics    map[string]*PathMetricsSnapshot `json:"path_metrics"`
	MethodMetrics  map[string]int64             `json:"method_metrics"`
}

// PathMetricsSnapshot represents an immutable snapshot of path metrics
type PathMetricsSnapshot struct {
	TotalRequests  int64           `json:"total_requests"`
	TotalErrors    int64           `json:"total_errors"`
	AverageLatency time.Duration   `json:"average_latency"`
	MaxLatency     time.Duration   `json:"max_latency"`
	StatusCodes    map[int]int64   `json:"status_codes"`
}

// HistogramSnapshot represents an immutable snapshot of a histogram
type HistogramSnapshot struct {
	Count   int64           `json:"count"`
	Sum     int64           `json:"sum"`
	Average float64         `json:"average"`
	Min     int64           `json:"min"`
	Max     int64           `json:"max"`
	Buckets map[int64]int64 `json:"buckets"`
}

// MetricsMiddleware returns HTTP middleware for collecting request metrics
func (mc *MetricsCollector) MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		mc.RecordRequestStart()
		defer mc.RecordRequestEnd()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, status: 200}
		defer func() {
			duration := time.Since(start)
			mc.RecordHTTPRequest(r.Method, r.URL.Path, wrapped.status, duration)
		}()

		next.ServeHTTP(wrapped, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.status = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// DefaultMetricsCollector is the default metrics collector instance
var DefaultMetricsCollector *MetricsCollector

// InitMetricsCollector initializes the default metrics collector
func InitMetricsCollector(enabled bool) {
	config := MetricsConfig{
		Enabled:          enabled,
		HistogramBuckets: []int64{10, 50, 100, 500, 1000, 5000, 10000},
		TrackPaths:       true,
		TrackStatusCodes: true,
	}
	DefaultMetricsCollector = NewMetricsCollector(config)
}

// Helper function
func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
