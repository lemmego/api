// Package dbtest holds the conformance checks every db.Connection must pass.
//
// Three connectors implement the same seam, and without a shared kit each
// would grow its own near-copy of these assertions that drifts from the
// others. Importing testing from a non-test package follows net/http/httptest.
package dbtest

import (
	"context"
	"testing"

	"github.com/lemmego/api/db"
)

// AssertConnection checks the invariants a registered db.Connection must
// hold: a live pool, a dialect it can name, and the connection name it was
// built from.
func AssertConnection(t testing.TB, c db.Connection) {
	t.Helper()

	if c == nil {
		t.Fatal("connection is nil")
	}

	pool := c.SQLDB()
	if pool == nil {
		t.Fatal("SQLDB returned nil; a registered connection must carry a live pool")
	}
	if err := pool.PingContext(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}

	// An unknown dialect is a legitimate runtime state for a driver this
	// framework does not name, but never for a connector in this repo: it
	// means the connector's driver string did not survive ParseDialect.
	if c.Dialect() == db.DialectUnknown {
		t.Error("Dialect is DialectUnknown; the connector's driver name did not parse")
	}
	if c.Name() == "" {
		t.Error("Name is empty; the connection should report which sql.connections key built it")
	}
}
