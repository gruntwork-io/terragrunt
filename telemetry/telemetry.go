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
var metricExporter metric.Exporter

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
	}
	if metricExporter != nil {
		if err := metricExporter.Shutdown(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// Telemetry - collect telemetry from function execution - metrics and traces.
func Telemetry(opts *options.TerragruntOptions, name string, attrs map[string]interface{}, fn func(childCtx context.Context) error) error {
	// wrap telemetry collection with trace and time metric
	return Trace(opts, name, attrs, func(childCtx context.Context) error {
		return Time(opts, name, attrs, fn)
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

// ErrorMissingEnvVariable error for missing TERRAGRUNT_TELEMERTY_TRACE_EXPORTER_HTTP_ENDPOINT
type ErrorMissingEnvVariable struct {
	Vars []string
}

func (e *ErrorMissingEnvVariable) Error() string {
	return fmt.Sprintf("missing environment variable: %v", e.Vars)
}
