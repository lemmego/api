package session

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
	"reflect"
)

type Provider struct {
	sess *session.Session
}

func (s *Provider) Provide(a app.App) error {
	var sess *session.Session
	sessionDriver := a.Config().Get("session.driver")
	cookie := scs.SessionCookie{
		Name:     a.Config().Get("session.cookie").(string),
		Domain:   a.Config().Get("session.domain").(string),
		HttpOnly: a.Config().Get("session.http_only").(bool),
		Path:     a.Config().Get("session.path").(string),
		Persist:  false,
		SameSite: a.Config().Get("session.same_site").(http.SameSite),
		Secure:   a.Config().Get("session.secure").(bool),
	}

	if sessionDriver == session.DriverMemory {
		sess = session.New(memstore.New(), cookie)
	}

	if sessionDriver == session.DriverFile {
		sess = session.New(session.NewFileSession(a.Config().Get("session.files").(string)), cookie)
	}

	if sessionDriver == session.DriverRedis {
		pool := &redis.Pool{
			MaxIdle: 10,
			Dial: func() (redis.Conn, error) {
				conn, err := redis.Dial("tcp", fmt.Sprintf("%s:%d", config.Get("keyvalue.connections.redis.host").(string), config.Get("keyvalue.connections.redis.port").(int)))
				if err != nil {
					return nil, fmt.Errorf("failed to connect to redis: %v", err)
				}
				return conn, err
			},
		}
		sess = session.New(redisstore.New(pool), cookie)
	}
	a.AddService(sess)

	return nil
}

func Get(a app.App) *session.Session {
	return a.Service(reflect.TypeOf(&session.Session{})).(*session.Session)
}
