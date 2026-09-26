// Package middleware provides HTTP middleware components for the Lemmego framework.
//
// This file contains CSRF (Cross-Site Request Forgery) protection middleware
// that generates and validates tokens to prevent CSRF attacks. It uses secure
// token generation and validation with session-based token storage.
package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
	"github.com/lemmego/api/req"
)

// CSRFOpts holds configuration options for CSRF middleware.
type CSRFOpts struct {
	// ExcludePatterns contains regex patterns for routes that should skip CSRF verification.
	// For example: []string{"/api/.*", "/webhooks/.*"}
	ExcludePatterns []string
}

// compiledRegexCache caches compiled regex patterns for performance.
var compiledRegexCache = make(map[string]*regexp.Regexp)
var regexCacheMutex sync.RWMutex

// getRandomToken generates a cryptographically secure random token of the specified length.
// It uses crypto/rand for secure random number generation and base64 encoding for the token.
func getRandomToken(length int) string {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		slog.Error("Critical error generating random token", "error", err)
		panic("Failed to generate CSRF token")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// matchedToken reports whether the request carries the session's CSRF token.
//
// It deliberately does not rotate the token. Rotating on every verified
// request means any client that reuses a token — one that missed a Set-Cookie,
// had a request already in flight, was restored from the back/forward cache,
// or is a second tab — is rejected with 419, and stays rejected until a full
// page load. Laravel, Rails and Django all keep a per-session token for this
// reason; a CSRF token is a shared secret, not a nonce. The token still
// changes whenever the session itself is regenerated.
func matchedToken(c app.HttpProvider) bool {
	sessionToken := c.SessionString("_token")
	token := getTokenFromRequest(c)

	return tokensMatch(sessionToken, token)
}

// tokensMatch compares the session token with the one the request carried,
// in constant time so the comparison leaks nothing through timing.
func tokensMatch(sessionToken, requestToken string) bool {
	if sessionToken == "" || requestToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(sessionToken), []byte(requestToken)) == 1
}

func getTokenFromRequest(c app.HttpProvider) string {
	token := c.Header("X-XSRF-TOKEN")
	if token == "" {
		token = c.Request().PostFormValue("_token")
	}
	if token == "" {
		token = c.Request().FormValue("_token")
	}

	//if token == "" {
	//	if csrfCookie, err := c.Request().Cookie("XSRF-TOKEN"); err == nil {
	//		token = strings.TrimSpace(csrfCookie.Value)
	//	}
	//}

	if token == "" {
		body := map[string]any{}
		if err := req.DecodeJSONBody(c.ResponseWriter(), c.Request(), &body); err != nil {
			token = ""
		}
		if val, ok := body["_token"].(string); ok {
			token = val
		}
	}
	return token
}

// shouldExcludePath checks if the given path matches any of the exclusion patterns.
func shouldExcludePath(path string, patterns []string) bool {
	for _, pattern := range patterns {
		// Check cache first
		regexCacheMutex.RLock()
		regex, ok := compiledRegexCache[pattern]
		regexCacheMutex.RUnlock()
		if !ok {
			// Compile and cache the regex
			var err error
			regex, err = regexp.Compile(pattern)
			if err != nil {
				slog.Warn("Invalid CSRF exclusion pattern", "pattern", pattern, "error", err)
				continue
			}
			regexCacheMutex.Lock()
			// Another goroutine may have compiled the same pattern while we
			// were compiling it. Reusing that value keeps the cache consistent.
			if cached, exists := compiledRegexCache[pattern]; exists {
				regex = cached
			} else {
				compiledRegexCache[pattern] = regex
			}
			regexCacheMutex.Unlock()
		}

		if regex.MatchString(path) {
			return true
		}
	}
	return false
}

// VerifyCSRF creates and returns a CSRF protection middleware handler with optional configuration.
// If opts is nil, default options are used (no exclusions).
func VerifyCSRF(opts *CSRFOpts) app.Handler {
	return func(c app.Context) error {
		// Check if this route should be excluded from CSRF verification
		if opts != nil && len(opts.ExcludePatterns) > 0 {
			if shouldExcludePath(c.Request().URL.Path, opts.ExcludePatterns) {
				return c.Next()
			}
		}

		if c.IsReading() || matchedToken(c) {
			// Refresh the cookie on every response, not only HTML ones. An XHR
			// navigation that skipped it used to leave the client with nothing
			// to send on the next write.
			if !strings.HasPrefix(c.Request().URL.Path, "/static") {
				token := ""
				if val, ok := c.Session("_token").(string); ok && val != "" {
					token = val
				} else {
					token = getRandomToken(40)
				}
				c.PutSession("_token", token)
				c.Set("_token", token)

				// TODO: Find a way to share the token with inertia
				//i, err := di.Resolve[*inertia.Inertia](c.App().Container())
				//
				//if err == nil && i != nil {
				//	i.ShareProp("csrfToken", token)
				//}

				c.SetCookie(&http.Cookie{
					Name:     "XSRF-TOKEN",
					Value:    token,
					Expires:  time.Now().Add(csrfCookieLifetime()),
					Path:     "/",
					Domain:   "",
					Secure:   c.App().InProduction(),
					HttpOnly: false,
					SameSite: http.SameSiteLaxMode, // Prevents the browser from sending this cookie along with cross-site requests
				})
			}
			return c.Next()
		}

		return c.PageExpired()
	}
}

// defaultCSRFCookieLifetime is how long the token cookie lives when the session
// configuration does not say.
const defaultCSRFCookieLifetime = 2 * time.Hour

// csrfCookieLifetime reads the session lifetime, which was previously asserted
// straight out of the configuration — so a project whose session config was
// absent, or that spelled the lifetime as a number of seconds rather than a
// duration, panicked on the first request that set the token cookie.
func csrfCookieLifetime() time.Duration {
	switch value := config.Get("session.lifetime").(type) {
	case time.Duration:
		if value > 0 {
			return value
		}
	case int:
		if value > 0 {
			return time.Duration(value) * time.Minute
		}
	}
	return defaultCSRFCookieLifetime
}
