package session

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/redisstore"
	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	"github.com/gomodule/redigo/redis"
	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
	"github.com/lemmego/api/session"
)

type Provider struct {
	sess *session.Session
}

func (s *Provider) Provide(a app.App) error {
	var sess *session.Session
	cfg := a.Config()
	sessionDriver := cfg.Get("session.driver")
	cookie, lifetime := sessionOptions(cfg, a.InProduction())

	if sessionDriver == session.DriverMemory {
		sess = session.New(memstore.New(), cookie)
	}

	if sessionDriver == session.DriverFile {
		sess = session.New(session.NewFileSession(cfg.Get("session.files").(string)), cookie)
	}

	if sessionDriver == session.DriverRedis {
		pool := &redis.Pool{
			MaxIdle: 10,
			Dial: func() (redis.Conn, error) {
				conn, err := redis.Dial("tcp", fmt.Sprintf("%s:%d", cfg.Get("keyvalue.connections.redis.host").(string), cfg.Get("keyvalue.connections.redis.port").(int)))
				if err != nil {
					return nil, fmt.Errorf("failed to connect to redis: %v", err)
				}
				return conn, err
			},
		}
		sess = session.New(redisstore.New(pool), cookie)
	}
	if sess == nil {
		return fmt.Errorf("unsupported session driver %q", sessionDriver)
	}
	sess.Lifetime = lifetime
	a.AddService(sess)

	return nil
}

func sessionOptions(c config.Configuration, production bool) (scs.SessionCookie, time.Duration) {
	getString := func(key, fallback string) string {
		if value, ok := c.Get(key).(string); ok {
			return value
		}
		return fallback
	}
	getBool := func(key string, fallback bool) bool {
		value := c.Get(key)
		switch value := value.(type) {
		case bool:
			return value
		case string:
			parsed, err := strconv.ParseBool(value)
			if err == nil {
				return parsed
			}
		}
		return fallback
	}

	expireOnClose := getBool("session.expire_on_close", false)
	persist := getBool("session.persist", !expireOnClose)
	if expireOnClose {
		persist = false
	}

	sameSite := http.SameSiteLaxMode
	switch value := c.Get("session.same_site").(type) {
	case http.SameSite:
		sameSite = value
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "strict":
			sameSite = http.SameSiteStrictMode
		case "none":
			sameSite = http.SameSiteNoneMode
		case "default", "0", "1":
			sameSite = http.SameSiteDefaultMode
		}
	}

	lifetime := 24 * time.Hour
	switch value := c.Get("session.lifetime").(type) {
	case time.Duration:
		lifetime = value
	case string:
		if parsed, err := time.ParseDuration(value); err == nil {
			lifetime = parsed
		}
	}

	return scs.SessionCookie{
		Name:     getString("session.cookie", "session"),
		Domain:   getString("session.domain", ""),
		HttpOnly: getBool("session.http_only", true),
		Path:     getString("session.path", "/"),
		Persist:  persist,
		SameSite: sameSite,
		Secure:   getBool("session.secure", production),
	}, lifetime
}

func Get(a app.App) *session.Session {
	return app.Get[*session.Session](a)
	// return a.Service(reflect.TypeOf(&session.Session{})).(*session.Session)
}
