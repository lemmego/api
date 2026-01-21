// Package middleware provides HTTP middleware components for the Lemmego framework.
//
// This file contains CSRF (Cross-Site Request Forgery) protection middleware
// that generates and validates tokens to prevent CSRF attacks. It uses secure
// token generation and validation with session-based token storage.
package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
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
var regexCacheMutex = make(map[string]*struct{})

// getRandomToken generates a cryptographically secure random token of the specified length.
// It uses crypto/rand for secure random number generation and base64 encoding for the token.
func getRandomToken(length int) string {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		slog.Error("Critical error generating random token:", err)
		panic("Failed to generate CSRF token")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func matchedToken(c app.HttpProvider) bool {
	sessionToken := c.SessionString("_token")
	token := getTokenFromRequest(c)

	matched := false
	if sessionToken != "" && token != "" {
		matched = sessionToken == token
	}

	if matched {
		c.PutSession("_token", getRandomToken(40))
	}

	return matched
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
		regex, ok := compiledRegexCache[pattern]
		if !ok {
			// Compile and cache the regex
			var err error
			regex, err = regexp.Compile(pattern)
			if err != nil {
				slog.Warn("Invalid CSRF exclusion pattern", "pattern", pattern, "error", err)
				continue
			}
			compiledRegexCache[pattern] = regex
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
			if c.WantsHTML() && !strings.HasPrefix(c.Request().URL.Path, "/static") {
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
					Expires:  time.Now().Add(config.Get("session.lifetime").(time.Duration)),
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
