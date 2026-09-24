package session

import (
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
)

func TestSessionOptionsUseConfiguredValues(t *testing.T) {
	c := config.M{
		"session": config.M{
			"cookie":          "app_session",
			"domain":          "example.test",
			"http_only":       false,
			"path":            "/app",
			"persist":         true,
			"same_site":       http.SameSiteStrictMode,
			"secure":          true,
			"lifetime":        90 * time.Minute,
			"expire_on_close": false,
		},
	}

	cfg := newTestConfig(c)
	cookie, lifetime := sessionOptions(cfg, false)

	if cookie.Name != "app_session" || cookie.Domain != "example.test" || cookie.HttpOnly || cookie.Path != "/app" {
		t.Fatalf("unexpected cookie settings: %+v", cookie)
	}
	if !cookie.Persist || cookie.SameSite != http.SameSiteStrictMode || !cookie.Secure {
		t.Fatalf("unexpected cookie settings: %+v", cookie)
	}
	if lifetime != 90*time.Minute {
		t.Fatalf("expected 90m lifetime, got %v", lifetime)
	}
}

func TestSessionOptionsExpireOnCloseOverridesPersist(t *testing.T) {
	cfg := newTestConfig(config.M{"session": config.M{
		"persist":         true,
		"expire_on_close": true,
	}})

	cookie, _ := sessionOptions(cfg, false)
	if cookie.Persist {
		t.Fatal("expire_on_close should create a session cookie")
	}
}

func TestSessionOptionsUseSaferProductionSecureDefault(t *testing.T) {
	cfg := newTestConfig(config.M{"session": config.M{}})

	productionCookie, _ := sessionOptions(cfg, true)
	developmentCookie, _ := sessionOptions(cfg, false)
	if !productionCookie.Secure || developmentCookie.Secure {
		t.Fatalf("unexpected secure defaults: production=%v development=%v", productionCookie.Secure, developmentCookie.Secure)
	}
}

func TestProviderUsesApplicationConfigForRedis(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			close(accepted)
			conn.Close()
		}
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	config.GetInstance().SetConfigMap(config.M{
		"keyvalue": config.M{"connections": config.M{"redis": config.M{
			"host": "127.0.0.1",
			"port": 1,
		}}},
	})

	a := app.Configure(app.WithConfig(config.M{
		"session": config.M{"driver": "redis"},
		"keyvalue": config.M{"connections": config.M{"redis": config.M{
			"host": "127.0.0.1",
			"port": port,
		}}},
	}))
	if err := (&Provider{}).Provide(a); err != nil {
		t.Fatal(err)
	}

	_, _, _ = Get(a).Store.Find("test")
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatalf("Redis provider did not use application-configured port %s", strconv.Itoa(port))
	}
}

func newTestConfig(values config.M) config.Configuration {
	c := config.New()
	c.SetConfigMap(values)
	return c
}
