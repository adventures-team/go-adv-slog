package advslog

import (
	"context"
	"log/slog"
	"runtime"
	"time"
)

// callerPC returns the program counter of the caller of the exported function
// that called it, for source attribution (see [slog.Record]). It must be
// called directly from the exported function.
func callerPC() uintptr {
	var pcs [1]uintptr
	// skip runtime.Callers, callerPC and the exported function itself
	runtime.Callers(3, pcs[:])

	return pcs[0]
}

// logPC emits a record like [slog.Logger.Log], but attributed to the given
// caller pc instead of the direct caller.
func logPC(ctx context.Context, l *slog.Logger, pc uintptr, level slog.Level, msg string, args ...any) {
	if !l.Enabled(ctx, level) {
		return
	}

	r := slog.NewRecord(time.Now(), level, msg, pc)
	r.Add(args...)
	_ = l.Handler().Handle(ctx, r)
}

// logAttrsPC is [logPC] for []slog.Attr.
func logAttrsPC(ctx context.Context, l *slog.Logger, pc uintptr, level slog.Level, msg string, attrs ...slog.Attr) {
	if !l.Enabled(ctx, level) {
		return
	}

	r := slog.NewRecord(time.Now(), level, msg, pc)
	r.AddAttrs(attrs...)
	_ = l.Handler().Handle(ctx, r)
}
