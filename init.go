package advslog

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
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
	addSource  bool
	ctxRouting bool
	level      *slog.Level
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

// TestingLog is the subset of [testing.TB] needed by [InitTest]. It is a
// separate interface so that importing advslog does not pull in the testing
// package.
type TestingLog interface {
	Log(args ...any)
}

// InitTest sets up the default slog logger for use from tests and returns it:
// colored human-readable output (honoring NO_COLOR) with second-precision
// time ([TestTimeFormat]), written through tb.Log so that it interleaves
// correctly with the test output and is shown only for failing tests (unless
// -v is given).
//
// Unlike [Init] it is not guarded by a once: call it from any test, even when
// some library has already called Init. It accepts the same options as Init.
func InitTest(tb TestingLog, opts ...Option) *slog.Logger {
	l := slog.New(buildHandler(testWriter{tb}, true, applyOptions(opts)))
	slog.SetDefault(l)

	return l
}

type testWriter struct{ tb TestingLog }

func (w testWriter) Write(p []byte) (int, error) {
	w.tb.Log(strings.TrimSuffix(string(p), "\n"))

	return len(p), nil
}

// buildHandler assembles the handler chain shared by Init and InitTest:
// console or JSON output, optionally wrapped for context routing.
func buildHandler(w io.Writer, isTerminal bool, o options) slog.Handler {
	var h slog.Handler
	if isTerminal {
		h = consoleHandler(w, o.addSource)
	} else {
		h = jsonHandler(w, o.addSource)
	}

	if o.ctxRouting {
		h = routingHandler{next: h}
	}

	return h
}

func jsonHandler(w io.Writer, addSource bool) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{
		AddSource:   addSource,
		Level:       Level,
		ReplaceAttr: replaceLevelName,
	})
}

func consoleHandler(w io.Writer, addSource bool) slog.Handler {
	return tint.NewTextHandler(w, &tint.Options{
		AddSource:   addSource,
		Level:       Level,
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
