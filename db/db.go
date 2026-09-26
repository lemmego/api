// Package db declares the seam between whatever opened the application's SQL
// connection and every framework package that needs to store something in it.
//
// The problem it solves is that there was no single place to ask. The service
// container keys on a concrete type, so resolving "the database" meant naming
// *orm.DB or *gorm.DB or *bun.DB at compile time; the GPA registry keys on a
// vendor string ("GORM", "Bun"), is off by default, and cannot hand out a
// repository through an interface anyway. A framework package needing a table
// therefore had to re-derive a DSN from configuration and open its own pool,
// which is how the queue ended up running against a different database from
// the rest of the application and losing every job on restart.
//
// Exactly one connector registers a Connection during bootstrap; everything
// else asks Resolve. An application with no database at all is normal and
// supported: Resolve reports false and nothing panics.
//
// The seam is deliberately narrow. It answers "which pool, and what dialect",
// and nothing else. Quoting, upserts and RETURNING belong to orm.Dialect and
// to the queue's own dialect layer, which are different interfaces serving
// different needs; unifying them here would turn this into a SQL-generation
// library that serves neither well.
package db

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/lemmego/api/app"
)

// Dialect names a SQL flavour, normalised away from the many spellings that
// appear in configuration and in driver registration.
type Dialect string

const (
	// DialectUnknown means the connector opened something this package does
	// not name. It is a real state, not an error: a caller writing
	// dialect-sensitive SQL must handle it rather than assume a default.
	DialectUnknown Dialect = ""

	SQLite    Dialect = "sqlite"
	MySQL     Dialect = "mysql"
	Postgres  Dialect = "postgres"
	SQLServer Dialect = "sqlserver"
)

// Connection is the application's SQL connection, whichever ORM opened it.
//
// A Connection is borrowed, never owned. The connector that registered it
// closes the pool during shutdown, so callers must not call Close on the
// value returned by SQLDB.
type Connection interface {
	// SQLDB returns the live pool. It is never nil for a registered
	// Connection.
	SQLDB() *sql.DB

	// Dialect reports the SQL flavour, or DialectUnknown when the connector
	// opened something this package does not name.
	Dialect() Dialect

	// Name is the key under sql.connections that this connection was built
	// from ("sqlite", "pgsql", ...). It is for diagnostics and for reading
	// connection-scoped configuration, not for routing: there is one
	// connection, and this reports which one it is.
	Name() string
}

// Resolve returns the application's SQL connection when one is registered.
//
// Reporting false is the ordinary answer for a project scaffolded without a
// database, not a failure. A caller that cannot work without one should say
// so with an error naming the fix, rather than dereferencing the zero value.
func Resolve(a app.App) (Connection, bool) {
	return app.Lookup[Connection](a)
}

// Has reports whether the application has a SQL connection.
func Has(a app.App) bool {
	_, ok := Resolve(a)
	return ok
}

// Register makes c the application's connection, reporting whether it did.
//
// The first connector to register wins and a later one is ignored, so an
// application that deliberately wires two connectors — one ORM for its own
// models, another for a legacy schema — boots with a deterministic answer
// instead of a panic. The rule is that the first connector listed in
// LoadProviders owns the seam, which matches the order providers already run
// in. Connectors call this at the end of Provide.
func Register(a app.App, c Connection) bool {
	return app.RegisterIfAbsent[Connection](a, c)
}

// ParseDialect normalises the driver spellings that appear in configuration
// and in driver registration onto a Dialect, reporting whether it recognised
// the name. An unrecognised name yields DialectUnknown.
func ParseDialect(driver string) (Dialect, bool) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "sqlite", "sqlite3":
		return SQLite, true
	case "mysql", "mariadb":
		return MySQL, true
	case "postgres", "postgresql", "pgsql", "pgx":
		return Postgres, true
	case "sqlserver", "mssql":
		return SQLServer, true
	}
	return DialectUnknown, false
}

// Placeholder renders the nth (1-based) bind parameter for d.
//
// PostgreSQL numbers its parameters and the others do not. This is the one
// piece of dialect-specific SQL that every raw-SQL consumer needs, and the
// only reason a small package writing one INSERT would otherwise have to
// import a whole ORM.
func Placeholder(d Dialect, n int) string {
	if d == Postgres {
		return "$" + strconv.Itoa(n)
	}
	return "?"
}
