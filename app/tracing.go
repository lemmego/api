// Package app provides request tracing capabilities for the Lemmego framework
package app

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Tracer handles request tracing with unique trace IDs and span tracking
type Tracer struct {
	mu           sync.RWMutex
	spans        map[string]*Span
	enabled      bool
	sampleRate   float64
	propagation  PropagationFormat
}

// PropagationFormat defines how trace context is propagated
type PropagationFormat string

const (
	// PropagationHeader uses HTTP headers for trace propagation
	PropagationHeader PropagationFormat = "header"
	// PropagationMetadata uses metadata for trace propagation
	PropagationMetadata PropagationFormat = "metadata"
)

// Span represents a single span in a trace
type Span struct {
	TraceID      string                 `json:"trace_id"`
	SpanID       string                 `json:"span_id"`
	ParentSpanID string                 `json:"parent_span_id,omitempty"`
	Name         string                 `json:"name"`
	StartTime    time.Time              `json:"start_time"`
	EndTime      time.Time              `json:"end_time"`
	Duration     time.Duration          `json:"duration"`
	Tags         map[string]string      `json:"tags,omitempty"`
	Events       []SpanEvent            `json:"events,omitempty"`
	Status       SpanStatus             `json:"status"`
	mu           sync.RWMutex
}

// SpanEvent represents an event within a span
type SpanEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	Name      string                 `json:"name"`
	Attributes map[string]string     `json:"attributes,omitempty"`
}

// SpanStatus represents the status of a span
type SpanStatus string

const (
	// SpanStatusOK indicates the span completed successfully
	SpanStatusOK SpanStatus = "ok"
	// SpanStatusError indicates the span encountered an error
	SpanStatusError SpanStatus = "error"
	// SpanStatusInternal indicates an internal error
	SpanStatusInternal SpanStatus = "internal"
)

// TracerConfig holds configuration for the tracer
type TracerConfig struct {
	Enabled     bool
	SampleRate  float64 // 0.0 to 1.0, where 1.0 means trace everything
	Propagation PropagationFormat
}

// NewTracer creates a new tracer
func NewTracer(config TracerConfig) *Tracer {
	t := &Tracer{
		spans:       make(map[string]*Span),
		enabled:     config.Enabled,
		sampleRate:  config.SampleRate,
		propagation: config.Propagation,
	}

	if t.sampleRate <= 0 {
		t.sampleRate = 1.0 // Default: trace everything
	}
	if t.sampleRate > 1.0 {
		t.sampleRate = 1.0
	}

	return t
}

// StartSpan starts a new span with the given name
func (t *Tracer) StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	if !t.enabled {
		return ctx, &Span{}
	}

	spanID := uuid.New().String()
	var parentSpanID string
	var traceID string

	// Check if there's a parent span in context
	if parentSpan := SpanFromContext(ctx); parentSpan != nil {
		parentSpanID = parentSpan.SpanID
		traceID = parentSpan.TraceID
	} else {
		// Generate new trace ID
		traceID = uuid.New().String()
	}

	span := &Span{
		TraceID:      traceID,
		SpanID:       spanID,
		ParentSpanID: parentSpanID,
		Name:         name,
		StartTime:    time.Now(),
		Tags:         make(map[string]string),
		Events:       make([]SpanEvent, 0),
		Status:       SpanStatusOK,
	}

	// Store span in tracer
	t.mu.Lock()
	t.spans[spanID] = span
	t.mu.Unlock()

	// Add span to context
	ctx = ContextWithSpan(ctx, span)

	return ctx, span
}

// Finish marks the span as completed
func (s *Span) Finish() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.EndTime = time.Now()
	s.Duration = s.EndTime.Sub(s.StartTime)
}

// SetTag sets a tag on the span
func (s *Span) SetTag(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Tags == nil {
		s.Tags = make(map[string]string)
	}
	s.Tags[key] = value
}

// SetTags sets multiple tags on the span
func (s *Span) SetTags(tags map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Tags == nil {
		s.Tags = make(map[string]string)
	}
	for k, v := range tags {
		s.Tags[k] = v
	}
}

// AddEvent adds an event to the span
func (s *Span) AddEvent(name string, attributes map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	event := SpanEvent{
		Timestamp:  time.Now(),
		Name:       name,
		Attributes: attributes,
	}
	s.Events = append(s.Events, event)
}

// SetError sets the span status to error with an error message
func (s *Span) SetError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Status = SpanStatusError
	s.SetTag("error", err.Error())
}

// GetSpan retrieves a span by ID
func (t *Tracer) GetSpan(spanID string) *Span {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.spans[spanID]
}

// GetTrace retrieves all spans in a trace
func (t *Tracer) GetTrace(traceID string) []*Span {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var spans []*Span
	for _, span := range t.spans {
		if span.TraceID == traceID {
			spans = append(spans, span)
		}
	}
	return spans
}

// ClearTrace removes all spans for a given trace ID
func (t *Tracer) ClearTrace(traceID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for spanID, span := range t.spans {
		if span.TraceID == traceID {
			delete(t.spans, spanID)
		}
	}
}

// ExtractTraceID extracts trace ID from HTTP headers
func (t *Tracer) ExtractTraceID(r *http.Request) string {
	switch t.propagation {
	case PropagationHeader:
		return r.Header.Get("X-Trace-ID")
	case PropagationMetadata:
		return r.URL.Query().Get("trace_id")
	default:
		return r.Header.Get("X-Trace-ID")
	}
}

// InjectTraceID injects trace ID into HTTP headers
func (t *Tracer) InjectTraceID(w http.ResponseWriter, traceID string) {
	switch t.propagation {
	case PropagationHeader:
		w.Header().Set("X-Trace-ID", traceID)
	case PropagationMetadata:
		// For metadata propagation, we'd use a different mechanism
		// This is a placeholder for alternative propagation methods
	default:
		w.Header().Set("X-Trace-ID", traceID)
	}
}

// ShouldSample determines whether to sample this trace
func (t *Tracer) ShouldSample() bool {
	// Simple sampling based on sample rate
	// In production, you might use more sophisticated sampling strategies
	return t.sampleRate >= 1.0 || t.sampleRate > 0
}

// traceContextKey is the context key for storing spans
type traceContextKey struct{}

// ContextWithSpan adds a span to the context
func ContextWithSpan(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, traceContextKey{}, span)
}

// SpanFromContext retrieves a span from the context
func SpanFromContext(ctx context.Context) *Span {
	if span, ok := ctx.Value(traceContextKey{}).(*Span); ok {
		return span
	}
	return nil
}

// RequestIDFromContext retrieves request ID from the context
func RequestIDFromContext(ctx context.Context) string {
	if requestID, ok := ctx.Value("request_id").(string); ok {
		return requestID
	}
	return ""
}

// TraceMiddleware is middleware that adds tracing to HTTP requests
func (t *Tracer) TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !t.enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Extract or generate trace ID
		traceID := t.ExtractTraceID(r)
		if traceID == "" {
			traceID = uuid.New().String()
		}

		// Generate span ID for this request
		spanID := uuid.New().String()

		// Inject trace ID into response
		t.InjectTraceID(w, traceID)

		// Add trace context to the request context
		ctx := r.Context()
		ctx = context.WithValue(ctx, "trace_id", traceID)
		ctx = context.WithValue(ctx, "span_id", spanID)
		ctx = context.WithValue(ctx, "request_id", spanID)

		// Create span for this request
		_, span := t.StartSpan(ctx, fmt.Sprintf("%s %s", r.Method, r.URL.Path))
		defer span.Finish()

		// Set standard tags
		span.SetTag("http.method", r.Method)
		span.SetTag("http.url", r.URL.String())
		span.SetTag("http.host", r.Host)
		span.SetTag("http.scheme", r.URL.Scheme)
		span.SetTag("http.user_agent", r.Header.Get("User-Agent"))
		span.SetTag("http.remote_addr", r.RemoteAddr)

		// Wrap response writer to capture status code
		rw := &tracingResponseWriter{
			ResponseWriter: w,
			span:          span,
		}

		// Add event for request start
		span.AddEvent("request.started", map[string]string{
			"path": r.URL.Path,
		})

		// Call next handler
		next.ServeHTTP(rw, r.WithContext(ctx))

		// Add event for request completion
		span.AddEvent("request.completed", map[string]string{
			"status_code": fmt.Sprintf("%d", rw.status),
		})

		// Set status based on HTTP status
		if rw.status >= 400 {
			span.SetTag("error", fmt.Sprintf("HTTP %d", rw.status))
			if rw.status >= 500 {
				span.Status = SpanStatusError
			}
		}
	})
}

// tracingResponseWriter wraps http.ResponseWriter to capture status code
type tracingResponseWriter struct {
	http.ResponseWriter
	status int
	span   *Span
}

func (rw *tracingResponseWriter) WriteHeader(statusCode int) {
	rw.status = statusCode
	rw.span.SetTag("http.status_code", fmt.Sprintf("%d", statusCode))
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *tracingResponseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = 200
	}
	return rw.ResponseWriter.Write(b)
}

// Flush implements http.Flusher interface
func (rw *tracingResponseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack implements http.Hijacker interface
func (rw *tracingResponseWriter) Hijack() (c interface{}, rw2 interface{}, err error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("http.Hijacker is not supported")
}

// DefaultTracer is the default tracer instance
var DefaultTracer *Tracer

// InitTracer initializes the default tracer
func InitTracer(enabled bool, sampleRate float64) {
	config := TracerConfig{
		Enabled:     enabled,
		SampleRate:  sampleRate,
		Propagation: PropagationHeader,
	}
	DefaultTracer = NewTracer(config)
}

// GetTracer returns the default tracer
func GetTracer() *Tracer {
	if DefaultTracer == nil {
		return NewTracer(TracerConfig{Enabled: false})
	}
	return DefaultTracer
}
