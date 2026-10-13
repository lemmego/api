package middleware

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/lemmego/api/app"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

type LogOptions struct {
	Headers   bool
	UserAgent bool
}

// Asserted at compile time, because the previous attempt at this in
// api/app/tracing.go declared Hijack as (interface{}, interface{}, error) and
// therefore satisfied nothing — with a comment above it claiming otherwise.
// A declaration cannot be wrong about this.
var (
	_ http.Flusher                              = (*responseRecorder)(nil)
	_ http.Hijacker                             = (*responseRecorder)(nil)
	_ interface{ Unwrap() http.ResponseWriter } = (*responseRecorder)(nil)
)

type responseRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader wraps the ResponseWriter's WriteHeader method to capture the status code.
func (r *responseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

// Status method returns the status code.
func (r *responseRecorder) Status() int {
	return r.status
}

// Unwrap returns the writer this one wraps, which is what lets everything
// net/http can do through an optional interface keep working through it.
//
// Embedding http.ResponseWriter promotes exactly three methods — Header,
// Write and WriteHeader — because that is all the interface has. http.Flusher
// and http.Hijacker are not part of it, so a handler asking
// `w.(http.Flusher)` got false the moment this middleware was installed, and
// one asking `w.(http.Hijacker)` likewise. http.ResponseController traverses
// Unwrap to find the real writer, so one method restores flushing, hijacking
// and write deadlines together.
//
// This was not theoretical: with this middleware installed, tasker's own
// dashboard event stream answered 500 "streaming not supported" — a feature
// that shipped broken and stayed broken, because nothing else in the
// framework had ever asked to stream.
func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// Flush and Hijack are forwarded explicitly, because Unwrap alone is not
// enough.
//
// http.ResponseController follows Unwrap, so a handler written against it
// works with only the method above. A handler written as `w.(http.Flusher)`
// does not: a type assertion does not traverse anything, so it sees this type
// and this type's methods. Both styles exist in the wild — tasker's own
// dashboard uses the assertion — so both have to work.
//
// Each forwards through a ResponseController built on the wrapped writer
// rather than on this one, which would recurse through these same methods.
// A failure to flush is logged rather than discarded: http.Flusher cannot
// report one, and a Flush that exists and quietly does nothing is worse than
// no Flush at all, because the caller's assertion succeeded.
func (r *responseRecorder) Flush() {
	if err := http.NewResponseController(r.ResponseWriter).Flush(); err != nil {
		log.Printf("middleware: a response could not be flushed: %v", err)
	}
}

func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(r.ResponseWriter).Hijack()
}

func RequestLogger(opts ...*LogOptions) app.HTTPMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Capture the start time to measure latency
			start := time.Now()

			var logStr strings.Builder
			logStr.WriteString(fmt.Sprintf("[%s] <- %s %s", r.Method, r.URL, r.RemoteAddr))

			if len(opts) > 0 && opts[0].Headers {
				headers, err := json.Marshal(r.Header)
				if err != nil {
					panic(err)
				}
				logStr.WriteString(fmt.Sprintf("\n\nHeaders: %s\n\n", headers))
			}

			// Log request details
			log.Printf("%s", logStr.String())

			// Create a responseRecorder to capture the status
			recorder := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)

			// Calculate the time taken for this request
			latency := time.Since(start)

			// Log after the request is processed including the status code
			log.Printf(
				"[%s] -> %s %s - %d | Completed in %v",
				r.Method,
				r.URL.Path,
				r.RemoteAddr,
				recorder.Status(),
				latency,
			)
		})
	}
}
