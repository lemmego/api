package providers

import (
	"fmt"
	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
	"github.com/lemmego/api/db"
	"time"
)

func init() {
	app.RegisterService(func(a app.App) error {
		defaultConnection := a.Config().Get("database.default")
		connection := a.Config().Get(fmt.Sprintf("database.connections.%s", defaultConnection))
		driver := connection.(config.M)["driver"].(string)
		database := connection.(config.M)["database"].(string)

		if database == "" || driver == "" {
			panic("database: database and driver must be present")
		}

		dbConfig := &db.Config{
			ConnName: defaultConnection.(string),
			Driver:   driver,
			Database: database,
		}

		if driver != db.DialectSQLite {
			dbConfig.Host = a.Config().Get(fmt.Sprintf("database.connections.%s.host", defaultConnection)).(string)
			dbConfig.Port = a.Config().Get(fmt.Sprintf("database.connections.%s.port", defaultConnection)).(int)
			dbConfig.User = a.Config().Get(fmt.Sprintf("database.connections.%s.user", defaultConnection)).(string)
			dbConfig.Password = a.Config().Get(fmt.Sprintf("database.connections.%s.password", defaultConnection)).(string)
			dbConfig.Params = a.Config().Get(fmt.Sprintf("database.connections.%s.params", defaultConnection)).(string)
			dbConfig.AutoCreate = a.Config().Get(fmt.Sprintf("database.connections.%s.auto_create", defaultConnection)).(bool)
		}

		c, err := db.NewConnection(dbConfig).Open()

		if err != nil {
			panic(err)
		}

		sqlDB := c.SqlDB()

		if driver != "sqlite" && sqlDB != nil {
			// Based on expected peak concurrency and server capacity
			sqlDB.SetMaxOpenConns(a.Config().Get(fmt.Sprintf("database.connections.%s.max_open_conns", defaultConnection)).(int))
			// Enough to handle rapid requests but not too many to consume excess resources
			sqlDB.SetMaxIdleConns(a.Config().Get(fmt.Sprintf("database.connections.%s.max_idle_conns", defaultConnection)).(int))
			// Connections will be closed after an hour of being open
			sqlDB.SetConnMaxLifetime(a.Config().Get(fmt.Sprintf("database.connections.%s.conn_max_lifetime", defaultConnection)).(time.Duration))
		}

		a.AddService(db.AddConnection(c))
		return nil
	})
}
