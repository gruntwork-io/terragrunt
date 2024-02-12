package telemetry

import (
	"context"

	"github.com/gruntwork-io/terragrunt/options"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"

	"github.com/gruntwork-io/go-commons/env"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"

	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

type traceExporterType string

const (
	noneTraceExporterType     traceExporterType = "none"
	consoleTraceExporterType  traceExporterType = "console"
	otlpHttpTraceExporterType traceExporterType = "otlpHttp"
	otlpGrpcTraceExporterType traceExporterType = "otlpGrpc"
	httpTraceExporterType     traceExporterType = "http"
)

// Trace - collect traces for method execution
func Trace(opts *options.TerragruntOptions, name string, attrs map[string]interface{}, fn func(childCtx context.Context) error) error {
	ctx := opts.CtxTelemetryCtx
	if spanExporter == nil || traceProvider == nil || ctx == nil { // invoke function without tracing
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

// configureTraceCollection - configure the traces collection
func configureTraceCollection(ctx context.Context, opts *TelemetryOptions) error {
	exp, err := newTraceExporter(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}
	if exp == nil { // no exporter
		return nil
	}
	spanExporter = exp
	traceProvider, err = newTraceProvider(opts, spanExporter)
	if err != nil {
		return errors.WithStack(err)
	}
	otel.SetTracerProvider(traceProvider)
	rootTracer = traceProvider.Tracer(opts.AppName)
	return nil
}

// newTraceProvider - create a new trace provider with terragrunt version.
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

// newTraceExporter - create a new exporter based on the telemetry options.
func newTraceExporter(ctx context.Context, opts *TelemetryOptions) (sdktrace.SpanExporter, error) {
	exporterType := traceExporterType(env.GetString(opts.Vars["TERRAGRUNT_TELEMETRY_TRACE_EXPORTER"], string(noneTraceExporterType)))
	insecure := env.GetBool(opts.Vars["TERRAGRUNT_TELEMERTY_TRACE_EXPORTER_INSECURE_ENDPOINT"], false)
	switch exporterType {
	case httpTraceExporterType:
		endpoint := env.GetString(opts.Vars["TERRAGRUNT_TELEMERTY_TRACE_EXPORTER_HTTP_ENDPOINT"], "")
		if endpoint == "" {
			return nil, &ErrorMissingEnvVariable{
				Vars: []string{"TERRAGRUNT_TELEMERTY_TRACE_EXPORTER_HTTP_ENDPOINT"},
			}
		}
		endpointOpt := otlptracehttp.WithEndpoint(endpoint)
		config := []otlptracehttp.Option{endpointOpt}
		if insecure {
			config = append(config, otlptracehttp.WithInsecure())
		}
		return otlptracehttp.New(ctx, config...)
	case otlpHttpTraceExporterType:
		var config []otlptracehttp.Option
		if insecure {
			config = append(config, otlptracehttp.WithInsecure())
		}
		return otlptracehttp.New(ctx, config...)
	case otlpGrpcTraceExporterType:
		var config []otlptracegrpc.Option
		if insecure {
			config = append(config, otlptracegrpc.WithInsecure())
		}
		return otlptracegrpc.New(ctx, config...)
	case consoleTraceExporterType:
		return stdouttrace.New(stdouttrace.WithWriter(opts.Writer))
	case noneTraceExporterType:
		// no trace exporter
		return nil, nil
	default:
		return nil, nil
	}
}

// openSpan - create a new span with attributes.
func openSpan(ctx context.Context, name string, attrs map[string]interface{}) (context.Context, trace.Span) {
	if traceProvider == nil {
		return ctx, nil
	}
	childCtx, span := rootTracer.Start(ctx, name)
	// convert attrs map to span.SetAttributes
	span.SetAttributes(mapToAttributes(attrs)...)
	return childCtx, span
}
