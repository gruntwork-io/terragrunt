package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

const (
	telemeterContextKey ctxKey = iota
)

type ctxKey byte

func ContextWithTelemeter(ctx context.Context, tlm *Telemeter) context.Context {
	return context.WithValue(ctx, telemeterContextKey, tlm)
}

func TelemeterFromContext(ctx context.Context) *Telemeter {
	if val := ctx.Value(telemeterContextKey); val != nil {
		if val, ok := val.(*Telemeter); ok {
			return val
		}
	}

	return new(Telemeter)
}

func TraceParentFromContext(ctx context.Context) (string, error) {
	span := trace.SpanFromContext(ctx)
	spanContext := span.SpanContext()

	if !spanContext.IsValid() {
		return "", nil
	}

	traceID := spanContext.TraceID().String()
	spanID := spanContext.SpanID().String()
	flags := "00"

	if spanContext.TraceFlags().IsSampled() {
		flags = "01"
	}

	traceparent := "00-" + traceID + "-" + spanID + "-" + flags

	return traceparent, nil
}
