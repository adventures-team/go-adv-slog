package advslog

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"golang.org/x/term"
)

const tracebackMaxSize = 65536

// TracebackAttr returns the current goroutine's stack trace as a "traceback"
// attr.
//
// If stderr is a terminal, the stack is printed there instead and an empty
// Attr (omitted by handlers) is returned: during local debugging the logging
// context is not needed — there is a single user and the failing request is
// known — and a raw stack is easier to read.
func TracebackAttr() slog.Attr {
	buf := make([]byte, tracebackMaxSize)
	n := runtime.Stack(buf, false)

	if term.IsTerminal(int(os.Stderr.Fd())) {
		_, _ = os.Stderr.Write(buf[:n])

		return slog.Attr{}
	}

	return slog.String("traceback", string(buf[:n]))
}

// PanicAttrs converts a recovered panic value into attrs: a typed
// "panic_reason" and the stack trace from [TracebackAttr].
//
// A reason implementing [fmt.Stringer], or being a string or an error, is
// logged as a string or an error respectively (some built-in panics arrive as
// errors); anything else is logged as-is, marshaled through reflection.
func PanicAttrs(reason any) []slog.Attr {
	var reasonAttr slog.Attr

	switch r := reason.(type) {
	case fmt.Stringer:
		reasonAttr = slog.String("panic_reason", r.String())

	case string:
		reasonAttr = slog.String("panic_reason", r)

	case error:
		reasonAttr = slog.Any("panic_reason", r)

	default:
		reasonAttr = slog.Any("panic_reason", reason)
	}

	return []slog.Attr{reasonAttr, TracebackAttr()}
}

// LogPanic logs a recovered panic value with its stack trace at
// [slog.LevelError] through the logger from ctx (see [Ctx]).
func LogPanic(ctx context.Context, msg string, reason any) {
	logAttrsPC(ctx, Ctx(ctx), callerPC(), slog.LevelError, msg, PanicAttrs(reason)...)
}

// Recover recovers a panic in flight, logs it like [LogPanic] and suppresses
// its propagation. It must be used directly in a defer statement:
//
//	defer advslog.Recover(ctx, "panic in the config watcher")
//
// When there is no panic, Recover does nothing.
//
// Because the panic is swallowed, Recover fits only the simple
// handle-log-forget scenarios: a background worker iteration, a watcher
// callback — places where execution just moves on. When the handling is
// anything more than logging (re-panicking, converting the panic into a
// returned error, failing the affected request, updating a metric), recover
// the panic manually in your own deferred function and report it with
// [LogPanic]:
//
//	defer func() {
//		if reason := recover(); reason != nil {
//			advslog.LogPanic(ctx, "panic in the handler", reason)
//			err = fmt.Errorf("handler panicked: %v", reason)
//		}
//	}()
func Recover(ctx context.Context, msg string) {
	if reason := recover(); reason != nil { //nolint:revive // Recover is itself the deferred function; recover() works when called directly by it
		logAttrsPC(ctx, Ctx(ctx), callerPC(), slog.LevelError, msg, PanicAttrs(reason)...)
	}
}
