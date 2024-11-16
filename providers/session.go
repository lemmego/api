package providers

import (
	"fmt"
	"github.com/alexedwards/scs/redisstore"
	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	"github.com/gomodule/redigo/redis"
	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
	"github.com/lemmego/api/session"
	"net/http"
)

func init() {
	app.RegisterService(func(a app.App) error {
		sessionConfig := a.Config().Get("session")
		sessionDriver := a.Config().Get("session.driver")
		cookie := scs.SessionCookie{
			Name:     sessionConfig.(config.M)["cookie"].(string),
			Domain:   sessionConfig.(config.M)["domain"].(string),
			HttpOnly: sessionConfig.(config.M)["http_only"].(bool),
			Path:     sessionConfig.(config.M)["path"].(string),
			Persist:  true,
			SameSite: sessionConfig.(config.M)["same_site"].(http.SameSite),
			Secure:   sessionConfig.(config.M)["secure"].(bool),
		}

		if sessionDriver == session.DRIVER_MEMORY {
			session.Set(memstore.New(), cookie)
		}

		if sessionDriver == session.DRIVER_FILE {
			session.Set(session.NewFileSession(sessionConfig.(config.M)["files"].(string)), cookie)
		}

		if sessionDriver == session.DRIVER_REDIS {
			pool := &redis.Pool{
				MaxIdle: 10,
				Dial: func() (redis.Conn, error) {
					conn, err := redis.Dial("tcp", fmt.Sprintf("%s:%d", config.Get("database.redis.connections.default.host").(string), config.Get("database.redis.connections.default.port").(int)))
					if err != nil {
						return nil, fmt.Errorf("failed to connect to redis: %v", err)
					}
					return conn, err
				},
			}
			session.Set(redisstore.New(pool), cookie)
		}

		a.AddService(session.Get())
		return nil
	})
}
