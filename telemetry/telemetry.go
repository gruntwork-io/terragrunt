package telemetry

import (
	"context"

	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	oteltrace "go.opentelemetry.io/otel/sdk/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

var telemetryExporter *oteltrace.SpanExporter

var traceProvider *sdktrace.TracerProvider

func InitTelemetry(ctx context.Context, app *cli.App) error {
	// TODO: add opt in flag, disabled by default
	exporter, err := newConsoleExporter(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	telemetryExporter = &exporter
	traceProvider, err = newTraceProvider(app, exporter)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func newConsoleExporter(ctx context.Context) (oteltrace.SpanExporter, error) {
	return stdouttrace.New(stdouttrace.WithPrettyPrint())
}

func newTraceProvider(app *cli.App, exp sdktrace.SpanExporter) (*sdktrace.TracerProvider, error) {
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("Terragrunt"),
			semconv.ServiceVersion(app.Version),
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
