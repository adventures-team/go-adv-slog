package advslog

import (
	"context"
	"log/slog"
	"os"
)

// Err returns an attr holding err under the "error" key, following the
// zerolog convention. A nil err yields an empty Attr, which the built-in
// handlers omit.
func Err(err error) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}

	return slog.Any("error", err)
}

// osExit is swapped out in tests.
var osExit = os.Exit

// Fatal logs msg at [LevelFatal] to the default logger and terminates the
// process with exit status 1. It replaces zerolog's log.Fatal().
func Fatal(msg string, args ...any) {
	logPC(context.Background(), slog.Default(), callerPC(), LevelFatal, msg, args...)
	osExit(1)
}

// FatalContext is [Fatal] logging through the logger from ctx (see [Ctx]).
func FatalContext(ctx context.Context, msg string, args ...any) {
	logPC(ctx, Ctx(ctx), callerPC(), LevelFatal, msg, args...)
	osExit(1)
}
