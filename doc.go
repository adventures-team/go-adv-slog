// Package advslog complements the standard [log/slog] package with the pieces
// needed for seamless contextual logging in services: process-wide logger
// initialization, storing a logger in a [context.Context], zerolog-compatible
// level parsing with custom TRACE/FATAL/PANIC levels, and panic logging with
// stack traces.
//
// The standard library deliberately leaves these features out (logger-in-context
// was removed from the slog design before Go 1.21; a Fatal level and level-name
// parsing were declined), expecting them to be provided by packages like this one.
//
// # Initialization
//
// [Init] sets up the process-wide default logger once: it logs to the file
// named by the GO_LOG environment variable (stderr by default), chooses
// colored human-readable output on a terminal and JSON otherwise, redirects
// the standard log package, and respects the dynamic [Level]. [InitTest] does
// the same for tests, writing through testing.TB.Log.
//
// # Contextual logging
//
// [NewContext] stores a logger in a context, [Ctx] retrieves it (falling back
// to [slog.Default]). This is the replacement for zerolog's
// Logger.WithContext / zerolog.Ctx idiom:
//
//	ctx = advslog.NewContext(ctx, logger.With("request_id", id))
//	...
//	advslog.Ctx(ctx).Info("request processed", "status", status)
//
// With [WithContextRouting], even code unaware of this package joins in:
// records logged through the default logger's *Context methods (e.g.
// slog.InfoContext(ctx, ...) inside an external library) are routed to the
// context logger as well.
//
// # Levels
//
// [Level] is the dynamic level shared by all handlers created by this package;
// change it at runtime with Level.Set (the replacement for
// zerolog.SetGlobalLevel). [ParseLevel] parses both slog and zerolog level
// names, including the custom [LevelTrace], [LevelFatal] and [LevelPanic].
// [Fatal] and [FatalContext] log at [LevelFatal] and exit. With
// [WithLevelConfig], the default and per-package levels are driven by
// OnlineConf and change on the fly.
//
// # Panic logging
//
// [Recover] is a deferred one-liner that recovers a panic and logs its typed
// reason with a stack trace; [LogPanic], [PanicAttrs] and [TracebackAttr] are
// the underlying building blocks for custom recovery flows.
//
// # Subpackages
//
// The masking of sensitive data (URL parameters, JSON by XPath) lives in the
// logmask subpackage, and the outgoing HTTP request/response logger built on
// top of it lives in reqlog. They are separate packages so that their JSON
// processing dependencies stay out of programs that do not need them.
package advslog
