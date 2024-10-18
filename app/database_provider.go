package app

import (
	"github.com/lemmego/api/db"
)

type DatabaseProvider struct {
	*ServiceProvider
}

func (provider *DatabaseProvider) Register(a AppManager) {
	dbConfig := &db.Config{
		ConnName:   "pgsql",
		Driver:     a.Config().Get("database.connections.pgsql.driver").(string),
		Host:       a.Config().Get("database.connections.pgsql.host").(string),
		Port:       a.Config().Get("database.connections.pgsql.port").(int),
		Database:   a.Config().Get("database.connections.pgsql.database").(string),
		User:       a.Config().Get("database.connections.pgsql.user").(string),
		Password:   a.Config().Get("database.connections.pgsql.password").(string),
		Params:     a.Config().Get("database.connections.pgsql.params").(string),
		AutoCreate: a.Config().Get("database.connections.pgsql.auto_create", false).(bool),
	}

	dbc, err := db.NewConnection(dbConfig).Open()

	if err != nil {
		panic(err)
	}
	provider.App.AddService(dbc)
	//app.Bind((*db.DB)(nil), func() *db.DB {
	//	return dbc
	//})
	//app.SetDB(dbc)
	//app.SetDbFunc(func(c context.Context, config *db.Config) (*db.DB, error) {
	//	if config == nil {
	//		config = dbConfig
	//	}
	//	return db.NewConnection(config).
	//		// WithForceCreateDb(). // Force create db if not exists
	//		Open()
	//})
}

func (provider *DatabaseProvider) Boot(a AppManager) {
	//
}
