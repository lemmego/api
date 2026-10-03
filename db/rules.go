package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lemmego/api/app"
)

// Unique reports whether a column does not already hold a value.
//
//	v.Field("handle", i.Handle).Required().Check(db.Unique(i.ctx.App(), "users", "handle"))
//
// It lives here rather than on the validator because the validator is in
// api/app and this needs a connection, which api/db resolves — and api/db
// imports api/app, so the dependency can only point one way. The seam is
// vField.Check, which exists so a rule can report that it could not run.
//
// except excludes rows by primary key, for an update: a profile keeping its
// own handle must not collide with itself.
//
//	db.Unique(c.App(), "users", "handle", user.ID)
//
// Identifiers are not escaped and must not come from a request. They are
// written into the SQL, because a placeholder cannot stand in for a table or
// column name in any dialect — so this validates them and refuses anything
// that is not a plain identifier rather than trusting the caller.
func Unique(a app.App, table, column string, except ...any) func(any) (bool, string, error) {
	return func(value any) (bool, string, error) {
		found, err := countRows(a, table, column, value, except...)
		if err != nil {
			return false, "This value could not be checked", err
		}
		if found > 0 {
			return false, "This " + humanise(column) + " is already taken", nil
		}
		return true, "", nil
	}
}

// Exists reports whether a column already holds a value — the mirror of
// Unique, for a foreign key supplied by a client.
func Exists(a app.App, table, column string) func(any) (bool, string, error) {
	return func(value any) (bool, string, error) {
		found, err := countRows(a, table, column, value)
		if err != nil {
			return false, "This value could not be checked", err
		}
		if found == 0 {
			return false, "This " + humanise(column) + " does not exist", nil
		}
		return true, "", nil
	}
}

func countRows(a app.App, table, column string, value any, except ...any) (int, error) {
	if err := validIdentifier(table); err != nil {
		return 0, fmt.Errorf("db: table name: %w", err)
	}
	if err := validIdentifier(column); err != nil {
		return 0, fmt.Errorf("db: column name: %w", err)
	}

	// A nil app is what a unit test passes, and Resolve dereferences it.
	// Reporting rather than panicking matters because this is a validation
	// path: a panic here takes down a request that was only trying to check
	// whether a handle was free.
	if a == nil {
		return 0, ErrNoConnection
	}
	connection, ok := Resolve(a)
	if !ok || connection == nil {
		return 0, ErrNoConnection
	}
	sqlDB := connection.SQLDB()
	if sqlDB == nil {
		return 0, ErrNoConnection
	}

	dialect := connection.Dialect()
	quoted := quoteIdentifier(dialect, table)
	quotedColumn := quoteIdentifier(dialect, column)

	args := []any{value}
	query := "SELECT COUNT(*) FROM " + quoted + " WHERE " + quotedColumn + " = " + Placeholder(dialect, 1)

	// A soft-deleted row still occupies a unique index, so it has to count as
	// taken. Excluding it here would let a sign-up pass validation and then
	// fail on the constraint, which is a 500 rather than a message.
	if len(except) > 0 {
		query += " AND " + quoteIdentifier(dialect, "id") + " <> " + Placeholder(dialect, 2)
		args = append(args, except[0])
	}

	var count int
	err := sqlDB.QueryRowContext(context.Background(), query, args...).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	return count, nil
}

// validIdentifier refuses anything that is not a bare identifier.
//
// A table or column name cannot be a bind parameter in any dialect, so it is
// interpolated — which means this is the only thing standing between a
// mistaken caller and an injection. It is deliberately strict: letters,
// digits and underscores, not starting with a digit.
func validIdentifier(name string) error {
	if name == "" {
		return ErrBadIdentifier
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case r >= '0' && r <= '9':
			if i == 0 {
				return ErrBadIdentifier
			}
		default:
			return ErrBadIdentifier
		}
	}
	return nil
}

func quoteIdentifier(d Dialect, name string) string {
	switch d {
	case MySQL:
		return "`" + name + "`"
	default:
		return `"` + name + `"`
	}
}

// humanise turns a column name into something a message can show.
func humanise(column string) string {
	return strings.ReplaceAll(column, "_", " ")
}

// ErrNoConnection means no database connection is registered, so nothing can
// be checked. It is reported rather than treated as "not taken": passing
// validation because the database is absent would let a duplicate through to
// the constraint, which answers 500 instead of a message.
var ErrNoConnection = errors.New("db: no connection is registered")

// ErrBadIdentifier means a table or column name was not a bare identifier.
var ErrBadIdentifier = errors.New("db: a table or column name must be letters, digits and underscores")
