package telemetry

import (
	"context"
	"fmt"
	"io"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/sdk/metric"

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

var spanExporter oteltrace.SpanExporter
var traceProvider *sdktrace.TracerProvider
var rootTracer trace.Tracer

type traceExporterType string
type metricsExporterType string

var metricExporter metric.Exporter

const (
	noneTraceExporterType     traceExporterType = "none"
	consoleTraceExporterType  traceExporterType = "console"
	otlpHttpTraceExporterType traceExporterType = "otlpHttp"
	otlpGrpcTraceExporterType traceExporterType = "otlpGrpc"
	httpTraceExporterType     traceExporterType = "http"

	noneMetricsExporterType     metricsExporterType = "none"
	oltpHttpMetricsExporterType metricsExporterType = "otlpHttp"
	grpcHttpMetricsExporterType metricsExporterType = "grpcHttp"
)

// InitTelemetry - initialize the telemetry provider.
func InitTelemetry(ctx context.Context, opts *TelemetryOptions) error {

	if !env.GetBool(opts.Vars["TERRAGRUNT_TELEMETRY_ENABLED"], false) {
		return nil
	}

	if err := configureTraceCollection(ctx, opts); err != nil {
		return err
	}

	if err := configureMetricsCollection(ctx, opts); err != nil {
		return err
	}

	return nil
}

// configureMetricsCollection - configure the metrics collection
func configureMetricsCollection(ctx context.Context, opts *TelemetryOptions) error {
	exporter, err := newMetricsExporter(ctx, opts)
	if err != nil {
		return err
	}
	metricExporter = exporter
	return nil
}

// newMetricsExporter - create a new exporter based on the telemetry options.
func newMetricsExporter(ctx context.Context, opts *TelemetryOptions) (metric.Exporter, error) {
	exporterType := metricsExporterType(env.GetString(opts.Vars["TERRAGRUNT_TELEMETRY_METRIC_EXPORTER"], string(noneMetricsExporterType)))
	insecure := env.GetBool(opts.Vars["TERRAGRUNT_TELEMERTY_METRIC_EXPORTER_INSECURE_ENDPOINT"], false)
	switch exporterType {
	case oltpHttpMetricsExporterType:
		var config []otlpmetrichttp.Option
		if insecure {
			config = append(config, otlpmetrichttp.WithInsecure())
		}
		return otlpmetrichttp.New(ctx, config...)
	case grpcHttpMetricsExporterType:
		var config []otlpmetricgrpc.Option
		if insecure {
			config = append(config, otlpmetricgrpc.WithInsecure())
		}
		return otlpmetricgrpc.New(ctx, config...)
	case noneMetricsExporterType:
		return nil, nil
	default:
		return nil, nil

	}
}

// ShutdownTelemetry - shutdown the telemetry provider.
func ShutdownTelemetry(ctx context.Context) error {
	if traceProvider != nil {
		if err := traceProvider.Shutdown(ctx); err != nil {
			return err
		}
	}
	if metricExporter != nil {
		if err := metricExporter.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Trace - span execution of a function with attributes.
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

// ErrorMissingEnvVariable error for missing TERRAGRUNT_TELEMERTY_TRACE_EXPORTER_HTTP_ENDPOINT
type ErrorMissingEnvVariable struct {
	Vars []string
}

func (e *ErrorMissingEnvVariable) Error() string {
	return fmt.Sprintf("missing environment variable: %v", e.Vars)
}
