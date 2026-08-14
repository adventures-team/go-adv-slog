package advslog

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestErr(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(jsonHandler(&buf, false))

	logger.LogAttrs(context.Background(), slog.LevelError, "failed", Err(errors.New("boom")))
	if !strings.Contains(buf.String(), `"error":"boom"`) {
		t.Errorf(`"error":"boom" not found in %s`, buf.String())
	}

	buf.Reset()
	logger.LogAttrs(context.Background(), slog.LevelInfo, "ok", Err(nil))
	if strings.Contains(buf.String(), "error") {
		t.Errorf("Err(nil) produced an error attr: %s", buf.String())
	}
}

func TestFatal(t *testing.T) {
	exitCode := -1
	osExit = func(code int) { exitCode = code }
	defer func() { osExit = os.Exit }()

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(jsonHandler(&buf, false)))
	defer slog.SetDefault(prev)

	Fatal("fatal happened", Err(errors.New("boom")))

	if exitCode != 1 {
		t.Errorf("exit code = %d, expected 1", exitCode)
	}
	if !strings.Contains(buf.String(), `"level":"FATAL"`) || !strings.Contains(buf.String(), `"msg":"fatal happened"`) {
		t.Errorf("unexpected fatal record: %s", buf.String())
	}
}

func TestFatalContext(t *testing.T) {
	exitCode := -1
	osExit = func(code int) { exitCode = code }
	defer func() { osExit = os.Exit }()

	var buf bytes.Buffer
	ctx := NewContext(context.Background(), slog.New(jsonHandler(&buf, false)))

	FatalContext(ctx, "fatal in ctx")

	if exitCode != 1 {
		t.Errorf("exit code = %d, expected 1", exitCode)
	}
	if !strings.Contains(buf.String(), `"msg":"fatal in ctx"`) {
		t.Errorf("unexpected fatal record: %s", buf.String())
	}
}
