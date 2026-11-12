package db

import (
	"errors"
	"github.com/lemmego/api/app"
	"testing"
)

// A table or column name cannot be a bind parameter in any dialect, so it is
// interpolated into the SQL — which makes this check the only thing between a
// mistaken caller and an injection.
func TestValidIdentifierRefusesAnythingButABareIdentifier(t *testing.T) {
	for _, name := range []string{
		"users", "user_skills", "handle", "_private", "a1",
	} {
		if err := validIdentifier(name); err != nil {
			t.Errorf("validIdentifier(%q) = %v, want it accepted", name, err)
		}
	}

	for _, name := range []string{
		"",
		"1users",
		"users; DROP TABLE users",
		"users WHERE 1=1",
		`users"`,
		"users'",
		"users--",
		"users\x00",
		"user-skills",
		"public.users",
		"users ",
		"*",
	} {
		if err := validIdentifier(name); err == nil {
			t.Errorf("validIdentifier(%q) was accepted", name)
		}
	}
}

// Without a connection the rule must report that it could not check, not
// that the value is free. Passing validation because the database is absent
// lets a duplicate reach the unique index, which answers 500 instead of a
// message the person can act on.
func TestUniqueReportsAMissingConnection(t *testing.T) {
	ok, message, err := Unique(nil, "users", "handle")("ada")
	if ok {
		t.Error("Unique passed with no connection")
	}
	if !errors.Is(err, ErrNoConnection) {
		t.Errorf("err = %v, want ErrNoConnection", err)
	}
	if message == "" {
		t.Error("no message for the user")
	}
}

func TestExistsReportsAMissingConnection(t *testing.T) {
	ok, _, err := Exists(nil, "users", "id")(1)
	if ok {
		t.Error("Exists passed with no connection")
	}
	if !errors.Is(err, ErrNoConnection) {
		t.Errorf("err = %v, want ErrNoConnection", err)
	}
}

// A bad identifier is the caller's bug and must surface as an error rather
// than a silently malformed query.
func TestRulesRefuseABadIdentifier(t *testing.T) {
	if _, _, err := Unique(nil, "users; DROP TABLE users", "handle")("x"); !errors.Is(err, ErrBadIdentifier) {
		t.Errorf("err = %v, want ErrBadIdentifier", err)
	}
	if _, _, err := Unique(nil, "users", "handle'")("x"); !errors.Is(err, ErrBadIdentifier) {
		t.Errorf("err = %v, want ErrBadIdentifier", err)
	}
}

func TestQuoteIdentifierPerDialect(t *testing.T) {
	if got := quoteIdentifier(MySQL, "users"); got != "`users`" {
		t.Errorf("MySQL quoting = %s", got)
	}
	if got := quoteIdentifier(Postgres, "users"); got != `"users"` {
		t.Errorf("Postgres quoting = %s", got)
	}
	if got := quoteIdentifier(SQLite, "users"); got != `"users"` {
		t.Errorf("SQLite quoting = %s", got)
	}
}

// TestTheRulesReturnKeysTheValidatorCanResolve replaces a test of humanise,
// which these two rules were the only callers of.
//
// They used to build their own English — "This " + humanise(column) + " is
// already taken" — from the *column* name. They return a message key now, and
// the validator binds :attribute from the *field* name, which is the one the
// reader is looking at and only coincidentally the same string.
//
// The keys have to exist in the framework's own English or they would render
// as themselves, which is what this asserts.
func TestTheRulesReturnKeysTheValidatorCanResolve(t *testing.T) {
	builtin := app.BuiltinValidationMessages()
	for _, key := range []string{
		"validation.unique",
		"validation.exists",
		"validation.uncheckable",
	} {
		text, ok := builtin[key]
		if !ok {
			t.Errorf("%s is not in the framework's own English, so it would reach a "+
				"reader as the raw key", key)
			continue
		}
		if text == "" {
			t.Errorf("%s is empty", key)
		}
	}

	// And the rules really do return those keys rather than prose.
	rule := Unique(nil, "users", "email")
	_, message, err := rule("someone@example.com")
	if err == nil {
		t.Fatal("Unique with no connection should report that it could not check")
	}
	if message != "validation.uncheckable" {
		t.Errorf("message = %q, want the key", message)
	}
}
