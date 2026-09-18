package advslog

import (
	"io"
	stdlog "log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInit exercises the once-guarded global Init exactly once for the whole
// package: GO_LOG pointing at a file (not a TTY) must produce JSON and
// redirect the standard log package.
func TestInit(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "test.log")
	t.Setenv(EnvGoLog, logPath)
	t.Cleanup(func() { Level.Set(LevelTrace) }) // WithLevel mutates the shared Level

	logger := Init(WithLevel(slog.LevelDebug))

	if logger == nil {
		t.Fatal("Init returned nil")
	}
	if slog.Default() != logger {
		t.Error("Init did not install the default logger")
	}
	if Level.Level() != slog.LevelDebug {
		t.Errorf("WithLevel not applied: %v", Level.Level())
	}
	if again := Init(); again != logger {
		t.Error("second Init returned a different logger")
	}

	logger.Debug("json line", "k", "v")
	stdlog.Print("via stdlib log")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}

	out := string(data)
	for _, want := range []string{`"level":"DEBUG"`, `"msg":"json line"`, `"k":"v"`, `"msg":"via stdlib log"`} {
		if !strings.Contains(out, want) {
			t.Errorf("%s not found in log file: %s", want, out)
		}
	}
}

// tbRecorder stands in for a testing.TB, recording each line written through
// Output (the handler writes one per record).
type tbRecorder struct{ lines []string }

func (r *tbRecorder) Output() io.Writer { return r }

func (r *tbRecorder) Write(p []byte) (int, error) {
	r.lines = append(r.lines, strings.TrimSuffix(string(p), "\n"))

	return len(p), nil
}

func TestInitTest(t *testing.T) {
	prev := slog.Default()
	defer slog.SetDefault(prev)

	t.Run("no color with NO_COLOR", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")

		rec := &tbRecorder{}
		logger := InitTest(rec)

		if slog.Default() != logger {
			t.Error("InitTest did not install the default logger")
		}

		logger.Info("test line", "k", "v")

		if len(rec.lines) != 1 {
			t.Fatalf("got %d lines, expected 1: %q", len(rec.lines), rec.lines)
		}
		if strings.Contains(rec.lines[0], "\x1b[") {
			t.Errorf("found ANSI escapes despite NO_COLOR: %q", rec.lines[0])
		}
		for _, want := range []string{"INF", "test line", "k=v"} {
			if !strings.Contains(rec.lines[0], want) {
				t.Errorf("%s not found in %q", want, rec.lines[0])
			}
		}
	})

	t.Run("color by default", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")

		rec := &tbRecorder{}
		logger := InitTest(rec)
		logger.Info("colored line")

		if len(rec.lines) != 1 {
			t.Fatalf("got %d lines, expected 1: %q", len(rec.lines), rec.lines)
		}
		if !strings.Contains(rec.lines[0], "\x1b[") {
			t.Errorf("no ANSI escapes in default-color mode: %q", rec.lines[0])
		}
	})

	// and the real thing, visible with -v and on failures
	logger := InitTest(t)
	logger.Info("hello from InitTest", "test", t.Name())
}

func TestInitTestCustomLevelNames(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	prev := slog.Default()
	defer slog.SetDefault(prev)

	old := Level.Level()
	Level.Set(LevelTrace)
	defer Level.Set(old)

	rec := &tbRecorder{}
	logger := InitTest(rec)

	logger.Log(nil, LevelTrace, "trace visible") //nolint:staticcheck // nil ctx tolerance is part of the contract

	if len(rec.lines) != 1 || !strings.Contains(rec.lines[0], "TRACE") {
		t.Errorf("TRACE not rendered by the test handler: %q", rec.lines)
	}
}
