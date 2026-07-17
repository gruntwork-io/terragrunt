package telemetry

import (
	"context"
	"io"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	otellog "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

const (
	noneLogsExporterType     logsExporterType = "none"
	consoleLogsExporterType  logsExporterType = "console"
	otlpHTTPLogsExporterType logsExporterType = "otlpHttp"
	otlpGrpcLogsExporterType logsExporterType = "otlpGrpc"
)

type logsExporterType string

// Logger holds the OpenTelemetry logs signal provider and exporter.
type Logger struct {
	provider *log.LoggerProvider
	exporter log.Exporter
}

// NewLogger creates and configures the logs collection. It returns nil when no
// logs exporter is configured, matching the behaviour of [NewMeter] and [NewTracer].
func NewLogger(ctx context.Context, appName, appVersion string, writer io.Writer, opts *Options) (*Logger, error) {
	exporter, err := NewLogsExporter(ctx, writer, opts)
	if err != nil {
		return nil, err
	}

	if exporter == nil {
		return nil, nil
	}

	provider, err := newLogsProvider(ctx, exporter, appName, appVersion)
	if err != nil {
		return nil, err
	}

	otellog.SetLoggerProvider(provider)

	return &Logger{
		provider: provider,
		exporter: exporter,
	}, nil
}

// NewLogsExporter creates a new logs exporter based on the telemetry options.
// The structure mirrors NewMetricsExporter and NewTraceExporter; the per-signal
// OTLP option types prevent sharing a single implementation.
//
//nolint:dupl
func NewLogsExporter(ctx context.Context, writer io.Writer, opts *Options) (log.Exporter, error) {
	exporterType := logsExporterType(opts.LogsExporter)
	if exporterType == "" {
		exporterType = noneLogsExporterType
	}

	switch exporterType {
	case otlpHTTPLogsExporterType:
		var config []otlploghttp.Option
		if opts.LogsExporterInsecureEndpoint {
			config = append(config, otlploghttp.WithInsecure())
		}

		return otlploghttp.New(ctx, config...)
	case otlpGrpcLogsExporterType:
		var config []otlploggrpc.Option
		if opts.LogsExporterInsecureEndpoint {
			config = append(config, otlploggrpc.WithInsecure())
		}

		return otlploggrpc.New(ctx, config...)
	case consoleLogsExporterType:
		return stdoutlog.New(stdoutlog.WithWriter(writer))
	case noneLogsExporterType:
		return nil, nil
	default:
		return nil, nil
	}
}

// newLogsProvider creates a new logs provider with the terragrunt resource attributes.
func newLogsProvider(ctx context.Context, exp log.Exporter, appName, appVersion string) (*log.LoggerProvider, error) {
	r, err := resource.New(ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(
			semconv.ServiceName(appName),
			semconv.ServiceVersion(appVersion),
		),
		resource.WithTelemetrySDK(),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, err
	}

	return log.NewLoggerProvider(
		log.WithResource(r),
		log.WithProcessor(log.NewBatchProcessor(exp)),
	), nil
}
