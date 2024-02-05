package telemetry

import (
	"context"
	"os"

	"github.com/gruntwork-io/terragrunt/options"

	"go.opentelemetry.io/otel/attribute"

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

// TraceFull - span execution of a function with attributes.
func TraceFull(ctx context.Context, name string, attrs map[string]interface{}, fn func(childCtx context.Context) error) error {
	if traceProvider == nil { // invoke function without tracing
		return fn(ctx)
	}
	childCtx, span := openSpan(ctx, name, attrs)
	defer func() {
		span.End()
	}()
	err := fn(childCtx)
	if err != nil {
		// record error in span
		span.RecordError(err)
	}

	return err
}

// Trace - span execution of a function.
func Trace(ctx context.Context, name string, fn func(childCtx context.Context) error) error {
	return TraceFull(ctx, name, map[string]interface{}{}, fn)
}

func TraceCommand(ctx context.Context, opts *options.TerragruntOptions, fn func(childCtx context.Context) error) error {
	return TraceFull(ctx, opts.TerraformCommand, map[string]interface{}{
		"command": opts.TerraformCommand,
		"args":    opts.TerraformCliArgs,
		"dir":     opts.WorkingDir,
	}, fn)
}

func openSpan(ctx context.Context, name string, attrs map[string]interface{}) (context.Context, trace.Span) {
	if traceProvider == nil {
		return ctx, nil
	}
	childCtx, span := rootTracer.Start(ctx, name)
	// convert attrs map to span.SetAttributes
	span.SetAttributes(mapToAttributes(attrs)...)
	return childCtx, span
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

func mapToAttributes(data map[string]interface{}) []attribute.KeyValue {
	var attrs []attribute.KeyValue
	for k, v := range data {
		switch val := v.(type) {
		case string:
			attrs = append(attrs, attribute.String(k, val))
		case int:
			attrs = append(attrs, attribute.Int64(k, int64(val)))
		case int64:
			attrs = append(attrs, attribute.Int64(k, val))
		case float64:
			attrs = append(attrs, attribute.Float64(k, val))
		case bool:
			attrs = append(attrs, attribute.Bool(k, val))
		// Add other types as necessary
		default:
			// Handle or ignore unsupported types
		}
	}
	return attrs
}
