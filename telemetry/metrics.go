package telemetry

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"

	"github.com/gruntwork-io/go-commons/env"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type metricsExporterType string

const (
	noneMetricsExporterType     metricsExporterType = "none"
	consoleMetricsExporterType  metricsExporterType = "console"
	oltpHttpMetricsExporterType metricsExporterType = "otlpHttp"
	grpcHttpMetricsExporterType metricsExporterType = "grpcHttp"

	ErrorsCounter = "errors"

	readerInterval = 1 * time.Second
)

var metricNameCleanPattern = regexp.MustCompile(`[^A-Za-z0-9_.-/]`)
var multipleUnderscoresPattern = regexp.MustCompile(`_+`)

// Time - collect time for function execution
func Time(ctx context.Context, name string, attrs map[string]interface{}, fn func(childCtx context.Context) error) error {
	if metricExporter == nil {
		return fn(ctx)
	}

	metricAttrs := mapToAttributes(attrs)
	histogram, err := meter.Int64Histogram(CleanMetricName(name + "_duration"))
	if err != nil {
		return errors.WithStack(err)
	}
	startTime := time.Now()
	err = fn(ctx)
	histogram.Record(ctx, time.Since(startTime).Milliseconds(), otelmetric.WithAttributes(metricAttrs...))
	if err != nil {
		// count errors
		Count(ctx, ErrorsCounter, 1)
		Count(ctx, name+"_errors", 1)
	} else {
		Count(ctx, name+"_success", 1)
	}
	return err
}

// Count - add to counter provided value
func Count(ctx context.Context, name string, value int64) {
	if ctx == nil || metricExporter == nil {
		return
	}
	counter, err := meter.Int64Counter(CleanMetricName(name + "_count"))
	if err != nil {
		return
	}
	counter.Add(ctx, value)
}

// configureMetricsCollection - configure the metrics collection
func configureMetricsCollection(ctx context.Context, opts *TelemetryOptions) error {
	exporter, err := NewMetricsExporter(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}
	metricExporter = exporter
	if metricExporter == nil {
		return nil
	}
	provider, err := newMetricsProvider(opts, metricExporter)
	if err != nil {
		return errors.WithStack(err)
	}
	metricProvider = provider
	otel.SetMeterProvider(metricProvider)
	// configure app meter
	meter = otel.GetMeterProvider().Meter(opts.AppName)
	return nil
}

// NewMetricsExporter - create a new exporter based on the telemetry options.
func NewMetricsExporter(ctx context.Context, opts *TelemetryOptions) (metric.Exporter, error) {
	exporterType := metricsExporterType(env.GetString(opts.Vars["TERRAGRUNT_TELEMETRY_METRIC_EXPORTER"], string(noneMetricsExporterType)))
	insecure := env.GetBool(opts.GetValue("TERRAGRUNT_TELEMETRY_METRIC_EXPORTER_INSECURE_ENDPOINT", "TERRAGRUNT_TELEMERTY_METRIC_EXPORTER_INSECURE_ENDPOINT"), false)

	// TODO: Remove this lint suppression
	switch exporterType { //nolint:exhaustive
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
	case consoleMetricsExporterType:
		return stdoutmetric.New(stdoutmetric.WithWriter(opts.Writer))
	default:
		return nil, nil

	}
}

// newMetricsProvider - create a new metrics provider.
func newMetricsProvider(opts *TelemetryOptions, exp metric.Exporter) (*metric.MeterProvider, error) {
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

	meterProvider := metric.NewMeterProvider(
		metric.WithResource(r),
		metric.WithReader(metric.NewPeriodicReader(exp, metric.WithInterval(readerInterval))),
	)
	return meterProvider, nil
}

// CleanMetricName - clean metric name from invalid characters.
func CleanMetricName(metricName string) string {
	cleanedName := metricNameCleanPattern.ReplaceAllString(metricName, "_")
	cleanedName = multipleUnderscoresPattern.ReplaceAllString(cleanedName, "_")
	return strings.Trim(cleanedName, "_")
}
