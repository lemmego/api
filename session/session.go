// Package session provides HTTP session management for the Lemmego framework.
//
// It wraps the scs (Session Cookie Store) library to provide session functionality
// with support for multiple storage backends including memory, file, and Redis.
// The session system handles secure cookie management and session data persistence.
package session

import (
	"context"

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
