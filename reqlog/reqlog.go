// Package reqlog logs outgoing requests to external APIs — with duration,
// status and masking of sensitive data — through the logger stored in the
// context (see [advslog.Ctx]).
//
// It is a separate package so that its JSON-masking dependencies (via
// logmask) stay out of programs that do not log outgoing requests.
package reqlog

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"time"

	advslog "github.com/adventures-team/go-adv-slog"
	"github.com/adventures-team/go-adv-slog/logmask"
)

// OutgoingRequestLogger logs requests to an external API. The struct holds
// only the settings, not a logger, so it can be stored globally; the logger
// is taken from the context passed to [OutgoingRequestLogger.LogRequest].
type OutgoingRequestLogger struct {
	APIName     string // external API name, written with every request/response record
	paramMasker logmask.ValuesMasker
	respMasker  logmask.XPathMasker
}

// OutgoingRequest logs the response matching a previously logged request. Do
// not retain it after [OutgoingRequest.LogResponse].
type OutgoingRequest struct {
	ctx        context.Context //nolint:containedctx // request-scoped short-lived object, like a span
	logger     *slog.Logger
	started    time.Time
	apiName    string
	respMasker logmask.XPathMasker
}

// NewOutgoingRequestLogger creates the settings object for logging requests
// to an external API.
//   - apiName is written with every request/response log record
//   - sensitiveParams lists the request parameters to mask with
//     [logmask.MaskString]
//   - sensitiveResponsePaths lists the paths (XPath syntax, see
//     [logmask.XPathMasker]) to mask in responses; nil means "use
//     sensitiveParams"
//
// Pass empty non-nil slices to disable masking.
func NewOutgoingRequestLogger(apiName string, sensitiveParams, sensitiveResponsePaths []string) OutgoingRequestLogger {
	if sensitiveResponsePaths == nil {
		sensitiveResponsePaths = sensitiveParams
	}

	return OutgoingRequestLogger{
		APIName:     apiName,
		paramMasker: logmask.NewValuesMasker(sensitiveParams),
		respMasker:  logmask.NewXPathMasker(sensitiveResponsePaths),
	}
}

// SetNeedTrimSpace sets the NeedTrimSpace flag of the response masker:
// additional whitespace removal from the JSON (for servers responding with
// pretty-printed JSON). Off by default, as the operation is not fast.
func (orl *OutgoingRequestLogger) SetNeedTrimSpace(val bool) {
	orl.respMasker.NeedTrimSpace = val
}

// LogRequest logs a request to requestURL with the given params at
// [slog.LevelDebug] through the logger from ctx. Parameters listed in
// sensitiveParams are masked.
//
// The returned object logs the corresponding API response. When debug logging
// is disabled at request time, LogRequest returns nil and the response
// (including an erroneous one) is not logged either; a nil *OutgoingRequest
// is safe to use.
//
// TODO support parameters in JSON format
func (orl OutgoingRequestLogger) LogRequest(ctx context.Context, httpMethod, requestURL string, params url.Values) *OutgoingRequest {
	logger := advslog.Ctx(ctx)
	if !logger.Enabled(ctx, slog.LevelDebug) {
		return nil
	}

	or := &OutgoingRequest{
		ctx:        ctx,
		logger:     logger,
		started:    time.Now(),
		apiName:    orl.APIName,
		respMasker: orl.respMasker,
	}

	logger.LogAttrs(ctx, slog.LevelDebug, orl.APIName+" Request",
		slog.String("method", httpMethod),
		slog.String("url", requestURL),
		slog.Any("params", orl.paramMasker.MaskValues(params)),
	)

	return or
}

// LogResponse logs the external API response together with the request
// duration.
//
// The body is expected to be JSON: it is masked and embedded verbatim as the
// "response" attr. A body that fails to parse is logged as a string under
// "broken_response". A non-nil err raises the record level to
// [slog.LevelError].
func (or *OutgoingRequest) LogResponse(statusCode int, body []byte, err error) {
	if or == nil {
		return
	}

	level := slog.LevelDebug
	if err != nil {
		level = slog.LevelError
	}

	if !or.logger.Enabled(or.ctx, level) {
		return
	}

	attrs := make([]slog.Attr, 0, 4)

	if err != nil {
		attrs = append(attrs, advslog.Err(err))
	}

	if statusCode != 0 {
		attrs = append(attrs, slog.Int("status", statusCode))
	}

	attrs = append(attrs, slog.Duration("duration", time.Since(or.started)))

	if masked, maskErr := or.respMasker.MaskJSON(body); maskErr == nil {
		attrs = append(attrs, slog.Any("response", json.RawMessage(masked)))
	} else {
		// TODO strip sensitive data from the broken body with regexps, see the comments in logmask
		attrs = append(attrs, slog.String("broken_response", string(body)))
	}

	or.logger.LogAttrs(or.ctx, level, or.apiName+" Response", attrs...)
}
