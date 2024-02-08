package telemetry

import (
	"context"
	"fmt"
	"io"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"

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

// TelemetryOptions - options for telemetry provider.
type TelemetryOptions struct {
	Vars       map[string]string
	AppName    string
	AppVersion string
	Writer     io.Writer
	ErrWriter  io.Writer
}

var telemetryExporter oteltrace.SpanExporter
var traceProvider *sdktrace.TracerProvider
var rootTracer trace.Tracer

type telemetryExporterType string

const (
	consoleType  telemetryExporterType = "console"
	otlpHttpType telemetryExporterType = "otlpHttp"
	otlpGrpcType telemetryExporterType = "otlpGrpc"
	httpType     telemetryExporterType = "http"
)

// InitTelemetry - initialize the telemetry provider.
func InitTelemetry(ctx context.Context, opts *TelemetryOptions) error {

	if !env.GetBool(opts.Vars["TERRAGRUNT_TELEMETRY_ENABLED"], false) {
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

// ShutdownTelemetry - shutdown the telemetry provider.
func ShutdownTelemetry(ctx context.Context) error {
	if traceProvider != nil {
		return traceProvider.Shutdown(ctx)
	}
	return nil
}

// Trace - span execution of a function with attributes.
func Trace(opts *options.TerragruntOptions, name string, attrs map[string]interface{}, fn func(childCtx context.Context) error) error {
	ctx := opts.CtxTelemetryCtx
	if traceProvider == nil || ctx == nil { // invoke function without tracing
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

// newExporter - create a new exporter based on the telemetry options.
func newExporter(ctx context.Context, opts *TelemetryOptions) (sdktrace.SpanExporter, error) {
	exporterType := telemetryExporterType(env.GetString(opts.Vars["TERRAGRUNT_TELEMETRY_EXPORTER"], string(consoleType)))
	insecure := env.GetBool(opts.Vars["TERRAGRUNT_TELEMERTY_EXPORTER_INSECURE_ENDPOINT"], false)
	switch exporterType {
	case httpType:
		endpoint := env.GetString(opts.Vars["TERRAGRUNT_TELEMERTY_EXPORTER_HTTP_ENDPOINT"], "")
		if endpoint == "" {
			return nil, &ErrorMissingExporterEndpoint{}
		}
		endpointOpt := otlptracehttp.WithEndpoint(endpoint)
		config := []otlptracehttp.Option{endpointOpt}
		if insecure {
			config = append(config, otlptracehttp.WithInsecure())
		}
		return otlptracehttp.New(ctx, config...)
	case otlpHttpType:
		var config []otlptracehttp.Option
		if insecure {
			config = append(config, otlptracehttp.WithInsecure())
		}
		return otlptracehttp.New(ctx, config...)
	case otlpGrpcType:
		var config []otlptracegrpc.Option
		if insecure {
			config = append(config, otlptracegrpc.WithInsecure())
		}
		return otlptracegrpc.New(ctx)
	case consoleType:
		return stdouttrace.New(stdouttrace.WithWriter(opts.Writer))
	default:
		return stdouttrace.New(stdouttrace.WithWriter(opts.Writer))
	}
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

// mapToAttributes - convert map to attributes to pass to span.SetAttributes.
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
		default:
			attrs = append(attrs, attribute.String(k, fmt.Sprintf("%v", val)))
		}
	}
	return attrs
}

// ErrorMissingExporterEndpoint error for missing TERRAGRUNT_TELEMERTY_EXPORTER_HTTP_ENDPOINT
type ErrorMissingExporterEndpoint struct {
}

func (e *ErrorMissingExporterEndpoint) Error() string {
	return "http exporter requires TERRAGRUNT_TELEMERTY_EXPORTER_HTTP_ENDPOINT defined"
}
