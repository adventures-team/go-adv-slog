package advslog_test

import (
	"context"
	"log/slog"

	advslog "github.com/adventures-team/go-adv-slog"
)

// fetchFromAPI simulates code in an external library that knows nothing about
// this package: it logs with a plain log/slog context-aware call through the
// default logger, not through [advslog.Ctx].
func fetchFromAPI(ctx context.Context) {
	slog.InfoContext(ctx, "fetching", "attempt", 1)
}

// With context routing enabled, records logged by foreign code through the
// default logger's *Context methods are routed to the request-scoped logger
// stored in the context and pick up its attributes.
func ExampleWithContextRouting() {
	advslog.Init(advslog.WithContextRouting())

	ctx := advslog.NewContext(context.Background(),
		slog.Default().With("request_id", "req-42"))

	fetchFromAPI(ctx)
	// The foreign record carries the context logger's attributes:
	// {"time":"...","level":"INFO","msg":"fetching","request_id":"req-42","attempt":1}
}
