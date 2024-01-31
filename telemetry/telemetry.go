package telemetry

import (
	"context"
	"os"

	"github.com/gruntwork-io/go-commons/env"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"

	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	oteltrace "go.opentelemetry.io/otel/sdk/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

type TelemetryOptions struct {
	AppName    string
	AppVersion string
}

var telemetryExporter oteltrace.SpanExporter
var traceProvider *sdktrace.TracerProvider
var rootTracer trace.Tracer

type telemetryExporterType string

const (
	consoleType telemetryExporterType = "console"
	httpType    telemetryExporterType = "http"
)

func InitTelemetry(ctx context.Context, opts *TelemetryOptions) error {

	if env.GetBool(os.Getenv("TERRAGRUNT_TELEMETRY_ENABLED"), false) == false {
		return nil
	}

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
	exporterType := telemetryExporterType(env.GetString(os.Getenv("TERRAGRUNT_TELEMETRY_EXPORTER"), string(consoleType)))
	switch exporterType {
	case httpType:
		endpoint := env.GetString(os.Getenv("TERRAGRUNT_TELEMERTY_EXPORTER_HTTP_ENDPOINT"), "")
		insecureOpt := otlptracehttp.WithInsecure()
		endpointOpt := otlptracehttp.WithEndpoint(endpoint)
		return otlptracehttp.New(ctx, insecureOpt, endpointOpt)
	default:
		return stdouttrace.New()
	}
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
