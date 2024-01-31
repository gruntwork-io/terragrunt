package telemetry

import (
	"context"

	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	oteltrace "go.opentelemetry.io/otel/sdk/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

var telemetryExporter oteltrace.SpanExporter
var traceProvider *sdktrace.TracerProvider
var rootTracer trace.Tracer

type TelemetryOptions struct {
	AppName    string
	AppVersion string
}

func InitTelemetry(ctx context.Context, opts *TelemetryOptions) error {
	// TODO: add opt in flag, disabled by default
	exp, err := newExporter(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}
	telemetryExporter = exp
	traceProvider, err = newTraceProvider(opts, telemetryExporter)
	if err != nil {
		return errors.WithStack(err)
	}
	otel.SetTracerProvider(traceProvider)
	rootTracer = traceProvider.Tracer(opts.AppName)
	return nil
}

func ShutdownTelemetry(ctx context.Context) error {
	if traceProvider != nil {
		return traceProvider.Shutdown(ctx)
	}
	return nil
}

func OpenSpan(ctx context.Context, name string, attrs map[string]interface{}) (context.Context, trace.Span) {
	if traceProvider == nil {
		return ctx, nil
	}
	childCtx, span := rootTracer.Start(ctx, name)
	// TODO: add attributes
	// span.SetAttributes()
	return childCtx, span
}

func Span(ctx context.Context, name string, attrs map[string]interface{}, fn func(childCtx context.Context) error) error {
	childCtx, span := OpenSpan(ctx, name, attrs)
	defer func() {
		if span != nil {
			span.End()
		}
	}()
	return fn(childCtx)
}

func newExporter(ctx context.Context, opts *TelemetryOptions) (sdktrace.SpanExporter, error) {
	return stdouttrace.New()
}

func newTraceProvider(opts *TelemetryOptions, exp sdktrace.SpanExporter) (*sdktrace.TracerProvider, error) {
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(opts.AppName),
			semconv.ServiceVersion(opts.AppVersion),
		),
	)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(r),
	), nil
}
