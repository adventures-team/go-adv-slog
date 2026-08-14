package advslog_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	advslog "github.com/adventures-team/go-adv-slog"
)

// stdoutLogger returns a JSON logger with deterministic output (no time) for
// the examples.
func stdoutLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
}

// The logger enriched with request-scoped fields travels in the context;
// every layer below logs through it without passing the logger explicitly.
func ExampleCtx() {
	logger := stdoutLogger().With("request_id", "req-42")
	ctx := advslog.NewContext(context.Background(), logger)

	processRequest(ctx)
	// Output: {"level":"INFO","msg":"request processed","request_id":"req-42","status":200}
}

func processRequest(ctx context.Context) {
	advslog.Ctx(ctx).Info("request processed", "status", 200)
}

func ExampleErr() {
	logger := stdoutLogger()

	_, err := os.Open("/nonexistent")
	logger.LogAttrs(context.Background(), slog.LevelError, "open failed", advslog.Err(err))
	// Output: {"level":"ERROR","msg":"open failed","error":"open /nonexistent: no such file or directory"}
}

func ExampleParseLevel() {
	for _, name := range []string{"warn", "trace", "fatal"} {
		level, _ := advslog.ParseLevel(name)
		fmt.Println(level)
	}
	// Output:
	// WARN
	// DEBUG-4
	// ERROR+4
}

// Init is called once at program startup; everything else uses the standard
// slog API or the context helpers.
func ExampleInit() {
	logger := advslog.Init()
	logger.Info("service started", "version", "1.2.3")

	slog.Info("the global slog logger works too")
}

// Recover logs a panic with its typed reason and stack trace through the
// context logger. Use it in goroutines and worker loops.
func ExampleRecover() {
	ctx := advslog.NewContext(context.Background(), stdoutLogger())

	go func() {
		defer advslog.Recover(ctx, "panic in the worker")

		panic("unexpected state")
	}()
}
