package session_test

import (
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
	"github.com/lemmego/api/db"
	sessionprovider "github.com/lemmego/api/providers/session"
	"github.com/lemmego/api/session"
)

// A driver that is registered but never dialled. sql.Open does not connect, and
// the stores' cleanup sweeps are five minutes apart, so no test here ever
// reaches the wire — which is the point: this exercises store *selection*, and
// the SQL each store writes is scs's to get right, not ours.
type idleDriver struct{}

func (idleDriver) Open(string) (driver.Conn, error) {
	return nil, driver.ErrBadConn
}

func init() {
	sql.Register("lemmego-session-idle", idleDriver{})
}

type fakeConnection struct {
	pool    *sql.DB
	dialect db.Dialect
	name    string
}

func (f *fakeConnection) SQLDB() *sql.DB      { return f.pool }
func (f *fakeConnection) Dialect() db.Dialect { return f.dialect }
func (f *fakeConnection) Name() string        { return f.name }

func appWithConnection(t *testing.T, dialect db.Dialect, name string) app.App {
	t.Helper()
	pool, err := sql.Open("lemmego-session-idle", "")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	a := app.Configure(app.WithConfig(config.M{
		"session": config.M{"driver": session.DriverDatabase},
	}))
	if !db.Register(a, &fakeConnection{pool: pool, dialect: dialect, name: name}) {
		t.Fatal("db.Register() did not register the connection")
	}
	return a
}

// The three dialects scs ships a store for. One name in configuration, the
// product supplied by whichever connector registered the connection — which is
// the whole reason this driver is called "database" and not after one of them.
func TestDatabaseDriverSelectsAStorePerDialect(t *testing.T) {
	for _, dialect := range []db.Dialect{db.Postgres, db.MySQL, db.SQLite} {
		t.Run(string(dialect), func(t *testing.T) {
			a := appWithConnection(t, dialect, string(dialect))

			provider := &sessionprovider.Provider{}
			if err := provider.Provide(a); err != nil {
				t.Fatalf("Provide() with a %s connection error = %v", dialect, err)
			}
			sess := app.Get[*session.Session](a)
			if sess == nil {
				t.Fatal("no session was registered")
			}
			if sess.Store == nil {
				t.Fatal("the session was registered with no store")
			}
		})
	}
}

// This is the failure the error text exists for. Provide runs in LoadProviders
// order, so listing the session provider before the connector means db.Resolve
// finds nothing — and the application would otherwise boot looking healthy and
// sign everybody out, which is indistinguishable from a cookie problem.
func TestDatabaseDriverWithoutAConnectionNamesTheOrdering(t *testing.T) {
	a := app.Configure(app.WithConfig(config.M{
		"session": config.M{"driver": session.DriverDatabase},
	}))

	provider := &sessionprovider.Provider{}
	err := provider.Provide(a)
	if err == nil {
		t.Fatal("Provide() with no registered connection returned no error")
	}
	// The fix, not just the symptom: an error saying only "no connection"
	// sends the reader looking for a broken DSN.
	if !strings.Contains(err.Error(), "LoadProviders") {
		t.Errorf("error does not point at provider ordering: %v", err)
	}
}

// A store issuing another dialect's SQL fails one request at a time, long after
// boot. Refusing here turns that into a startup error that names the
// connection.
func TestDatabaseDriverRefusesADialectWithNoStore(t *testing.T) {
	a := appWithConnection(t, db.SQLServer, "mssql")

	provider := &sessionprovider.Provider{}
	err := provider.Provide(a)
	if err == nil {
		t.Fatal("Provide() with a sqlserver connection returned no error")
	}
	for _, want := range []string{"mssql", string(db.SQLServer)} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q: %v", want, err)
		}
	}
}

// The unsupported-driver message is the only place a reader learns what the
// choices are, so it has to list the one just added.
func TestUnknownDriverErrorListsTheDatabaseDriver(t *testing.T) {
	a := app.Configure(app.WithConfig(config.M{
		"session": config.M{"driver": "memcached"},
	}))

	provider := &sessionprovider.Provider{}
	err := provider.Provide(a)
	if err == nil {
		t.Fatal("Provide() with an unknown driver returned no error")
	}
	if !strings.Contains(err.Error(), session.DriverDatabase) {
		t.Errorf("the supported-driver list omits %q: %v", session.DriverDatabase, err)
	}
}
