package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/propagation"
)

type contextKey byte

const (
	telemeterContextKey contextKey = iota
	TraceParentEnv                 = "TRACEPARENT"
	traceParentHeader              = "traceparent"
)

// ContextWithTelemeter returns a new context with the provided Telemeter attached.
func ContextWithTelemeter(ctx context.Context, telemeter *Telemeter) context.Context {
	return context.WithValue(ctx, telemeterContextKey, telemeter)
}

// TelemeterFromContext retrieves the Telemeter from the context.
// Returns a zero-value Telemeter (safe no-op) if not present or nil.
func TelemeterFromContext(ctx context.Context) *Telemeter {
	if val := ctx.Value(telemeterContextKey); val != nil {
		if telemeter, ok := val.(*Telemeter); ok && telemeter != nil {
			return telemeter
		}
	}

	return new(Telemeter)
}

// TraceParentFromContext returns the W3C traceparent header value of the current
// span, so that a child process joins this trace as its child. Falls back to the
// remote parent this process inherited when there is no current span, and to an
// empty string when there is neither.
func TraceParentFromContext(ctx context.Context, telemetry *Options) string {
	carrier := propagation.MapCarrier{}

	propagation.TraceContext{}.Inject(ctx, carrier)

	if traceParent := carrier.Get(traceParentHeader); traceParent != "" {
		return traceParent
	}

	if telemetry != nil {
		return telemetry.TraceParent
	}

	return ""
}
