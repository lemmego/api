package providers

import (
	"fmt"

	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
	"github.com/lemmego/api/di"
	"github.com/lemmego/db"
)

func init() {
	app.RegisterService(func(a app.App) error {
		dbConfig := GetConfig()
		conn := db.NewConnection(dbConfig)
		_, err := conn.Open()
		if err != nil {
			return fmt.Errorf("database: failed to open connection: %w", err)
		}

		db.DM().Add("default", conn)
		db.DM().Add(dbConfig.ConnName, conn)

		return di.For[*db.DatabaseManager](a.Container()).
			AsSingleton().
			UseInstance(db.DM())
	})
}

func GetConfig(connName ...string) *db.Config {
	var name string
	if len(connName) > 0 && connName[0] != "" {
		name = connName[0]
	} else {
		name = "default"
	}
	defaultConnection := config.Get(fmt.Sprintf("database.%s", name))
	connection := config.Get(fmt.Sprintf("database.connections.%s", defaultConnection))
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
		dbConfig.Host = config.Get(fmt.Sprintf("database.connections.%s.host", defaultConnection)).(string)
		dbConfig.Port = config.Get(fmt.Sprintf("database.connections.%s.port", defaultConnection)).(int)
		dbConfig.User = config.Get(fmt.Sprintf("database.connections.%s.user", defaultConnection)).(string)
		dbConfig.Password = config.Get(fmt.Sprintf("database.connections.%s.password", defaultConnection)).(string)
		dbConfig.Params = config.Get(fmt.Sprintf("database.connections.%s.params", defaultConnection)).(string)
		//dbConfig.AutoCreate = config.Get(fmt.Sprintf("database.connections.%s.auto_create", defaultConnection)).(bool)
	}

	return dbConfig
}
