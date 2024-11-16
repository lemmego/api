package session

import (
	"github.com/alexedwards/scs/v2"
	"sync"
)

const (
	DRIVER_MEMORY = "memory" // Not recommended for production
	DRIVER_FILE   = "file"
	DRIVER_REDIS  = "redis"
)

var session *Session
var once sync.Once

func Get() *Session {
	return session
}

func Set(store scs.Store, cookie scs.SessionCookie) {
	if session == nil {
		once.Do(func() {
			session = newSession(store, cookie)
		})
	}
}

type Session struct {
	*scs.SessionManager
}

func newSession(store scs.Store, cookie scs.SessionCookie) *Session {
	s := scs.New()
	s.Store = store
	s.Cookie = cookie
	return &Session{s}
}
