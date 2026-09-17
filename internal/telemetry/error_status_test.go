package telemetry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func newTestTracer(t *testing.T) (*Tracer, *tracetest.InMemoryExporter) {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter)))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	return &Tracer{
		Tracer:       provider.Tracer("test"),
		provider:     provider,
		spanExporter: exporter,
	}, exporter
}

// TestTraceSetsErrorStatusAndType verifies that a failed operation records the
// span status and error.type attribute, per the OpenTelemetry recording-errors
// convention, so backends that classify errors by span status detect failures.
func TestTraceSetsErrorStatusAndType(t *testing.T) {
	t.Parallel()

	tracer, exporter := newTestTracer(t)

	sentinel := errors.New("boom")

	err := tracer.Trace(t.Context(), "op", nil, func(context.Context) error {
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	span := spans[0]

	require.Equal(t, codes.Error, span.Status.Code, "span status must be Error")
	require.Equal(t, "boom", span.Status.Description, "span status description must carry the error message")

	var errorType string

	for _, kv := range span.Attributes {
		if kv.Key == attribute.Key("error.type") {
			errorType = kv.Value.AsString()
		}
	}

	require.Equal(t, "*errors.errorString", errorType, "error.type must be set to the error's type")
}

// TestTraceSuccessLeavesStatusUnset verifies that a successful operation does
// not mark the span as errored.
func TestTraceSuccessLeavesStatusUnset(t *testing.T) {
	t.Parallel()

	tracer, exporter := newTestTracer(t)

	require.NoError(t, tracer.Trace(t.Context(), "op", nil, func(context.Context) error {
		return nil
	}))

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	require.Equal(t, codes.Unset, spans[0].Status.Code, "successful span must not be marked as errored")
}
