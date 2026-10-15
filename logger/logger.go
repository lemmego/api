// Package logger wires github.com/lemmego/log into an application's
// lifecycle.
//
// It is one provider and two helpers. Everything about formatting, rotation,
// channels and terminal detection lives in lemmego/log, which depends on
// nothing but the standard library; this package is the part that has to know
// what an application is — where configuration comes from, when providers
// run, and whether the process is a web server or a CLI command.
//
//	func LoadProviders() []app.Provider {
//	    return []app.Provider{
//	        &logger.Provider{},
//	        // everything else
//	    }
//	}
//
// After that, slog is the API. There is deliberately no logger.Info: a
// package that logs should import log/slog and nothing else, so it can be
// used outside an application too.
package logger

import (
	"flag"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/lemmego/api/app"
	"github.com/lemmego/log"
)

// Environment variables this provider reads.
const (
	EnvChannel  = "LOG_CHANNEL"
	EnvLevel    = "LOG_LEVEL"
	EnvPath     = "LOG_FILE"
	EnvMaxSize  = "LOG_MAX_SIZE"
	EnvMaxFiles = "LOG_MAX_FILES"
	EnvDaily    = "LOG_DAILY"
	EnvSource   = "LOG_SOURCE"
	EnvQuiet    = "LOG_QUIET"
)

// Provider configures slog from the environment and installs it as the
// process default.
//
// **Register it first.** Every other provider logs while it boots, and a
// record emitted before this one runs goes to whatever slog's default was —
// the runtime's plain text handler, which is not the file, not the format and
// not the level the application asked for. Nothing breaks if it is further
// down the list; the boot records just go somewhere else, which is confusing
// in exactly the situation you are reading logs to understand.
//
// It is the single owner of slog.SetDefault. That used to be shared: this
// package installed a logger from init() at import time, and app.Run
// installed a second one for CLI commands, so the effective logger depended
// on an ordering nobody controlled and the loser's configuration was
// discarded in silence. Console quietening is a field on log.Options now —
// see resolve — so the intent survives with one owner instead of two.
type Provider struct {
	// Options overrides the environment. A zero field falls back to the
	// environment, then to the documented default, so a project that wants
	// one thing fixed in code and the rest configurable sets one field.
	Options log.Options
}

// Provide builds the logger and installs it.
func (p *Provider) Provide(a app.App) error {
	env := &environment{}
	opts := p.resolve(a, env)

	logger, err := log.New(opts)
	if err != nil {
		// A file that cannot be opened fails the boot. A log that silently
		// goes nowhere is the failure lemmego/log exists to prevent, and a
		// deployment is better off refusing to start than running
		// unobserved.
		return err
	}

	log.SetDefault(logger)
	a.AddService(logger)

	// After the logger is installed, so a complaint about the configuration
	// appears in the channel the application configured rather than on
	// whatever slog's default happened to be. lemmego/log reports an unknown
	// channel or level the same way, for the same reason.
	env.report()

	// Said once, because "where is the log file" is the first question
	// anybody has and the answer is otherwise in configuration they have to
	// go and find.
	if path := logger.Path(); path != "" {
		slog.Info("log: writing to file", "path", path, "channel", opts.Channel)
	}

	return nil
}

// Deliberately not a ShutdownProvider.
//
// Closing the file at shutdown looks tidy and is worse twice over. Nothing is
// buffered — the rotator and the console handler each write a whole record
// straight to the file descriptor, which is what makes a record atomic
// against another process appending to the same file — so there is nothing a
// Close would flush. And providers shut down in registration order, so the
// logger registered first would close first and every remaining shutdown
// record would fall back to stderr with a warning attached. The process is
// exiting; the kernel closes the descriptor.

// environment reads the provider's variables.
//
// It is here rather than config.MustEnv for two reasons, both of which bit in
// practice. MustEnv treats a variable that is *present and empty* as a
// decision, so a stray `LOG_CHANNEL=` line in a production .env would have
// chosen the console and written no file at all — the one failure the file
// channel exists to prevent, arrived at through a blank value nobody would
// read as a choice. And MustEnv panics on a value it cannot parse, so
// `LOG_MAX_SIZE=` would take the process down before anything could log why.
//
// So: absent and empty both mean "use the default", and an unreadable value
// means the same plus a complaint, reported once the logger is up. Nothing
// about logging configuration is worth refusing to boot over except a file
// that cannot be opened, which log.New reports as an error.
//
// Reading os.Getenv directly is safe here because .env is loaded by
// api/config's init, and this package imports api/app, which imports
// api/config — so the file is in the environment long before a provider runs.
type environment struct {
	complaints []complaint
}

type complaint struct {
	variable string
	value    string
	want     string
}

func (e *environment) str(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func (e *environment) boolean(key string, fallback bool) bool {
	raw := e.str(key, "")
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		e.complaints = append(e.complaints, complaint{key, raw, "true or false"})
		return fallback
	}
	return parsed
}

func (e *environment) integer(key string, fallback int64) int64 {
	raw := e.str(key, "")
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		e.complaints = append(e.complaints, complaint{key, raw, "a whole number"})
		return fallback
	}
	return parsed
}

// report names every variable that was ignored, and what would have been
// accepted. Silence here is the thing to avoid: a typo in a logging variable
// is otherwise invisible until somebody goes looking for a log that was never
// written.
func (e *environment) report() {
	for _, c := range e.complaints {
		slog.Warn("log: ignoring an unreadable setting, using the default",
			"variable", c.variable, "value", c.value, "want", c.want)
	}
}

func (p *Provider) resolve(a app.App, env *environment) log.Options {
	opts := p.Options

	// A test boot writes nowhere, and this overrides LOG_CHANNEL rather than
	// defaulting under it.
	//
	// apptest boots the real provider list from the project root, so without
	// this every test appends to the application's own log file — shared
	// mutable state between the suite and whatever else is running, plus a
	// few megabytes of boot records that are not about any one test. A test
	// that wants to assert on logging sets Options.Channel explicitly, which
	// still wins because that branch is checked first.
	//
	// Detected from the test binary's own flags rather than from app.env: the
	// harness is handed the real configuration, so app.env is whatever .env
	// says — "development", usually — and a check against it never fires.
	if opts.Channel == "" && runningUnderTest() {
		opts.Channel = log.ChannelNull
	}

	if opts.Channel == "" {
		opts.Channel = env.str(EnvChannel, defaultChannel(a))
	}
	if opts.Level == "" {
		// APP_DEBUG as the fallback, because it is already set in every
		// scaffolded project and is what somebody reaches for before
		// discovering LOG_LEVEL.
		fallback := "info"
		if env.boolean("APP_DEBUG", false) {
			fallback = "debug"
		}
		opts.Level = env.str(EnvLevel, fallback)
	}
	if opts.Path == "" {
		opts.Path = env.str(EnvPath, log.DefaultPath)
	}
	if opts.MaxSize == 0 {
		opts.MaxSize = env.integer(EnvMaxSize, 0)
	}
	if opts.MaxFiles == 0 {
		opts.MaxFiles = int(env.integer(EnvMaxFiles, 0))
	}
	if !opts.Daily {
		opts.Daily = env.boolean(EnvDaily, true)
	}
	if !opts.Source {
		opts.Source = env.boolean(EnvSource, !inProduction(a))
	}

	// A CLI command's own output is the reason somebody ran it, and
	// info-level boot chatter buries it. Quiet raises the console to warn and
	// leaves the file alone, so a command that fails overnight still leaves a
	// record — which is the improvement on app.Run's old approach of
	// replacing the default logger with a warn-level text handler and losing
	// the file along with the chatter.
	//
	// LOG_QUIET overrides it, because the console/command split is not the
	// one that matters on a container platform. A worker or a scheduler is a
	// long-running server that happens to be spelled as a subcommand: it has
	// no operator reading its output, its whole job is to say what it did,
	// and the file quiet mode preserves instead lives in an ephemeral
	// container nobody will ever read. `LOG_QUIET=false` is how such a
	// process asks for its own voice back.
	if !opts.Quiet {
		opts.Quiet = env.boolean(EnvQuiet, a.RunningInConsole())
	}

	return opts
}

// defaultChannel is console in development and stack in production.
//
// Not file in production: a container platform collects stdio, and a
// deployment whose only log is a file inside an ephemeral filesystem has a
// log nobody will read. Both is the answer that works in both places.
func defaultChannel(a app.App) string {
	if inProduction(a) {
		return log.ChannelStack
	}
	return log.ChannelConsole
}

func inProduction(a app.App) bool { return env(a) == "production" }

// env is app.env, lowercased, or empty.
//
// Read from the configuration rather than through app.InProduction, which
// panics on an application whose app configuration is absent — and a provider
// is exactly where that is survivable.
func env(a app.App) string {
	if a == nil || a.Config() == nil {
		return ""
	}
	name, _ := a.Config().Get("app.env").(string)
	return strings.ToLower(strings.TrimSpace(name))
}

// runningUnderTest reports whether this is a `go test` binary.
//
// The test.v flag is registered by the testing package's main, so its
// presence is the signal. testing.Testing() answers the same question more
// directly, but importing testing into production code registers its whole
// flag set on the application's command line.
func runningUnderTest() bool { return flag.Lookup("test.v") != nil }

// Logger returns the application's logger, for a caller that needs the file
// path or has to close it.
//
// Ordinary logging does not go through this. Use log/slog: the default logger
// is this one.
func Logger(a app.App) (*log.Logger, bool) { return app.Lookup[*log.Logger](a) }
