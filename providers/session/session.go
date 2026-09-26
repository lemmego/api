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

// defaultSessionFiles is where a file-backed session stores its data when the
// configuration does not say.
const defaultSessionFiles = "./storage/session"

// Default Redis connection settings, used when the keyvalue configuration is
// absent. A project scaffolded without Redis has no such section, and
// SESSION_DRIVER=redis in that project used to panic on the first request that
// touched the session rather than failing anywhere useful.
const (
	defaultRedisHost = "localhost"
	defaultRedisPort = 6379
)

func (s *Provider) Provide(a app.App) error {
	var sess *session.Session
	cfg := a.Config()
	cookie, lifetime := sessionOptions(cfg, a.InProduction())

	// An absent driver means an absent session configuration, which is the
	// default rather than an error: the file store needs nothing but a
	// directory.
	sessionDriver, _ := cfg.Get("session.driver").(string)
	if sessionDriver == "" {
		sessionDriver = session.DriverFile
	}

	switch sessionDriver {
	case session.DriverMemory:
		sess = session.New(memstore.New(), cookie)

	case session.DriverFile:
		files, ok := cfg.Get("session.files").(string)
		if !ok || files == "" {
			files = defaultSessionFiles
		}
		sess = session.New(session.NewFileSession(files), cookie)

	case session.DriverRedis:
		host, ok := cfg.Get("keyvalue.connections.redis.host").(string)
		if !ok || host == "" {
			host = defaultRedisHost
		}
		port, ok := cfg.Get("keyvalue.connections.redis.port").(int)
		if !ok || port == 0 {
			port = defaultRedisPort
		}
		password, _ := cfg.Get("keyvalue.connections.redis.password").(string)
		address := fmt.Sprintf("%s:%d", host, port)

		pool := &redis.Pool{
			MaxIdle: 10,
			Dial: func() (redis.Conn, error) {
				var opts []redis.DialOption
				// The password was configured but never sent, so a session
				// against an authenticated Redis could not connect.
				if password != "" {
					opts = append(opts, redis.DialPassword(password))
				}
				conn, err := redis.Dial("tcp", address, opts...)
				if err != nil {
					return nil, fmt.Errorf("session: connecting to redis at %s: %w", address, err)
				}
				return conn, nil
			},
		}
		sess = session.New(redisstore.New(pool), cookie)

	default:
		return fmt.Errorf("session: unsupported driver %q (supported: %s, %s, %s)",
			sessionDriver, session.DriverMemory, session.DriverFile, session.DriverRedis)
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
