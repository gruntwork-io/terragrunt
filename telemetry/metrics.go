package telemetry

import (
	"context"
	"time"

	"github.com/gruntwork-io/go-commons/env"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

type metricsExporterType string

const (
	noneMetricsExporterType     metricsExporterType = "none"
	oltpHttpMetricsExporterType metricsExporterType = "otlpHttp"
	grpcHttpMetricsExporterType metricsExporterType = "grpcHttp"
)

func Time(opts *options.TerragruntOptions, name string, fn func(childCtx context.Context) error) error {
	ctx := opts.CtxTelemetryCtx
	if ctx == nil || metricExporter == nil {
		return fn(ctx)
	}

	//metricExporter.
	//		meter = otel.Meter(opts.AppName)
	return nil
}

// configureMetricsCollection - configure the metrics collection
func configureMetricsCollection(ctx context.Context, opts *TelemetryOptions) error {
	exporter, err := newMetricsExporter(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}
	metricExporter = exporter
	if metricExporter == nil {
		return nil
	}
	metricProvider, err := newMetricsProvider(opts, metricExporter)
	if err != nil {
		return errors.WithStack(err)
	}
	otel.SetMeterProvider(metricProvider)
	// configure app meter
	meter = otel.GetMeterProvider().Meter(opts.AppName)
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
		metric.WithReader(metric.NewPeriodicReader(exp, metric.WithInterval(1*time.Second))),
	)
	return meterProvider, nil
}
