// Package session provides HTTP session management for the Lemmego framework.
//
// It wraps the scs (Session Cookie Store) library to provide session functionality
// with support for multiple storage backends including memory, file, and Redis.
// The session system handles secure cookie management and session data persistence.
package session

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
)

const (
	// DriverMemory uses in-memory storage (not recommended for production)
	DriverMemory = "memory"
	// DriverFile uses file-based storage for session data
	DriverFile = "file"
	// DriverRedis uses Redis for session storage
	DriverRedis = "redis"
)

// Session wraps the scs.SessionManager to provide session functionality.
// It manages session lifecycle, cookie handling, and data persistence.
type Session struct {
	*scs.SessionManager
}

func New(store scs.Store, cookie scs.SessionCookie) *Session {
	s := scs.New()
	s.Store = store
	s.Cookie = cookie
	return &Session{s}
}

// GetAs retrieves a session value and asserts it to T.
func (s *Session) GetAs[T any](ctx context.Context, key string) (T, bool) {
	value, ok := s.Get(ctx, key).(T)
	return value, ok
}

// PopAs retrieves and removes a session value, asserting it to T.
func (s *Session) PopAs[T any](ctx context.Context, key string) (T, bool) {
	value, ok := s.Pop(ctx, key).(T)
	return value, ok
}

// LoadAndSave is scs's middleware with one behaviour changed: a session whose
// stored data cannot be decoded is treated as no session at all, rather than
// as a server error.
//
// scs answers 500 and returns *without calling the handler* when Load fails,
// and nothing here sets an ErrorFunc, so the default is http.Error. One
// undecodable record therefore locks that visitor out of every page —
// including the login page they would need to recover — for the lifetime of
// the session, with no way out but clearing cookies.
//
// A record becomes undecodable for ordinary reasons: it is gob-encoded as a
// unit, so a deploy that stops registering a type stored in it invalidates
// every session holding one. A browser presenting a credential this server
// can no longer read should be treated as a browser presenting no credential.
func (s *Session) LoadAndSave(next http.Handler) http.Handler {
	inner := s.SessionManager.LoadAndSave(next)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(s.Cookie.Name)
		if err != nil || cookie.Value == "" {
			inner.ServeHTTP(w, r)
			return
		}

		if _, err := s.Load(r.Context(), cookie.Value); err != nil {
			slog.Warn("session: discarding a session whose data could not be read",
				"error", err, "cookie", s.Cookie.Name)
			// Clear it on the way out so the browser stops presenting it, and
			// hide it from scs so this request proceeds anonymously.
			//
			// The cookie is written by hand rather than through
			// WriteSessionCookie, which reads the session's "modified" flag
			// out of the context and panics when there is none — which is
			// precisely the situation here, since Load failed and returned no
			// context.
			s.clearSessionCookie(w)
			inner.ServeHTTP(w, withoutCookie(r, s.Cookie.Name))
			return
		}

		inner.ServeHTTP(w, r)
	})
}

// clearSessionCookie expires the session cookie using the manager's own
// cookie settings, so the browser drops it rather than presenting an
// unreadable credential on every subsequent request.
func (s *Session) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.Cookie.Name,
		Value:    "",
		Path:     s.Cookie.Path,
		Domain:   s.Cookie.Domain,
		Secure:   s.Cookie.Secure,
		HttpOnly: s.Cookie.HttpOnly,
		SameSite: s.Cookie.SameSite,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
}

// withoutCookie returns a copy of r carrying every cookie except name.
//
// The request is rebuilt rather than mutated: a handler further down may hold
// the original, and the Cookie header is the only place scs looks.
func withoutCookie(r *http.Request, name string) *http.Request {
	kept := r.Cookies()[:0]
	for _, cookie := range r.Cookies() {
		if cookie.Name != name {
			kept = append(kept, cookie)
		}
	}

	clone := r.Clone(r.Context())
	clone.Header.Del("Cookie")
	for _, cookie := range kept {
		clone.AddCookie(cookie)
	}
	return clone
}
