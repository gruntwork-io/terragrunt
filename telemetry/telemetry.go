package telemetry

import (
	"context"
	"fmt"
	"io"

	"github.com/gruntwork-io/terragrunt/options"

	"github.com/pkg/errors"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"

	"go.opentelemetry.io/otel/attribute"

	oteltrace "go.opentelemetry.io/otel/sdk/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
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

var meter otelmetric.Meter
var metricProvider *metric.MeterProvider
var metricExporter metric.Exporter

var parentTraceID *trace.TraceID
var parentSpanID *trace.SpanID
var parentTraceFlags *trace.TraceFlags

// InitTelemetry - initialize the telemetry provider.
func InitTelemetry(ctx context.Context, opts *TelemetryOptions) error {

	if err := configureTraceCollection(ctx, opts); err != nil {
		return errors.WithStack(err)
	}

	if err := configureMetricsCollection(ctx, opts); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// ShutdownTelemetry - shutdown the telemetry provider.
func ShutdownTelemetry(ctx context.Context) error {
	if traceProvider != nil {
		if err := traceProvider.Shutdown(ctx); err != nil {
			return errors.WithStack(err)
		}
		traceProvider = nil
	}
	if metricProvider != nil {
		if err := metricProvider.Shutdown(ctx); err != nil {
			return errors.WithStack(err)
		}
		metricProvider = nil
	}
	return nil
}

// Telemetry - collect telemetry from function execution - metrics and traces.
func Telemetry(ctx context.Context, opts *options.TerragruntOptions, name string, attrs map[string]interface{}, fn func(childCtx context.Context) error) error {
	// wrap telemetry collection with trace and time metric
	return Trace(ctx, name, attrs, func(ctx context.Context) error {
		return Time(ctx, name, attrs, fn)
	})
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

// GetValue - get variable value, first check for key if not found check for deprecated key.
func (to *TelemetryOptions) GetValue(key, deprecated string) string {
	// check for key
	if value, found := to.Vars[key]; found {
		return value
	}
	// check for deprecated key and print warning
	if value, found := to.Vars[deprecated]; found {
		// print deprecation warning
		_, _ = fmt.Fprintf(to.ErrWriter, "WARNING: %s is deprecated, use %s instead\n", deprecated, key)
		return value

	}

	return ""
}

// ErrorMissingEnvVariable error for missing environment variable.
type ErrorMissingEnvVariable struct {
	Vars []string
}

func (e *ErrorMissingEnvVariable) Error() string {
	return fmt.Sprintf("missing environment variable: %v", e.Vars)
}
