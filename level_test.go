package advslog

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in       string
		expected slog.Level
	}{
		// standard slog names, case-insensitive, with offsets
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"Warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR+4", LevelFatal},
		// zerolog extras
		{"trace", LevelTrace},
		{"TRACE", LevelTrace},
		{"warning", slog.LevelWarn},
		{"fatal", LevelFatal},
		{"Panic", LevelPanic},
	}

	for _, tt := range tests {
		got, err := ParseLevel(tt.in)
		if err != nil {
			t.Errorf("ParseLevel(%q): unexpected error: %v", tt.in, err)
		} else if got != tt.expected {
			t.Errorf("ParseLevel(%q) = %v, expected %v", tt.in, got, tt.expected)
		}
	}

	if _, err := ParseLevel("verbose"); err == nil {
		t.Error("ParseLevel(verbose): expected an error")
	}
}

func TestLevelNamesInJSONOutput(t *testing.T) {
	old := Level.Level()
	Level.Set(LevelTrace)
	defer Level.Set(old)

	var buf bytes.Buffer
	logger := slog.New(jsonHandler(&buf, false))
	ctx := context.Background()

	logger.Log(ctx, LevelTrace, "trace msg")
	logger.Log(ctx, slog.LevelInfo, "info msg")
	logger.Log(ctx, LevelFatal, "fatal msg")
	logger.Log(ctx, LevelPanic, "panic msg")
	logger.Log(ctx, slog.LevelWarn+1, "custom offset msg")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	expected := []string{
		`"level":"TRACE"`,
		`"level":"INFO"`,
		`"level":"FATAL"`,
		`"level":"PANIC"`,
		`"level":"WARN+1"`, // unnamed levels keep the slog default rendering
	}

	if len(lines) != len(expected) {
		t.Fatalf("got %d lines, expected %d: %q", len(lines), len(expected), lines)
	}

	for i, want := range expected {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d: %s not found in %s", i, want, lines[i])
		}
	}
}

func TestLevelDefaultsToTrace(t *testing.T) {
	// zerolog parity: everything is logged until a level is set.
	// The freshly-constructed default is checked (not the shared Level, which
	// other tests legitimately mutate).
	if got := defaultLevel().Level(); got != LevelTrace {
		t.Errorf("default Level = %v, expected %v", got, LevelTrace)
	}
}
