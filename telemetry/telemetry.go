package telemetry

import (
	"context"

	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	oteltrace "go.opentelemetry.io/otel/sdk/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

var telemetryExporter *oteltrace.SpanExporter

var traceProvider *sdktrace.TracerProvider

type TelemetryOptions struct {
	AppVersion string
}

func InitTelemetry(ctx context.Context, opts *TelemetryOptions) error {
	// TODO: add opt in flag, disabled by default
	exporter, err := newConsoleExporter(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	telemetryExporter = &exporter
	traceProvider, err = newTraceProvider(opts, exporter)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func ShutdownTelemetry(ctx context.Context) error {
	if telemetryExporter != nil {
		return (*telemetryExporter).Shutdown(ctx)
	}
	return nil
}

func newConsoleExporter(ctx context.Context) (oteltrace.SpanExporter, error) {
	return stdouttrace.New(stdouttrace.WithPrettyPrint())
}

func newTraceProvider(opts *TelemetryOptions, exp sdktrace.SpanExporter) (*sdktrace.TracerProvider, error) {
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("Terragrunt"),
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
