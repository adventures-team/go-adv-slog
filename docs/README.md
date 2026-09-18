[![Go Reference](https://pkg.go.dev/badge/github.com/adventures-team/go-adv-slog.svg)](https://pkg.go.dev/github.com/adventures-team/go-adv-slog)

# go-adv-slog

Contextual logging helpers for the standard [log/slog](https://pkg.go.dev/log/slog)
package: process initialization, logger-in-context, zerolog-compatible levels,
panic logging with stack traces, and sensitive-data masking.

The standard library deliberately leaves these pieces out (logger-in-context was
removed from the slog design; a Fatal level and level-name parsing were declined);
`advslog` fills the gaps without hiding slog behind a facade — everything stays a
plain `*slog.Logger`.

```
go get github.com/adventures-team/go-adv-slog
```

## Features

- **`Init`** — one-call setup of the default logger: logs to the file named by
  `GO_LOG` (stderr by default); colored human-readable output on a terminal
  (honors `NO_COLOR`), native slog JSON otherwise; redirects the standard `log`
  package.
- **`InitTest`** — the same for tests, routed through `testing.TB.Output` with
  second-precision timestamps. Every test running code that logs must call it:
  `go test` does not support log output written directly to stdout or stderr
  (see the `InitTest` documentation).
- **`NewContext` / `Ctx`** — store a request-scoped logger in a
  `context.Context` and retrieve it anywhere (falls back to `slog.Default()`;
  nil context tolerated). The replacement for zerolog's
  `Logger.WithContext` / `zerolog.Ctx`.
- **`WithContextRouting`** — opt-in: records logged with a context through the
  default logger (`slog.InfoContext(ctx, ...)`) — e.g. by external libraries
  that know nothing about this package — are routed to the context logger and
  pick up its attributes.
- **`Level`** — dynamic level shared by all handlers the package creates
  (`advslog.Level.Set(...)` replaces `zerolog.SetGlobalLevel`), plus
  **`ParseLevel`** understanding both slog and zerolog names, and the custom
  `LevelTrace`, `LevelFatal`, `LevelPanic` rendered as `TRACE`/`FATAL`/`PANIC`.
- **`WithLevelConfig`** — per-package log levels driven by
  [OnlineConf](https://github.com/onlineconf/onlineconf) (`/level` default,
  `/<import-path>/level` overrides), applied on the fly through onlineconf-go
  subscriptions: boost one package to `trace` or silence another to `error`
  without redeploys.
- **`Err`**, **`Fatal`**, **`FatalContext`** — the missing conveniences:
  an error attr under the conventional `"error"` key; log-and-exit.
- **`Recover` / `LogPanic` / `PanicAttrs` / `TracebackAttr`** — panic logging
  with a typed reason and a stack trace (printed raw to stderr instead when it
  is a terminal, for local debugging).
- **[`logmask`](../logmask/)** — masking of sensitive values: strings,
  URL parameters, and JSON fields addressed by XPath.
- **[`reqlog`](../reqlog/)** — outgoing HTTP request/response logging with
  duration, status and masking, built on `logmask` and the context logger.

## Quick start

```go
func main() {
	advslog.Init()

	slog.Info("service started", "version", version)

	// hand every request a logger enriched with its own fields
	ctx := advslog.NewContext(r.Context(), slog.Default().With("request_id", id))
	handle(ctx)
}

func handle(ctx context.Context) {
	defer advslog.Recover(ctx, "panic in handler")

	advslog.Ctx(ctx).Info("request processed", "status", 200)
	// {"time":"...","level":"INFO","msg":"request processed","request_id":"...","status":200}
}
```

In tests:

```go
func TestHandle(t *testing.T) {
	logger := advslog.InitTest(t) // colored output through t.Output, shown for failures and with -v
	logger.Info("fixture ready", "user", "alice")

	// the code under test logs through the context, as in main above
	handle(advslog.NewContext(t.Context(), logger.With("request_id", "test-1")))
}
```

Masked outgoing request logging:

```go
var apiLog = reqlog.NewOutgoingRequestLogger("Billing API", []string{"access_token"}, nil)

req := apiLog.LogRequest(ctx, http.MethodPost, url, params) // params masked
status, body, err := call(url, params)
req.LogResponse(status, body, err) // response masked, duration and status attached
```

## Configuration

| Knob | Effect |
|---|---|
| `GO_LOG` (env) | log destination file for `Init`; empty = stderr |
| `NO_COLOR` (env) | disables colored terminal output ([no-color.org](https://no-color.org)) |
| `advslog.Level` | dynamic level, `slog.LevelVar`; defaults to `LevelTrace` (everything is logged, as in zerolog) |
| `advslog.TestTimeFormat` | time format for terminal output; default `15:04:05` |
| `advslog.Init(advslog.WithAddSource())` | record source positions of log calls |
| `advslog.Init(advslog.WithLevel(l))` | initial `Level` value |
| `advslog.Init(advslog.WithContextRouting())` | route `slog.*Context` calls made through the default logger to the context logger |
| `advslog.Init(advslog.WithLevelConfig(oc))` | per-package levels from an onlineconf-go `Module`/`Subtree`, live updates |
