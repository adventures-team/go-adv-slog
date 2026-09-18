package advslog

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/lmittmann/tint"
	"golang.org/x/term"
)

// EnvGoLog names the environment variable specifying the log destination file
// for [Init]. Empty or unset means stderr.
const EnvGoLog = "GO_LOG"

// TestTimeFormat is the time format for terminal output ([Init] on a TTY and
// [InitTest]). It may be changed before the Init* call.
//
// JSON output always uses RFC 3339 with nanoseconds, the [slog.JSONHandler]
// default.
var TestTimeFormat = "15:04:05"

// Option configures [Init] and [InitTest].
type Option func(*options)

type options struct {
	addSource   bool
	ctxRouting  bool
	level       *slog.Level
	levelConfig LevelConfig
}

// applyOptions gathers the options and applies the [Level] change, shared by
// Init and InitTest.
func applyOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	if o.level != nil {
		Level.Set(*o.level)
	}

	return o
}

// WithAddSource makes the handler record the source position of the log call
// (see [slog.HandlerOptions]).
func WithAddSource() Option {
	return func(o *options) { o.addSource = true }
}

// WithLevel sets the initial value of [Level].
func WithLevel(l slog.Level) Option {
	return func(o *options) { o.level = &l }
}

// WithLevelConfig enables per-package log levels driven by OnlineConf through
// cfg — a *onlineconf.Module or, recommended, a *onlineconf.Subtree scoped to
// the service's logging section (a Module subscribes at the root, which
// OnlineConf notifies on every database update).
//
// Configuration layout, relative to cfg:
//
//	/level                       default level (drives [Level]; absent = keep current)
//	/<import-path>/level         override for one package, e.g.
//	                             /github.com/adventures-team/go-adv-slog/reqlog/level
//
// Values are level names accepted by [ParseLevel]. Changes apply on the fly
// via onlineconf-go subscriptions; the OnlineConf child_lists feature must be
// enabled for the module (subscriptions themselves already require it).
//
// A record belongs to the package whose code called the slog API, resolved
// from the record's PC — helpers logging on a caller's behalf own their
// records unless they stamp the caller's PC (as advslog's Fatal, LogPanic and
// Recover do). Handler Enabled reports at floor granularity —
// min(default, lowest override) — because it receives no caller information;
// the per-package decision happens when the record is handled. For expensive
// log arguments prefer [slog.LogValuer] values: records dropped by the
// per-package gate never resolve them.
func WithLevelConfig(cfg LevelConfig) Option {
	return func(o *options) { o.levelConfig = cfg }
}

// WithContextRouting makes [Init] (and [InitTest]) wrap the handler so that
// records logged through the default logger's context-aware methods
// ([slog.InfoContext], [slog.Logger.Log], ...) are routed to the logger
// stored in the context by [NewContext], when there is one.
//
// This brings contextual logging to code that knows nothing about this
// package: an external library calling slog.InfoContext(ctx, ...) emits its
// record through the request's logger with all of the logger's accumulated
// attributes. In code you control, [Ctx] remains the preferred way to log
// contextually.
//
// Routing intentionally applies only at the root handler:
//
//   - Derived loggers (slog.Default().With(...)) log through their own plain
//     handler and are not routed. Re-routing them would silently drop the
//     attributes accumulated in the derived handler's state.
//   - Only the *Context call forms can be routed: slog.Info(...) carries no
//     context to extract a logger from.
//
// The record is forwarded to the context logger's handler as-is, preserving
// the source attribution, and Enabled consults that handler too, so the
// context logger's level gating applies. Routing is loop-safe: the extracted
// logger is not re-extracted even when its own handler chain contains the
// routing handler (e.g. when the bare default logger is stored in the
// context).
func WithContextRouting() Option {
	return func(o *options) { o.ctxRouting = true }
}

var (
	initOnce      sync.Once
	defaultLogger *slog.Logger
)

// Init sets up the process-wide default logger and returns it.
//
// Logs go to the file named by the GO_LOG environment variable ([EnvGoLog]),
// stderr by default. If the destination is a terminal, output is colored and
// human-readable (honoring NO_COLOR, see https://no-color.org); otherwise it
// is JSON in the native log/slog format. Either way the level is controlled
// by the dynamic [Level] and the custom levels are rendered by name.
//
// The logger is installed with [slog.SetDefault], which also redirects the
// standard log package into it, and it is what [Ctx] falls back to for
// contexts without a logger.
//
// Init runs once: subsequent calls return the logger created by the first
// one.
func Init(opts ...Option) *slog.Logger {
	initOnce.Do(func() {
		o := applyOptions(opts)
		f := logFile()

		defaultLogger = slog.New(buildHandler(f, term.IsTerminal(int(f.Fd())), o))
		slog.SetDefault(defaultLogger)
	})

	return defaultLogger
}

// TestingOutput is the subset of [testing.TB] needed by [InitTest]: the Output
// method of *testing.T, *testing.B and *testing.F. It is a separate interface
// so that importing advslog does not pull in the testing package.
type TestingOutput interface {
	Output() io.Writer
}

// InitTest sets up the default slog logger for use from tests and returns it:
// colored human-readable output (honoring NO_COLOR) with second-precision
// time ([TestTimeFormat]), written through tb.Output so that it interleaves
// correctly with the test output and is shown only for failing tests (unless
// -v is given).
//
// Every test running code that logs must call InitTest first, whatever
// logging library that code uses, so that no log line reaches stdout or
// stderr directly. This is the position of the Go maintainers, not a matter
// of preference: test output belongs in testing.TB.Log or testing.TB.Output.
// Output written past them is not attributed to its test, is printed for
// passing tests too, and since Go 1.27 is mangled by go test -json, which
// drops or misreads the control characters of its own framing in it (ESC
// among them, so any colored output). A report of that was closed as working
// as intended with exactly this advice: https://go.dev/issue/81592. Code that
// logs through a logger of its own rather than the default one must be given
// the returned logger, e.g. through [NewContext].
//
// Unlike [Init] it is not guarded by a once: call it from any test, even when
// some library has already called Init. It accepts the same options as Init.
func InitTest(tb TestingOutput, opts ...Option) *slog.Logger {
	l := slog.New(buildHandler(tb.Output(), true, applyOptions(opts)))
	slog.SetDefault(l)

	return l
}

// buildHandler assembles the handler chain shared by Init and InitTest:
// console or JSON output, optionally wrapped for per-package levels and
// context routing (routing outermost).
func buildHandler(w io.Writer, isTerminal bool, o options) slog.Handler {
	// with level config active, levelHandler owns all gating and the sink
	// must not re-filter (an override may be more verbose than the default)
	sinkLevel := slog.Leveler(Level)
	if o.levelConfig != nil {
		sinkLevel = levelAll
	}

	var h slog.Handler
	if isTerminal {
		h = consoleHandler(w, o.addSource, sinkLevel)
	} else {
		h = jsonHandler(w, o.addSource, sinkLevel)
	}

	if o.levelConfig != nil {
		h = levelHandler{next: h, state: newLevelState(o.levelConfig)}
	}

	if o.ctxRouting {
		h = routingHandler{next: h}
	}

	return h
}

func jsonHandler(w io.Writer, addSource bool, level slog.Leveler) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{
		AddSource:   addSource,
		Level:       level,
		ReplaceAttr: replaceLevelName,
	})
}

func consoleHandler(w io.Writer, addSource bool, level slog.Leveler) slog.Handler {
	return tint.NewTextHandler(w, &tint.Options{
		AddSource:   addSource,
		Level:       level,
		TimeFormat:  TestTimeFormat,
		NoColor:     noColor(),
		ReplaceAttr: replaceLevelName,
	})
}

// noColor implements https://no-color.org: color is disabled when the
// NO_COLOR environment variable is present with a non-empty value.
func noColor() bool {
	return os.Getenv("NO_COLOR") != ""
}

func logFile() *os.File {
	name := os.Getenv(EnvGoLog)
	if name == "" {
		return os.Stderr
	}

	f, err := os.OpenFile(name, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644) //nolint:gosec // the log file location comes from the environment by design
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		os.Exit(1) //nolint:revive // an unusable log destination is fatal at startup by design
	}

	return f
}
