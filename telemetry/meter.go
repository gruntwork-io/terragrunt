package telemetry

import (
	"context"
	"io"
	"regexp"
	"time"

	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

const (
	noneMetricsExporterType     metricsExporterType = "none"
	consoleMetricsExporterType  metricsExporterType = "console"
	oltpHTTPMetricsExporterType metricsExporterType = "otlpHttp"
	grpcHTTPMetricsExporterType metricsExporterType = "grpcHttp"

	ErrorsCounter = "errors"

	readerInterval = 1 * time.Second
)

var (
	metricNameCleanPattern     = regexp.MustCompile(`[^A-Za-z0-9_.-/]`)
	multipleUnderscoresPattern = regexp.MustCompile(`_+`)
)

type metricsExporterType string

type Meter struct {
	otelmetric.Meter
	provider *metric.MeterProvider
	exporter metric.Exporter
}

// NewMeter creates and configures the metrics collection.
func NewMeter(ctx context.Context, appName, appVersion string, writer io.Writer, opts *Options) (*Meter, error) {
	exporter, err := NewMetricsExporter(ctx, writer, opts)
	if err != nil {
		return nil, errors.New(err)
	}

	if exporter == nil {
		return nil, nil
	}

	provider, err := newMetricsProvider(exporter, appName, appVersion)
	if err != nil {
		return nil, errors.New(err)
	}

	otel.SetMeterProvider(provider)

	meter := &Meter{
		Meter:    otel.GetMeterProvider().Meter(appName),
		provider: provider,
		exporter: exporter,
	}

	return meter, nil
}

// Time collects time for function execution
func (meter *Meter) Time(ctx context.Context, name string, attrs map[string]any, fn func(childCtx context.Context) error) error {
	if meter == nil || meter.exporter == nil {
		return fn(ctx)
	}

	metricAttrs := mapToAttributes(attrs)

	histogram, err := meter.Int64Histogram(CleanMetricName(name + "_duration"))
	if err != nil {
		return errors.New(err)
	}

	startTime := time.Now()
	err = fn(ctx)

	histogram.Record(ctx, time.Since(startTime).Milliseconds(), otelmetric.WithAttributes(metricAttrs...))

	if err != nil {
		// count errors
		meter.Count(ctx, ErrorsCounter, 1)
		meter.Count(ctx, name+"_errors", 1)
	} else {
		meter.Count(ctx, name+"_success", 1)
	}

	return err
}

// Count adds to counter provided value.
func (meter *Meter) Count(ctx context.Context, name string, value int64) {
	if ctx == nil || meter == nil || meter.exporter == nil {
		return
	}

	counter, err := meter.Int64Counter(CleanMetricName(name + "_count"))
	if err != nil {
		return
	}

	counter.Add(ctx, value)
}

// NewMetricsExporter - create a new exporter based on the telemetry options.
func NewMetricsExporter(ctx context.Context, writer io.Writer, opts *Options) (metric.Exporter, error) {
	exporterType := metricsExporterType(opts.MetricExporter)
	if exporterType == "" {
		exporterType = noneMetricsExporterType
	}

	// TODO: Remove this lint suppression
	switch exporterType { //nolint:exhaustive
	case oltpHTTPMetricsExporterType:
		var config []otlpmetrichttp.Option
		if opts.MetricExporterInsecureEndpoint {
			config = append(config, otlpmetrichttp.WithInsecure())
		}

		return otlpmetrichttp.New(ctx, config...)
	case grpcHTTPMetricsExporterType:
		var config []otlpmetricgrpc.Option
		if opts.MetricExporterInsecureEndpoint {
			config = append(config, otlpmetricgrpc.WithInsecure())
		}

		return otlpmetricgrpc.New(ctx, config...)
	case consoleMetricsExporterType:
		return stdoutmetric.New(stdoutmetric.WithWriter(writer))
	default:
		return nil, nil
	}
}

// newMetricsProvider creates a new metrics provider.
func newMetricsProvider(exp metric.Exporter, appName, appVersion string) (*metric.MeterProvider, error) {
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(appName),
			semconv.ServiceVersion(appVersion),
		),
	)
	if err != nil {
		return nil, errors.New(err)
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithResource(r),
		metric.WithReader(metric.NewPeriodicReader(exp, metric.WithInterval(readerInterval))),
	)

	return meterProvider, nil
}
