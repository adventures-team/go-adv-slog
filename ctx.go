package advslog

import (
	"context"
	"log/slog"
)

type ctxKey struct{}

// NewContext returns a copy of ctx with l stored in it, to be retrieved with
// [Ctx]. It replaces zerolog's Logger.WithContext.
func NewContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// Ctx returns the logger stored in ctx by [NewContext], or [slog.Default] if
// there is none. It replaces zerolog.Ctx with zerolog.DefaultContextLogger
// set.
//
// A nil logger stored in the context is treated as absent. ctx may be nil:
// functions accepting a context exclusively for logging should document that
// callers may pass nil.
func Ctx(ctx context.Context) *slog.Logger {
	if l := fromCtx(ctx); l != nil {
		return l
	}

	return slog.Default()
}

// fromCtx returns the logger stored in ctx, or nil when there is none (or ctx
// is nil).
func fromCtx(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return nil
	}

	l, _ := ctx.Value(ctxKey{}).(*slog.Logger)

	return l
}
