//go:build ignore

package providers

import (
	"fmt"
	"github.com/lemmego/gpa"
	"github.com/lemmego/gpagorm"

	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
)

func init() {
	app.RegisterService(func(a app.App) error {
		dbConfig := GetConfig()

		provider, err := gpagorm.NewProvider(dbConfig)
		if err != nil {
			return err
		}

		gpa.RegisterDefault(provider)

		return nil
	})
}

func GetConfig(connName ...string) gpa.Config {
	var name string
	if len(connName) > 0 && connName[0] != "" {
		name = connName[0]
	} else {
		name = "default"
	}
	defaultConnection := config.Get(fmt.Sprintf("sql.%s", name))
	connection := config.Get(fmt.Sprintf("sql.connections.%s", defaultConnection))
	driver := connection.(config.M)["driver"].(string)
	database := connection.(config.M)["database"].(string)

	if database == "" || driver == "" {
		panic("database: database and driver must be present")
	}

	dbConfig := gpa.Config{
		Driver:   driver,
		Database: database,
	}

	if driver != "sqlite" {
		dbConfig.Host = config.Get(fmt.Sprintf("database.connections.%s.host", defaultConnection)).(string)
		dbConfig.Port = config.Get(fmt.Sprintf("database.connections.%s.port", defaultConnection)).(int)
		dbConfig.Username = config.Get(fmt.Sprintf("database.connections.%s.user", defaultConnection)).(string)
		dbConfig.Password = config.Get(fmt.Sprintf("database.connections.%s.password", defaultConnection)).(string)
		dbConfig.Options = config.Get(fmt.Sprintf("database.connections.%s.options", defaultConnection)).(config.M)
		//dbConfig.AutoCreate = config.Get(fmt.Sprintf("database.connections.%s.auto_create", defaultConnection)).(bool)
	}

	return dbConfig
}
