package db

import (
	"database/sql"
	"testing"

	"github.com/lemmego/api/app"
)

type fakeConnection struct {
	name    string
	dialect Dialect
}

func (f *fakeConnection) SQLDB() *sql.DB   { return nil }
func (f *fakeConnection) Dialect() Dialect { return f.dialect }
func (f *fakeConnection) Name() string     { return f.name }

// A project scaffolded without a database is ordinary, not broken. This is
// the case the whole seam exists to make survivable, so it is asserted here
// rather than in any one connector: "no database" is the absence of a
// connector, which no connector's own tests can express.
func TestResolveWithNoDatabase(t *testing.T) {
	a := app.Configure()

	conn, ok := Resolve(a)
	if ok {
		t.Fatalf("Resolve reported a connection (%v) in an application that has none", conn)
	}
	if conn != nil {
		t.Fatalf("Resolve returned %v alongside ok == false; want nil", conn)
	}
	if Has(a) {
		t.Fatal("Has reported a connection in an application that has none")
	}
}

func TestRegisterIsFirstWins(t *testing.T) {
	a := app.Configure()
	first := &fakeConnection{name: "primary", dialect: SQLite}

	if !Register(a, first) {
		t.Fatal("the first Register should report that it stored the connection")
	}
	if Register(a, &fakeConnection{name: "secondary", dialect: Postgres}) {
		t.Fatal("a second Register should report that it stored nothing")
	}

	conn, ok := Resolve(a)
	if !ok {
		t.Fatal("Resolve found nothing after a successful Register")
	}
	if conn.Name() != "primary" {
		t.Fatalf("Resolve returned %q; the first connector registered should win", conn.Name())
	}
}

func TestParseDialect(t *testing.T) {
	for _, tc := range []struct {
		driver string
		want   Dialect
		known  bool
	}{
		{"sqlite", SQLite, true},
		{"sqlite3", SQLite, true},
		{"SQLite", SQLite, true},
		{"  sqlite  ", SQLite, true},
		{"mysql", MySQL, true},
		{"mariadb", MySQL, true},
		{"postgres", Postgres, true},
		{"postgresql", Postgres, true},
		{"pgsql", Postgres, true},
		{"pgx", Postgres, true},
		{"sqlserver", SQLServer, true},
		{"mssql", SQLServer, true},
		{"", DialectUnknown, false},
		{"oracle", DialectUnknown, false},
		{"mongodb", DialectUnknown, false},
	} {
		got, known := ParseDialect(tc.driver)
		if got != tc.want || known != tc.known {
			t.Errorf("ParseDialect(%q) = %q, %v; want %q, %v", tc.driver, got, known, tc.want, tc.known)
		}
	}
}

func TestPlaceholder(t *testing.T) {
	if got := Placeholder(Postgres, 1); got != "$1" {
		t.Errorf("Placeholder(Postgres, 1) = %q, want %q", got, "$1")
	}
	if got := Placeholder(Postgres, 12); got != "$12" {
		t.Errorf("Placeholder(Postgres, 12) = %q, want %q", got, "$12")
	}
	for _, d := range []Dialect{SQLite, MySQL, SQLServer, DialectUnknown} {
		if got := Placeholder(d, 3); got != "?" {
			t.Errorf("Placeholder(%q, 3) = %q, want %q", d, got, "?")
		}
	}
}
