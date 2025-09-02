package session

import (
	"github.com/alexedwards/scs/v2"
)

const (
	DriverMemory = "memory" // Not recommended for production
	DriverFile   = "file"
	DriverRedis  = "redis"
)

type Session struct {
	*scs.SessionManager
}

func New(store scs.Store, cookie scs.SessionCookie) *Session {
	s := scs.New()
	s.Store = store
	s.Cookie = cookie
	return &Session{s}
}
