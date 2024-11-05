package app

import (
	"fmt"
	"github.com/lemmego/api/db"
)

type DatabaseProvider struct {
	*ServiceProvider
}

func (provider *DatabaseProvider) Register(a AppManager) {
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

	dbm, err := db.NewDBManager().Add(c)

	if err != nil {
		panic(err)
	}

	a.AddService(dbm)
}

func (provider *DatabaseProvider) Boot(a AppManager) {
	//
}
