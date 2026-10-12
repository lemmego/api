package logger

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
	"github.com/lemmego/log"
)

// newApp builds an application the provider can read, with app.env set and
// RunningInConsole decided.
//
// The console flag goes through os.Args because that is where app.Configure
// reads it — len(os.Args) > 1 — and it is read once, at construction. Which
// also means a plain `go test` run is "in console": the testing package's own
// flags are arguments. Tests that care therefore say which they want rather
// than inheriting it.
func newApp(t *testing.T, env string, console bool) app.AppEngine {
	t.Helper()

	saved := os.Args
	if console {
		os.Args = []string{saved[0], "some:command"}
	} else {
		os.Args = []string{saved[0]}
	}
	t.Cleanup(func() { os.Args = saved })

	return app.Configure(app.WithConfig(config.M{
		"app": config.M{"name": "logger-test", "env": env, "port": 0},
	}))
}

// keepDefaultLogger restores the process logger, because installing one is
// the point of this package and a test that leaves it installed changes what
// every later test logs through.
func keepDefaultLogger(t *testing.T) {
	t.Helper()
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
}

// clearEnv removes the variables the provider reads, so a .env in the
// repository cannot decide a test's outcome.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		EnvChannel, EnvLevel, EnvPath, EnvMaxSize, EnvMaxFiles, EnvDaily, EnvSource, "APP_DEBUG",
	} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unsetting %s: %v", key, err)
		}
	}
}

func TestProvideInstallsTheDefaultLogger(t *testing.T) {
	clearEnv(t)
	keepDefaultLogger(t)

	out := &bytes.Buffer{}
	a := newApp(t, "development", false)
	provider := &Provider{Options: log.Options{Channel: log.ChannelConsole, Console: out}}

	if err := provider.Provide(a); err != nil {
		t.Fatalf("Provide: %v", err)
	}

	slog.Info("hello", "shape", "round")

	if got := out.String(); !strings.Contains(got, "hello") || !strings.Contains(got, "shape=round") {
		t.Fatalf("the default logger did not reach the configured writer, got %q", got)
	}

	if _, ok := Logger(a); !ok {
		t.Fatal("the logger was not registered as a service")
	}
}

func TestProvideWritesTheConfiguredFile(t *testing.T) {
	clearEnv(t)
	keepDefaultLogger(t)

	path := filepath.Join(t.TempDir(), "logs", "app.log")
	t.Setenv(EnvPath, path)
	t.Setenv(EnvChannel, log.ChannelFile)

	a := newApp(t, "development", false)
	// Channel comes from the environment here, which only happens because it
	// is also named explicitly: a test boot discards by default, see below.
	provider := &Provider{Options: log.Options{Channel: log.ChannelFile}}
	if err := provider.Provide(a); err != nil {
		t.Fatalf("Provide: %v", err)
	}

	slog.Error("the disk is on fire", "degrees", 451)

	logger, ok := Logger(a)
	if !ok {
		t.Fatal("the logger was not registered as a service")
	}
	if logger.Path() != path {
		t.Fatalf("Path() = %q, want %q", logger.Path(), path)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if !strings.Contains(string(written), "the disk is on fire") {
		t.Fatalf("the record is not in the file, got %q", written)
	}
}

// TestATestBootDiscards is the one that stops a suite appending to the
// application's own log file: apptest boots the real provider list from the
// project root, so without the override every test writes there.
func TestATestBootDiscards(t *testing.T) {
	clearEnv(t)
	keepDefaultLogger(t)

	path := filepath.Join(t.TempDir(), "app.log")
	t.Setenv(EnvChannel, log.ChannelStack)
	t.Setenv(EnvPath, path)

	a := newApp(t, "development", false)
	provider := &Provider{}

	if channel := provider.resolve(a, &environment{}).Channel; channel != log.ChannelNull {
		t.Fatalf("resolve().Channel = %q, want %q even though LOG_CHANNEL says stack",
			channel, log.ChannelNull)
	}

	if err := provider.Provide(a); err != nil {
		t.Fatalf("Provide: %v", err)
	}
	slog.Info("this record has nowhere to go")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a test boot created %s; os.Stat says %v", path, err)
	}
}

func TestAnExplicitChannelBeatsTheTestOverride(t *testing.T) {
	clearEnv(t)

	a := newApp(t, "development", false)
	provider := &Provider{Options: log.Options{Channel: log.ChannelJSON}}

	if channel := provider.resolve(a, &environment{}).Channel; channel != log.ChannelJSON {
		t.Fatalf("resolve().Channel = %q, want %q", channel, log.ChannelJSON)
	}
}

func TestLevelFallsBackToAppDebug(t *testing.T) {
	clearEnv(t)

	a := newApp(t, "development", false)
	provider := &Provider{}

	if level := provider.resolve(a, &environment{}).Level; level != "info" {
		t.Fatalf("resolve().Level = %q, want info with APP_DEBUG unset", level)
	}

	t.Setenv("APP_DEBUG", "true")
	if level := provider.resolve(a, &environment{}).Level; level != "debug" {
		t.Fatalf("resolve().Level = %q, want debug with APP_DEBUG=true", level)
	}

	t.Setenv(EnvLevel, "warn")
	if level := provider.resolve(a, &environment{}).Level; level != "warn" {
		t.Fatalf("resolve().Level = %q, want LOG_LEVEL to beat APP_DEBUG", level)
	}
}

// TestConsoleModeIsQuiet covers what app.Run used to do by replacing the
// default logger, and what it no longer does.
func TestConsoleModeIsQuiet(t *testing.T) {
	clearEnv(t)

	provider := &Provider{}

	if quiet := provider.resolve(newApp(t, "development", true), &environment{}).Quiet; !quiet {
		t.Fatal("a CLI command did not get a quiet console")
	}
	if quiet := provider.resolve(newApp(t, "development", false), &environment{}).Quiet; quiet {
		t.Fatal("a web process got a quiet console")
	}
}

func TestProductionDefaultsToStackAndDevelopmentToConsole(t *testing.T) {
	// Through defaultChannel rather than resolve, because resolve never
	// reaches it under a test binary — the discard override comes first.
	if channel := defaultChannel(newApp(t, "production", false)); channel != log.ChannelStack {
		t.Fatalf("production default = %q, want %q", channel, log.ChannelStack)
	}
	if channel := defaultChannel(newApp(t, "development", false)); channel != log.ChannelConsole {
		t.Fatalf("development default = %q, want %q", channel, log.ChannelConsole)
	}
	// An application with no configuration at all is a provider's problem to
	// survive, not to panic on.
	if channel := defaultChannel(nil); channel != log.ChannelConsole {
		t.Fatalf("default with no app = %q, want %q", channel, log.ChannelConsole)
	}
}

func TestSourceFollowsTheEnvironment(t *testing.T) {
	clearEnv(t)

	provider := &Provider{}
	if source := provider.resolve(newApp(t, "development", false), &environment{}).Source; !source {
		t.Fatal("development did not add source positions")
	}
	if source := provider.resolve(newApp(t, "production", false), &environment{}).Source; source {
		t.Fatal("production added source positions")
	}

	t.Setenv(EnvSource, "true")
	if source := provider.resolve(newApp(t, "production", false), &environment{}).Source; !source {
		t.Fatal("LOG_SOURCE=true was ignored in production")
	}
}

// TestAnUnopenablePathFailsTheBoot is the deliberate choice: a log that
// silently goes nowhere is worse than a process that refuses to start.
func TestAnUnopenablePathFailsTheBoot(t *testing.T) {
	clearEnv(t)
	keepDefaultLogger(t)

	// A regular file where the log directory should be, so MkdirAll cannot
	// succeed.
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("in the way"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", blocked, err)
	}

	a := newApp(t, "development", false)
	provider := &Provider{Options: log.Options{
		Channel: log.ChannelFile,
		Path:    filepath.Join(blocked, "logs", "app.log"),
	}}

	if err := provider.Provide(a); err == nil {
		t.Fatal("Provide accepted a path it cannot write to")
	}
}

// TestAnEmptyVariableMeansTheDefault is the reason this package reads the
// environment itself rather than through config.MustEnv.
//
// A blank `LOG_CHANNEL=` line in a production .env used to pick the console
// and write no file at all — the exact failure the file channel exists to
// prevent, reached through a value nobody would read as a choice. And a blank
// `LOG_MAX_SIZE=` took the process down, because MustEnv panics on a value it
// cannot parse.
func TestAnEmptyVariableMeansTheDefault(t *testing.T) {
	clearEnv(t)

	for _, key := range []string{EnvChannel, EnvLevel, EnvPath, EnvMaxSize, EnvMaxFiles, EnvDaily} {
		t.Setenv(key, "")
	}

	a := newApp(t, "production", false)
	provider := &Provider{Options: log.Options{Channel: log.ChannelFile}}

	env := &environment{}
	opts := provider.resolve(a, env)

	if opts.Level != "info" {
		t.Errorf("Level = %q, want info", opts.Level)
	}
	if opts.Path != log.DefaultPath {
		t.Errorf("Path = %q, want %q", opts.Path, log.DefaultPath)
	}
	if opts.MaxSize != 0 {
		t.Errorf("MaxSize = %d, want 0", opts.MaxSize)
	}
	if !opts.Daily {
		t.Error("Daily = false; an empty LOG_DAILY silently turned dated rotation off")
	}
	if len(env.complaints) != 0 {
		t.Errorf("an empty variable is not a mistake worth complaining about, got %+v", env.complaints)
	}

	// And the one that matters most, asserted on the read rather than
	// through resolve: resolve forces ChannelNull under a test binary before
	// it ever looks at LOG_CHANNEL, so a check there would pass whatever this
	// code did.
	if channel := env.str(EnvChannel, log.ChannelStack); channel != log.ChannelStack {
		t.Errorf("an empty LOG_CHANNEL read as %q; in production that writes no file at all", channel)
	}
}

// TestAnUnreadableVariableComplainsAndCarriesOn covers the other half: a typo
// should be visible, and should not be fatal.
func TestAnUnreadableVariableComplainsAndCarriesOn(t *testing.T) {
	clearEnv(t)

	t.Setenv(EnvDaily, "yesterday")
	t.Setenv(EnvMaxSize, "32MB")

	a := newApp(t, "development", false)
	env := &environment{}
	opts := (&Provider{}).resolve(a, env)

	if !opts.Daily {
		t.Error("an unreadable LOG_DAILY did not fall back to the default")
	}
	if opts.MaxSize != 0 {
		t.Errorf("MaxSize = %d, want the default", opts.MaxSize)
	}

	if len(env.complaints) != 2 {
		t.Fatalf("want a complaint for each unreadable variable, got %+v", env.complaints)
	}
	named := map[string]bool{}
	for _, c := range env.complaints {
		named[c.variable] = true
		if c.value == "" || c.want == "" {
			t.Errorf("a complaint has to say what was found and what was wanted, got %+v", c)
		}
	}
	for _, key := range []string{EnvDaily, EnvMaxSize} {
		if !named[key] {
			t.Errorf("%s was ignored without a complaint", key)
		}
	}
}
