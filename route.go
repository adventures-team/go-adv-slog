package advslog

import (
	"context"
	"log/slog"
)

// routingHandler dispatches records to the logger stored in the context, when
// one is present, and to the wrapped handler otherwise.
type routingHandler struct {
	next slog.Handler
}

func (h routingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if l := fromCtx(ctx); l != nil {
		// Logger.log consults Enabled before Handle: the strip guard is
		// needed here too, or a stored logger whose handler chain contains
		// this handler would recurse.
		return l.Handler().Enabled(stripLogger(ctx), level)
	}

	return h.next.Enabled(ctx, level)
}

func (h routingHandler) Handle(ctx context.Context, r slog.Record) error {
	if l := fromCtx(ctx); l != nil {
		return l.Handler().Handle(stripLogger(ctx), r)
	}

	return h.next.Handle(ctx, r)
}

// Derivations degrade to the wrapped handler: extraction is root-only, so the
// attributes preformatted into a derived handler are never bypassed.

func (h routingHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h.next.WithAttrs(attrs) }

func (h routingHandler) WithGroup(name string) slog.Handler { return h.next.WithGroup(name) }

// stripLogger hides the context logger from nested extraction ([Ctx] and
// [fromCtx] treat a stored nil as absent).
func stripLogger(ctx context.Context) context.Context {
	return NewContext(ctx, nil)
}
