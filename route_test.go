package advslog

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

// setupRoutedDefault installs a routing default logger writing JSON to the
// returned buffer, restoring the previous default when the test ends.
func setupRoutedDefault(t *testing.T) *bytes.Buffer {
	t.Helper()

	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(routingHandler{next: jsonHandler(&buf, false)}))

	return &buf
}

// requestCtx returns a context carrying a request-scoped logger derived from
// the (routing) default logger, as ecosystem code creates it.
func requestCtx(t *testing.T) context.Context {
	t.Helper()

	return NewContext(context.Background(), slog.Default().With("request_id", "req-42"))
}

// External code that never heard of this package logs through the default
// logger with a context: the record must be routed to the context logger.
func TestRoutingExternalCode(t *testing.T) {
	buf := setupRoutedDefault(t)
	ctx := requestCtx(t)

	slog.InfoContext(ctx, "from external lib", "detail", 1)

	out := buf.String()
	for _, want := range []string{`"msg":"from external lib"`, `"request_id":"req-42"`, `"detail":1`} {
		if !strings.Contains(out, want) {
			t.Errorf("%s not found in %s", want, out)
		}
	}
}

// Without a context there is nothing to extract: the record goes through the
// wrapped handler unchanged.
func TestRoutingNoContext(t *testing.T) {
	buf := setupRoutedDefault(t)

	slog.Info("no ctx available")

	out := buf.String()
	if !strings.Contains(out, `"msg":"no ctx available"`) {
		t.Errorf("record not logged: %s", out)
	}
	if strings.Contains(out, "request_id") {
		t.Errorf("unexpected request_id without a context: %s", out)
	}
}

// The ecosystem path — logging directly through the logger from the context —
// is unaffected by routing: attributes appear exactly once.
func TestRoutingDirectCtxLogger(t *testing.T) {
	buf := setupRoutedDefault(t)
	ctx := requestCtx(t)

	Ctx(ctx).Info("direct ctx logger")

	out := buf.String()
	if got := strings.Count(out, "request_id"); got != 1 {
		t.Errorf("request_id appears %d times, expected exactly 1: %s", got, out)
	}
}

// Extraction is root-only: a derived logger keeps the attributes preformatted
// into its own handler and is not routed, so they are never dropped.
func TestRoutingDerivedLoggerNotRouted(t *testing.T) {
	buf := setupRoutedDefault(t)
	ctx := requestCtx(t)

	libLogger := slog.Default().With("lib", "sdk")
	libLogger.InfoContext(ctx, "derived lib logger")

	out := buf.String()
	if !strings.Contains(out, `"lib":"sdk"`) {
		t.Errorf("derived logger attribute lost: %s", out)
	}
	if strings.Contains(out, "request_id") {
		t.Errorf("derived logger was routed (request_id present): %s", out)
	}
}

// Storing the bare default logger — whose handler chain contains the routing
// handler itself — must not recurse (the strip guard, in both Enabled and
// Handle). A regression here overflows the stack and kills the test binary.
func TestRoutingLoopSafety(t *testing.T) {
	buf := setupRoutedDefault(t)
	loopCtx := NewContext(context.Background(), slog.Default())

	slog.InfoContext(loopCtx, "no infinite loop")

	if !strings.Contains(buf.String(), `"msg":"no infinite loop"`) {
		t.Errorf("record not logged: %s", buf.String())
	}
}

// Enabled consults the context logger's handler, so its level gating applies
// to routed records.
func TestRoutingEnabledGating(t *testing.T) {
	buf := setupRoutedDefault(t)

	var quietBuf bytes.Buffer
	quiet := slog.New(slog.NewJSONHandler(&quietBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := NewContext(context.Background(), quiet)

	slog.InfoContext(ctx, "suppressed")
	slog.WarnContext(ctx, "warn passes")

	if quietBuf.String() != "" && strings.Contains(quietBuf.String(), "suppressed") {
		t.Errorf("info record not gated by the ctx logger level: %s", quietBuf.String())
	}
	if !strings.Contains(quietBuf.String(), `"msg":"warn passes"`) {
		t.Errorf("warn record missing: %s", quietBuf.String())
	}
	if buf.Len() != 0 {
		t.Errorf("routed records leaked into the default sink: %s", buf.String())
	}
}

// The option must plumb through Init's handler construction and InitTest.
func TestWithContextRoutingPlumbing(t *testing.T) {
	var buf bytes.Buffer

	h := buildHandler(&buf, false, options{ctxRouting: true})
	if _, ok := h.(routingHandler); !ok {
		t.Errorf("buildHandler did not wrap with routingHandler: %T", h)
	}

	h = buildHandler(&buf, false, options{})
	if _, ok := h.(routingHandler); ok {
		t.Error("buildHandler wrapped with routingHandler without the option")
	}
}

func TestInitTestWithContextRouting(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	prev := slog.Default()
	defer slog.SetDefault(prev)

	rec := &tbRecorder{}
	InitTest(rec, WithContextRouting())

	ctx := NewContext(context.Background(), slog.Default().With("request_id", "req-1"))
	slog.InfoContext(ctx, "routed in tests")

	if len(rec.lines) != 1 {
		t.Fatalf("got %d lines, expected 1: %q", len(rec.lines), rec.lines)
	}
	for _, want := range []string{"routed in tests", "request_id=req-1"} {
		if !strings.Contains(rec.lines[0], want) {
			t.Errorf("%s not found in %q", want, rec.lines[0])
		}
	}
}
