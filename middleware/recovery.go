// Package middleware provides HTTP middleware components for the Lemmego framework.
//
// This file contains panic recovery middleware that catches panics during
// HTTP request handling and converts them to appropriate HTTP error responses.
// It logs the panic and provides a clean error response to the client.
package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strings"

	"github.com/lemmego/api/app"
)

// RecoveryOpts holds configuration options for recovery middleware.
type RecoveryOpts struct {
	// LogStacktrace determines whether to log the full stack trace of panics
	LogStacktrace bool
	// EnableStackTraceInResponse determines whether to include stack traces in error responses
	// WARNING: Only enable this in development environments as it may leak sensitive information
	EnableStackTraceInResponse bool
	// CustomRecoveryFunc allows custom panic recovery logic
	// If set, this function will be called instead of the default recovery logic
	CustomRecoveryFunc func(app.Context, interface{})
}

// Default recovery configuration for production
var DefaultRecoveryOpts = &RecoveryOpts{
	LogStacktrace:                true,
	EnableStackTraceInResponse: false,
}

// Development recovery configuration
var DevelopmentRecoveryOpts = &RecoveryOpts{
	LogStacktrace:                true,
	EnableStackTraceInResponse: true,
}

// Recovery creates a panic recovery middleware with default options.
// The middleware catches panics, logs them, and returns a 500 Internal Server Error
// response to the client.
func Recovery() app.Handler {
	return RecoveryWithOpts(nil)
}

// RecoveryWithOpts creates a panic recovery middleware with custom options.
// If opts is nil, default options are used.
func RecoveryWithOpts(opts *RecoveryOpts) app.Handler {
	if opts == nil {
		opts = DefaultRecoveryOpts
	}

	return func(c app.Context) error {
		// Defer a panic recovery function
		defer func() {
			if err := recover(); err != nil {
				// Call custom recovery function if provided
				if opts.CustomRecoveryFunc != nil {
					opts.CustomRecoveryFunc(c, err)
					return
				}

				// Default panic recovery
				handlePanic(c, err, opts)
			}
		}()

		return c.Next()
	}
}

// handlePanic handles a recovered panic and sends an appropriate response
func handlePanic(c app.Context, err interface{}, opts *RecoveryOpts) {
	// Get stack trace
	stackTrace := getStackTrace(err)

	// Log the panic
	logAttrs := []slog.Attr{
		slog.String("panic", fmt.Sprintf("%v", err)),
		slog.String("method", c.Request().Method),
		slog.String("path", c.Request().URL.Path),
		slog.String("query", c.Request().URL.RawQuery),
		slog.String("remote_addr", c.Request().RemoteAddr),
	}

	if opts.LogStacktrace {
		logAttrs = append(logAttrs, slog.String("stack_trace", stackTrace))
	}

	slog.LogAttrs(c.Request().Context(), slog.LevelError, "panic recovered", logAttrs...)

	// Determine response based on client type
 acceptsHTML := strings.Contains(c.Request().Header.Get("Accept"), "text/html")
	acceptsJSON := strings.Contains(c.Request().Header.Get("Accept"), "application/json")

	if acceptsJSON && !acceptsHTML {
		// JSON response
		response := map[string]interface{}{
			"error":   "Internal Server Error",
			"message": "An unexpected error occurred",
		}

		if opts.EnableStackTraceInResponse {
			response["panic"] = fmt.Sprintf("%v", err)
			response["stack_trace"] = stackTrace
		}

		c.ResponseWriter().Header().Set("Content-Type", "application/json")
		c.ResponseWriter().WriteHeader(http.StatusInternalServerError)

		if jsonErr := json.NewEncoder(c.ResponseWriter()).Encode(response); jsonErr != nil {
			slog.Error("failed to encode panic response", "error", jsonErr)
		}
	} else {
		// HTML response
		c.ResponseWriter().WriteHeader(http.StatusInternalServerError)

		if opts.EnableStackTraceInResponse {
			html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<title>Panic Recovered</title>
	<style>
		body { font-family: system-ui, sans-serif; margin: 40px; }
		.panic { background: #fee; border: 1px solid #c00; padding: 20px; border-radius: 4px; }
		.stack { background: #f5f5f5; padding: 15px; margin-top: 20px; border-radius: 4px; font-family: monospace; font-size: 12px; }
		h1 { color: #c00; }
	</style>
</head>
<body>
	<div class="panic">
		<h1>Panic Recovered</h1>
		<p><strong>Error:</strong> %v</p>
		<div class="stack"><pre>%s</pre></div>
	</div>
</body>
</html>`, err, stackTrace)
			c.ResponseWriter().Write([]byte(html))
		} else {
			html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<title>Internal Server Error</title>
	<style>
		body { font-family: system-ui, sans-serif; margin: 40px; text-align: center; }
		h1 { color: #666; }
	</style>
</head>
<body>
	<h1>Internal Server Error</h1>
	<p>An unexpected error occurred. Please try again later.</p>
</body>
</html>`)
			c.ResponseWriter().Write([]byte(html))
		}
	}
}

// getStackTrace captures the stack trace from the panic point
func getStackTrace(err interface{}) string {
	// Get the stack trace
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)

	stack := string(buf[:n])

	// Try to get more specific error info
	var panicMsg string
	switch v := err.(type) {
	case string:
		panicMsg = v
	case error:
		panicMsg = v.Error()
	default:
		panicMsg = fmt.Sprintf("%v", v)
	}

	// Format the stack trace
	lines := strings.Split(stack, "\n")

	// Filter and format the stack trace
	var formattedStack []string
	for i, line := range lines {
		if i == 0 {
			// Skip the first line (it's just "goroutine X [running/created]:")
			continue
		}

		// Trim leading whitespace
		line = strings.TrimSpace(line)

		// Skip certain frames for cleaner output
		if strings.Contains(line, "runtime.") ||
		   strings.Contains(line, "panic(") ||
		   strings.Contains(line, "handlePanic") {
			continue
		}

		formattedStack = append(formattedStack, line)
	}

	return fmt.Sprintf("Panic: %s\n\nStack Trace:\n%s", panicMsg, strings.Join(formattedStack, "\n"))
}

// RecoveryWithCustomFunc creates a panic recovery middleware with a custom recovery function.
// The custom function receives the context and the recovered panic value.
func RecoveryWithCustomFunc(recoveryFunc func(*app.Context, interface{})) app.Handler {
	return func(c app.Context) error {
		defer func() {
			if err := recover(); err != nil {
				recoveryFunc(&c, err)
			}
		}()

		return c.Next()
	}
}

// PrettyPrintStackTrace returns a formatted, pretty-printed stack trace
// This is useful for logging stack traces in a more readable format
func PrettyPrintStackTrace(stackTrace string) string {
	lines := strings.Split(stackTrace, "\n")

	var result strings.Builder
	result.WriteString("Stack Trace:\n")
	result.WriteString(strings.Repeat("=", 80) + "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Highlight different parts of the stack trace
		if strings.Contains(line, "goroutine") {
			result.WriteString("\n" + line + "\n")
			result.WriteString(strings.Repeat("-", 80) + "\n")
		} else if strings.Contains(line, "created by") {
			result.WriteString("\n" + line + "\n")
			result.WriteString(strings.Repeat("-", 80) + "\n")
		} else {
			// Indent the function call
			result.WriteString("  " + line + "\n")
		}
	}

	result.WriteString(strings.Repeat("=", 80) + "\n")
	return result.String()
}

// GetPanicMessage extracts a meaningful panic message from the panic value
func GetPanicMessage(err interface{}) string {
	if err == nil {
		return "unknown panic"
	}

	switch v := err.(type) {
	case string:
		return v
	case error:
		return v.Error()
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}
