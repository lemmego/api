# lemmego/api

The core of [Lemmego](https://lemmego.org), a full-stack web framework for Go:
routing, a request context, middleware, sessions, validation, configuration, a
service container and the provider system every other module arrives through.

> ⚠️ Pre-1.0 and under active development. Minor versions may change public
> APIs.

You rarely add this by hand — `lemmego new` generates a project that already
depends on it. Full documentation is at
[lemmego.org/docs](https://lemmego.org/docs).

## A handler

```go
import "github.com/lemmego/api/app"

func Show(c app.Context) error {
    id := c.Param("id")

    post, err := repos.Post(c.App()).FindByID(c.RequestContext(), id)
    if err != nil {
        return c.NotFound(err)
    }

    return c.JSON(app.M{"post": post})
}
```

A handler is `func(c app.Context) error`. One value carries everything about
the request and every way of replying, and returning an error is how you hand
a failure to the framework's error mapping rather than writing a response for
it.

You answer **once**. Pick the reply that fits and return it.

## Routing

```go
func WebRoutes(a app.App) {
    r := a.Router()

    r.Get("/", handlers.Home)
    r.Post("/posts", handlers.Store)

    admin := r.Group("/admin")
    admin.UseBefore(auth.Protected, middleware.AdminOnly)
    admin.Get("/users", handlers.Users)
}
```

A group is a path prefix and a list of middleware, stamped onto each route as
you register it — so `UseBefore` on thirty admin routes is one line instead of
thirty, and once your routes are registered there is no group left at all.

`a.Router().Routes()` reports the whole table, which is what
`lemmego/mcp`'s `list-routes` reads.

## The service container

```go
a.AddService(mailer)                  // in a provider
mailer := app.Get[*mail.Mailer](a)    // anywhere with the app
```

Keyed by type, so there is nothing to name and nothing to spell wrong.

**`app.Get` panics when the service is absent.** That is right for a
dependency your application cannot run without, and wrong for one that is
optional — if a page reads a service that a deployment chose not to register,
the panic takes down every render. Check for it instead of assuming it.

## Providers

A provider is a unit of setup. It is handed the application at boot and
registers what it contributes.

```go
type MyProvider struct{}

func (p *MyProvider) Provide(a app.App) error {
    a.AddService(NewThing(a.Config()))
    return nil
}
```

```go
// bootstrap/providers.go
func LoadProviders() []app.Provider {
    return []app.Provider{
        &logger.Provider{},
        &ormconnector.Provider{},
        &session.Provider{},
        &fs.Provider{},
    }
}
```

**The order is load-bearing.** A provider that borrows the application's
database connection has to come after the connector that registered it, and a
provider that finds nothing registered does not usually fail loudly — it
derives its own, and then you have two connections and one of them ignoring
your configuration. The session provider's `database` driver takes its dialect
from your registered connection, so it has to come after the connector too.

Implement more interfaces to contribute more:

| Interface | Contributes |
|---|---|
| `Provider` | `Provide(a App) error` — the one everything implements |
| `RouteProvider` | `AddRoutes() RouteCallback` |
| `MiddlewareProvider` | `AddMiddlewares() []Handler` |
| `CommandProvider` | `AddCommands() []Command` |
| `PublishableProvider` | `AddPublishables() []*Publishable` — files copied into the project, such as a migration |
| `ShutdownProvider` | `Shutdown(ctx) error` |
| `ErrMapProvider` | `AddErrMap() ErrMap` — how an error becomes a response |

A module ships its migration as a publishable rather than creating tables at
boot, so the schema is yours to read, edit, roll back and find in
`schema_migrations`.

## Middleware

```go
func AdminOnly(c app.Context) error {
    user, ok := auth.UserAs[*models.User](c)
    if !ok || !user.IsStaff {
        return c.Forbidden(errors.New("staff only"))
    }
    return c.Next()
}
```

Middleware is an ordinary `app.Handler`. Call `c.Next()` to continue, or
return a response to stop the request there.

`UseBefore` and `UseAfter` on the router or a group apply it to a set of
routes; passing it to the route itself applies it to one.

**`UseBefore` prepends**, so middleware registered that way runs ahead of the
middleware listed on an individual route. If a shell or layout middleware
reads the current user, it will see a signed-out request on a signed-in one
unless the authentication middleware ran first — resolve the user lazily in
that case.

## The packages

| Package | What it holds |
|---|---|
| `app` | the application, the router, `Context`, the container, health checks, events |
| `config` | environment-based configuration; `MustEnv` and the `config.M` map |
| `req` | binding and validating input — JSON, URL-encoded, multipart |
| `res` | rendering templates |
| `db` | connection management, plus the `Unique` and `Exists` validation rules |
| `session` | sessions over scs, with `memory`, `file`, `redis` and `database` drivers |
| `middleware` | request logging, recovery, CSRF, tracing |
| `encryption` | symmetric encryption over your `APP_KEY` |
| `logger` | the provider that owns `slog.SetDefault`, over [`lemmego/log`](https://github.com/lemmego/log) |
| `fs`, `providers/fs` | storage disks over [`lemmego/fsys`](https://github.com/lemmego/fsys) |
| `di`, `container` | service resolution |
| `event` | the untyped in-process event emitter |
| `utils` | small helpers, including `Bcrypt` |

### Two notes on `config`

`config.MustEnv` treats a variable that is **present and empty** as a
decision, and **panics** on a value it cannot parse. So a stray `FOO=` line in
a production `.env` chooses the empty value rather than your default, and a
typo in a number takes the process down before anything can log why. Read the
variable directly where "absent" and "empty" should mean the same thing.

### One note on events

`app.Dispatch`/`app.On` is **untyped and synchronous on the calling
goroutine**, and a listener's error is logged rather than returned. It is
right for observability — a counter, a log line — and wrong for slow work.
Enqueue a job instead.

## Health checks

```go
a.GetHealthChecker().RegisterCheck("database", app.HealthCheck{
    Name:      "database",
    CheckFunc: checkDB,
    Critical:  true,
})
```

`api/app/health.go` supplies the checker and
`HealthCheckHandler`, `LivenessHandler` and `ReadinessHandler` — but it
registers **no routes**. Your application mounts them, which a scaffolded
project does at `/health`, `/livez` and `/readyz`.

`RegisterDefaultChecks` covers the filesystem and the session store only, so a
database check is yours to add. Register a check as critical only when your
site is useless without it: a readiness probe that fails on something
non-essential pulls every instance out of rotation for a site that still
serves pages.

## Testing

```bash
go test ./...
GOWORK=off go test ./...   # as a published module
```

For testing an application built on this, use
[`lemmego/apptest`](https://github.com/lemmego/apptest), which boots the real
application against a scratch Postgres schema.

## Contributing

Pull requests are welcome. For major changes, please open an issue first to
discuss what you would like to change, and please update tests as
appropriate.

## License

[MIT](https://choosealicense.com/licenses/mit/)
