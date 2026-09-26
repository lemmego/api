package session_test

import (
	"testing"

	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
	sessionprovider "github.com/lemmego/api/providers/session"
	"github.com/lemmego/api/session"
)

// A provider whose configuration is absent must boot on its defaults. Returning
// an error would be worse than useless here: registerProviders turns one into a
// panic, so "you did not configure me" would crash the application.
func TestProvideWithoutAnySessionConfig(t *testing.T) {
	a := app.Configure(app.WithConfig(config.M{}))

	provider := &sessionprovider.Provider{}
	if err := provider.Provide(a); err != nil {
		t.Fatalf("Provide() with no session config error = %v", err)
	}
	if app.Get[*session.Session](a) == nil {
		t.Fatal("no session was registered")
	}
}

// The file driver needs a directory; an absent one is a default, not a panic.
func TestProvideFileDriverWithoutAPath(t *testing.T) {
	a := app.Configure(app.WithConfig(config.M{
		"session": config.M{"driver": "file"},
	}))

	provider := &sessionprovider.Provider{}
	if err := provider.Provide(a); err != nil {
		t.Fatalf("Provide() error = %v", err)
	}
}

// The redis connection block is written into a scaffolded project only when
// something asks for Redis. Selecting the redis session driver in a project
// without it used to panic on the first request that touched the session,
// because the host and port were asserted inside the pool's dial closure.
func TestProvideRedisDriverWithoutAKeyvalueSection(t *testing.T) {
	a := app.Configure(app.WithConfig(config.M{
		"session": config.M{"driver": "redis"},
	}))

	provider := &sessionprovider.Provider{}
	if err := provider.Provide(a); err != nil {
		t.Fatalf("Provide() error = %v", err)
	}
	if app.Get[*session.Session](a) == nil {
		t.Fatal("no session was registered")
	}
}

func TestProvideRejectsAnUnknownDriver(t *testing.T) {
	a := app.Configure(app.WithConfig(config.M{
		"session": config.M{"driver": "memcached"},
	}))

	err := (&sessionprovider.Provider{}).Provide(a)
	if err == nil {
		t.Fatal("Provide() accepted an unknown driver")
	}
	for _, want := range []string{"memcached", "memory", "file", "redis"} {
		if !contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
