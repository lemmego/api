package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	DefaultPostgresDB = "postgres"
	DefaultMysqlDB    = "mysql"
)

var (
	ErrUnknownDriver                    = errors.New("unknown driver")
	ErrNoSuchConnection                 = errors.New("no such connection exists")
	ErrNoSuchDatabase                   = errors.New("no such database exists")
	ErrCannotConnectToDefaultConnection = errors.New("cannot connect to default connection")
	ErrCannotConnectToDefaultDB         = errors.New("cannot connect to default db")
)

type DatabaseManager struct {
	connections map[string]*Connection
}

func NewDBManager() *DatabaseManager {
	return &DatabaseManager{connections: make(map[string]*Connection)}
}

func (dm *DatabaseManager) Get(connName ...string) (*Connection, error) {
	var dbConn string
	if len(connName) == 0 {
		dbConn = os.Getenv("DB_CONNECTION")
	} else {
		dbConn = connName[0]
	}

	if dbConn == "" {
		return nil, ErrCannotConnectToDefaultConnection
	}

	if _, ok := dm.connections[dbConn]; ok {
		return dm.connections[dbConn], nil
	}

	return nil, ErrNoSuchConnection
}

func (dm *DatabaseManager) Add(conn *Connection) (*DatabaseManager, error) {
	if dm.HasConnection(conn.ConnName()) {
		return nil, errors.New("dm: connection already exists")
	}
	dm.connections[conn.ConnName()] = conn
	return dm, nil
}

func (dm *DatabaseManager) HasConnection(name string) bool {
	return dm.connections[name] != nil
}

func (dm *DatabaseManager) All() map[string]*Connection {
	return dm.connections
}

func (conn *Connection) BindWhere(c context.Context, columnName string) *gorm.DB {
	return conn.db.Where(fmt.Sprintf("%s = ?", columnName), c.Value(columnName))
}

func (conn *Connection) DB() *gorm.DB {
	return conn.db
}

type Model struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

type Config struct {
	ConnName   string
	Driver     string
	Host       string
	Port       int
	User       string
	Password   string
	Database   string
	Params     string
	AutoCreate bool
}

type Connection struct {
	forceCreateDb bool
	config        *Config
	db            *gorm.DB
}

func NewConnection(dbc *Config) *Connection {
	if dbc.ConnName == "" {
		dbc.ConnName = "sqlite"
	}

	if dbc.Driver != DialectMySQL && dbc.Driver != DialectPostgres && dbc.Driver != DialectSQLite {
		panic("unsupported driver")
	}

	if dbc.Driver != DialectSQLite && dbc.Host == "" {
		dbc.Host = "localhost"
	}

	if dbc.Driver == DialectMySQL && dbc.Port == 0 {
		dbc.Port = 3306
	}

	if dbc.Driver == DialectPostgres && dbc.Port == 0 {
		dbc.Port = 5432
	}

	if dbc.Driver != DialectSQLite && dbc.User == "" {
		panic("db username must be provided")
	}

	if dbc.Driver == DialectSQLite && dbc.Database == "" {
		panic("a path to database file must be provided for sqlite")
	}

	return &Connection{dbc.AutoCreate, dbc, nil}
}

func (c *Connection) Driver() string {
	return c.config.Driver
}

func (c *Connection) ConnName() string {
	return c.config.ConnName
}

func (c *Connection) DBName() string {
	return c.config.Database
}

func (c *Connection) DBHost() string {
	return c.config.Host
}

func (c *Connection) DBPort() int {
	return c.config.Port
}

func (c *Connection) DBUser() string {
	return c.config.User
}

func (c *Connection) DBPassword() string {
	return c.config.Password
}

func (c *Connection) DBParams() string {
	return c.config.Params
}

func (c *Connection) WithForceCreateDb() *Connection {
	c.forceCreateDb = true
	return c
}

func (c *Connection) IsOpen() bool {
	if c.db == nil {
		return false
	}

	sqlDB, err := c.db.DB()

	if err != nil {
		return false
	}

	if err := sqlDB.Ping(); err != nil {
		return false
	}

	return true
}

func (c *Connection) WithDatabase(database string) *Connection {
	c.config.Database = database
	return c
}

func (c *Connection) connectToMySQL() error {
	dbConfig := c.config
	dsn := &DataSource{
		Dialect:  DialectMySQL,
		Host:     dbConfig.Host,
		Port:     strconv.Itoa(dbConfig.Port),
		Username: dbConfig.User,
		Password: dbConfig.Password,
		Name:     dbConfig.Database,
		Params:   dbConfig.Params,
	}
	dsnStr, err := dsn.String()
	db, err := gorm.Open(mysql.Open(dsnStr), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	slog.Info(fmt.Sprintf("created db session %s", dsn.Name))

	c.db = db
	return nil
}

func (c *Connection) connectToPostgres() error {
	dbConfig := c.config
	dsn := &DataSource{
		Dialect:  DialectPostgres,
		Host:     dbConfig.Host,
		Port:     strconv.Itoa(dbConfig.Port),
		Username: dbConfig.User,
		Password: dbConfig.Password,
		Name:     dbConfig.Database,
		Params:   dbConfig.Params,
	}
	dsnStr, err := dsn.String()
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	db, err := gorm.Open(postgres.Open(dsnStr), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	slog.Info(fmt.Sprintf("created db session %s", dsn.Name))

	c.db = db
	return nil
}

func (c *Connection) Close() error {
	sqlDB, err := c.db.DB()

	if err != nil {
		return errors.New("failed to close db connection")
	}

	err = sqlDB.Close()
	if err != nil {
		return err
	}
	return nil
}

func (c *Connection) existsDb() error {
	var db *gorm.DB
	var err error
	dbConfig := c.config
	database := c.config.Database

	defer func() {
		c.WithDatabase(database)
		if db != nil {
			slog.Info(fmt.Sprintf("closing db session %s", db.Name()))
			sqlDB, err := db.DB()

			if err != nil {
				slog.Error(fmt.Sprintf("failed to fetch db session %s", db.Name()))
			}

			err = sqlDB.Close()
			if err != nil {
				slog.Error(fmt.Sprintf("failed to close db session %s", db.Name()))
			}
		}
	}()

	if dbConfig.Driver == DialectPostgres {
		err = c.WithDatabase(DefaultPostgresDB).connectToPostgres()
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		db = c.db

		var fetchedDatabase string
		db.Raw("SELECT datname FROM pg_catalog.pg_database WHERE lower(datname) = lower(?)", database).Scan(&fetchedDatabase)
		if fetchedDatabase == "" {
			return ErrCannotConnectToDefaultDB
		}
		if fetchedDatabase != database {
			return ErrNoSuchDatabase
		}
		return nil
	}

	if dbConfig.Driver == DialectMySQL {
		err = c.WithDatabase(DefaultMysqlDB).connectToMySQL()
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		db = c.db

		var fetchedDatabase string
		db.Raw("SELECT SCHEMA_NAME FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = ?", database).Scan(&fetchedDatabase)
		if fetchedDatabase == "" {
			return ErrCannotConnectToDefaultDB
		}
		if fetchedDatabase != database {
			return ErrNoSuchDatabase
		}
		return nil
	}

	return ErrUnknownDriver
}

func (c *Connection) Open() (*Connection, error) {
	if c.IsOpen() {
		slog.Info(fmt.Sprintf("closing db session %s, before opening a new one", c.config.Database))
		if err := c.Close(); err != nil {
			return nil, err
		}
	}

	if c.forceCreateDb {
		err := c.existsDb()
		if err != nil && errors.Is(err, ErrNoSuchDatabase) {
			if err = c.createDb(); err != nil {
				return nil, err
			}
		}
	}

	switch c.config.Driver {
	case DialectMySQL:
		err := c.connectToMySQL()
		if err != nil {
			return nil, err
		}
		//dbInstances[c.config.ConnName] = db
		return c, nil
	case DialectPostgres:
		err := c.connectToPostgres()
		if err != nil {
			return nil, err
		}
		//dbInstances[c.config.ConnName] = db
		return c, nil
	default:
		return nil, ErrUnknownDriver
	}
}

func (c *Connection) createDb() error {
	dbConfig := c.config
	database := dbConfig.Database
	var db *gorm.DB
	var err error

	defer func() {
		c.WithDatabase(database)
		if db != nil {
			slog.Info(fmt.Sprintf("closing db session %s", db.Name()))
			sqlDB, err := db.DB()

			if err != nil {
				slog.Error(fmt.Sprintf("failed to fetch db session %s", db.Name()))
			}

			err = sqlDB.Close()
			if err != nil {
				slog.Error(fmt.Sprintf("failed to close db session %s", db.Name()))
			}
		}
	}()

	if dbConfig.Driver == DialectPostgres {
		if _, err = c.WithDatabase(DefaultPostgresDB).Open(); err != nil {
			return err
		}
		db = c.db
		err = db.Exec("CREATE DATABASE " + database + " WITH OWNER " + dbConfig.User).Error
		if err != nil {
			return err
		} else {
			slog.Info("database", database, "created")
			return nil
		}
	}

	if dbConfig.Driver == DialectMySQL {
		if _, err = c.WithDatabase(DefaultMysqlDB).Open(); err != nil {
			return err
		}
		db = c.db
		err = db.Exec("CREATE DATABASE IF NOT EXISTS " + database).Error
		if err != nil {
			return err
		} else {
			slog.Info("database", database, "created")
			return nil
		}
	}

	return nil
}
