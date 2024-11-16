package providers

import (
	"fmt"
	"github.com/lemmego/api/app"
	"github.com/lemmego/api/db"
	"time"
)

func init() {
	app.RegisterService(func(a app.App) error {
		defaultConnection := a.Config().Get("database.default")
		dbConfig := &db.Config{
			ConnName:   defaultConnection.(string),
			Driver:     a.Config().Get(fmt.Sprintf("database.connections.%s.driver", defaultConnection)).(string),
			Host:       a.Config().Get(fmt.Sprintf("database.connections.%s.host", defaultConnection)).(string),
			Port:       a.Config().Get(fmt.Sprintf("database.connections.%s.port", defaultConnection)).(int),
			Database:   a.Config().Get(fmt.Sprintf("database.connections.%s.database", defaultConnection)).(string),
			User:       a.Config().Get(fmt.Sprintf("database.connections.%s.user", defaultConnection)).(string),
			Password:   a.Config().Get(fmt.Sprintf("database.connections.%s.password", defaultConnection)).(string),
			Params:     a.Config().Get(fmt.Sprintf("database.connections.%s.params", defaultConnection)).(string),
			AutoCreate: a.Config().Get(fmt.Sprintf("database.connections.%s.auto_create", defaultConnection)).(bool),
		}

		c, err := db.NewConnection(dbConfig).Open()

		if err != nil {
			panic(err)
		}

		sqlDB := c.SqlDB()
		// Based on expected peak concurrency and server capacity
		sqlDB.SetMaxOpenConns(a.Config().Get(fmt.Sprintf("database.connections.%s.max_open_conns", defaultConnection)).(int))
		// Enough to handle rapid requests but not too many to consume excess resources
		sqlDB.SetMaxIdleConns(a.Config().Get(fmt.Sprintf("database.connections.%s.max_idle_conns", defaultConnection)).(int))
		// Connections will be closed after an hour of being open
		sqlDB.SetConnMaxLifetime(a.Config().Get(fmt.Sprintf("database.connections.%s.conn_max_lifetime", defaultConnection)).(time.Duration))

		a.AddService(db.AddConnection(c))
		return nil
	})
}
