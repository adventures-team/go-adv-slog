package advslog

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"
)

// Under `go test` stderr is a pipe, not a terminal, so TracebackAttr returns
// the stack as an attr rather than printing it.

func TestTracebackAttr(t *testing.T) {
	a := TracebackAttr()

	if a.Key != "traceback" {
		t.Fatalf("key = %q, expected traceback", a.Key)
	}
	if !strings.Contains(a.Value.String(), "goroutine") {
		t.Errorf("traceback does not look like a stack trace: %.100s", a.Value.String())
	}
}

func TestPanicAttrs(t *testing.T) {
	tests := []struct {
		name     string
		reason   any
		expected string // expected panic_reason value as rendered by Value.String()
	}{
		{"string", "boom", "boom"},
		{"error", errors.New("some error"), "some error"},
		{"stringer", net.IPv4(127, 0, 0, 1), "127.0.0.1"},
		{"other", 42, "42"},
	}

	for _, tt := range tests {
		attrs := PanicAttrs(tt.reason)

		if len(attrs) != 2 {
			t.Fatalf("%s: got %d attrs, expected 2", tt.name, len(attrs))
		}
		if attrs[0].Key != "panic_reason" {
			t.Errorf("%s: first attr key = %q, expected panic_reason", tt.name, attrs[0].Key)
		}
		if got := attrs[0].Value.Resolve().String(); got != tt.expected {
			t.Errorf("%s: panic_reason = %q, expected %q", tt.name, got, tt.expected)
		}
		if attrs[1].Key != "traceback" {
			t.Errorf("%s: second attr key = %q, expected traceback", tt.name, attrs[1].Key)
		}
	}
}

func TestLogPanic(t *testing.T) {
	var buf bytes.Buffer
	ctx := NewContext(context.Background(), slog.New(jsonHandler(&buf, false, Level)))

	LogPanic(ctx, "worker panicked", errors.New("nil dereference"))

	out := buf.String()
	for _, want := range []string{`"level":"ERROR"`, `"msg":"worker panicked"`, `"panic_reason":"nil dereference"`, `"traceback":"goroutine`} {
		if !strings.Contains(out, want) {
			t.Errorf("%s not found in %s", want, out)
		}
	}
}

func TestRecover(t *testing.T) {
	var buf bytes.Buffer
	ctx := NewContext(context.Background(), slog.New(jsonHandler(&buf, false, Level)))

	func() {
		defer Recover(ctx, "caught")
		panic("boom")
	}()

	out := buf.String()
	for _, want := range []string{`"msg":"caught"`, `"panic_reason":"boom"`, "TestRecover"} {
		if !strings.Contains(out, want) {
			t.Errorf("%s not found in %s", want, out)
		}
	}
}

func TestRecoverNoPanic(t *testing.T) {
	var buf bytes.Buffer
	ctx := NewContext(context.Background(), slog.New(jsonHandler(&buf, false, Level)))

	func() {
		defer Recover(ctx, "should not log")
	}()

	if buf.Len() != 0 {
		t.Errorf("Recover logged without a panic: %s", buf.String())
	}
}
