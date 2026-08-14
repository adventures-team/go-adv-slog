package reqlog_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"

	advslog "github.com/adventures-team/go-adv-slog"
	"github.com/adventures-team/go-adv-slog/reqlog"
)

const testAPIURL = "https://example.com/api/v1/test/none"

var testLogger = reqlog.NewOutgoingRequestLogger("Test API", []string{"pin", "access_token"}, nil)

func testCtx(buf *bytes.Buffer, level slog.Level) context.Context {
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: level}))

	return advslog.NewContext(context.Background(), logger)
}

func TestOutgoingRequestLogger(t *testing.T) {
	var buf bytes.Buffer
	ctx := testCtx(&buf, slog.LevelDebug)

	params := make(url.Values)
	params.Set("test", "hello world")
	params.Set("pin", "1111")

	reqLog := testLogger.LogRequest(ctx, http.MethodPost, testAPIURL, params)
	if reqLog == nil {
		t.Fatal("LogRequest returned nil with debug logging enabled")
	}

	out := buf.String()
	for _, want := range []string{
		`"msg":"Test API Request"`,
		`"method":"POST"`,
		`"url":"` + testAPIURL + `"`,
		`"pin":["*****"]`,        // masked
		`"test":["hello world"]`, // untouched
	} {
		if !strings.Contains(out, want) {
			t.Errorf("request: %s not found in %s", want, out)
		}
	}

	buf.Reset()
	reqLog.LogResponse(http.StatusOK, []byte(`{"status":"ok", "access_token":"13213sfsad34wrgt4reft34ewrg43r"}`), nil)

	out = buf.String()
	for _, want := range []string{
		`"msg":"Test API Response"`,
		`"level":"DEBUG"`,
		`"status":200`,
		`"duration":`,
		`"response":{"status":"ok","access_token":"13213s*****"}`, // raw JSON embedding, token masked (sjson drops the space when rewriting)
	} {
		if !strings.Contains(out, want) {
			t.Errorf("response: %s not found in %s", want, out)
		}
	}
}

func TestLogResponseError(t *testing.T) {
	var buf bytes.Buffer
	ctx := testCtx(&buf, slog.LevelDebug)

	reqLog := testLogger.LogRequest(ctx, http.MethodGet, testAPIURL, nil)
	buf.Reset()

	reqLog.LogResponse(http.StatusBadGateway, []byte(`{"status":"error"}`), errors.New("upstream failed"))

	out := buf.String()
	for _, want := range []string{`"level":"ERROR"`, `"error":"upstream failed"`, `"status":502`} {
		if !strings.Contains(out, want) {
			t.Errorf("%s not found in %s", want, out)
		}
	}
}

func TestLogResponseBrokenJSON(t *testing.T) {
	var buf bytes.Buffer
	ctx := testCtx(&buf, slog.LevelDebug)

	reqLog := testLogger.LogRequest(ctx, http.MethodGet, testAPIURL, nil)
	buf.Reset()

	reqLog.LogResponse(http.StatusOK, []byte(`<html>not json</html>`), nil)

	if !strings.Contains(buf.String(), `"broken_response":"<html>not json</html>"`) {
		t.Errorf("broken_response not found in %s", buf.String())
	}
}

func TestLogRequestDebugDisabled(t *testing.T) {
	var buf bytes.Buffer
	ctx := testCtx(&buf, slog.LevelInfo) // debug disabled

	reqLog := testLogger.LogRequest(ctx, http.MethodGet, testAPIURL, nil)
	if reqLog != nil {
		t.Error("LogRequest did not return nil with debug logging disabled")
	}
	if buf.Len() != 0 {
		t.Errorf("LogRequest logged with debug disabled: %s", buf.String())
	}

	// nil receiver must be a no-op
	reqLog.LogResponse(http.StatusOK, []byte(`{}`), nil)
	if buf.Len() != 0 {
		t.Errorf("nil LogResponse logged: %s", buf.String())
	}
}
