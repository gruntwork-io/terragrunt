package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"
)

type contextKey byte

const (
	telemeterContextKey contextKey = iota
	TraceParentEnv                 = "TRACEPARENT"
)

// ContextWithTelemeter returns a new context with the provided Telemeter attached.
func ContextWithTelemeter(ctx context.Context, telemeter *Telemeter) context.Context {
	return context.WithValue(ctx, telemeterContextKey, telemeter)
}

// TelemeterFromContext retrieves the Telemeter from the context, or nil if not present.
func TelemeterFromContext(ctx context.Context) *Telemeter {
	if val := ctx.Value(telemeterContextKey); val != nil {
		if telemeter, ok := val.(*Telemeter); ok {
			return telemeter
		}
	}

	return new(Telemeter)
}

// TraceParentFromContext returns the W3C traceparent header value from the context's span, or an error if not available.
func TraceParentFromContext(ctx context.Context, telemetry *Options) string {
	span := trace.SpanFromContext(ctx)
	spanContext := span.SpanContext()

	if !spanContext.IsValid() {
		return ""
	}

	if len(telemetry.TraceParent) > 0 {
		return telemetry.TraceParent
	}

	traceID := spanContext.TraceID().String()
	spanID := spanContext.SpanID().String()
	flags := "00"

	if spanContext.TraceFlags().IsSampled() {
		flags = "01"
	}

	return fmt.Sprintf("00-%s-%s-%s", traceID, spanID, flags)
}
