package advslog

import (
	"context"
	"log/slog"
	"testing"
)

func TestCtx(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	ctx := NewContext(context.Background(), logger)
	if got := Ctx(ctx); got != logger {
		t.Error("Ctx did not return the logger stored by NewContext")
	}

	if got := Ctx(context.Background()); got != slog.Default() {
		t.Error("Ctx on a context without a logger did not fall back to slog.Default")
	}

	if got := Ctx(nil); got != slog.Default() { //nolint:staticcheck // nil ctx tolerance is part of the contract
		t.Error("Ctx(nil) did not fall back to slog.Default")
	}

	if got := Ctx(NewContext(context.Background(), nil)); got != slog.Default() {
		t.Error("Ctx did not treat a stored nil logger as absent")
	}
}

func TestCtxDerivedLogger(t *testing.T) {
	base := slog.New(slog.DiscardHandler)
	derived := base.With("request_id", "req-1")

	ctx := NewContext(context.Background(), derived)
	if got := Ctx(ctx); got != derived {
		t.Error("Ctx did not return the derived logger")
	}
}
