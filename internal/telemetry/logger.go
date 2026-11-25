package telemetry

import (
	"context"
	"errors"
	"io"
	"strconv"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/sdk/log"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

const (
	noneLogsExporterType     logsExporterType = "none"
	consoleLogsExporterType  logsExporterType = "console"
	otlpHTTPLogsExporterType logsExporterType = "otlpHttp"
	otlpGrpcLogsExporterType logsExporterType = "otlpGrpc"

	maxQueueSizeEnvName = "OTEL_BLRP_MAX_QUEUE_SIZE"
	maxBatchSizeEnvName = "OTEL_BLRP_MAX_EXPORT_BATCH_SIZE"

	defaultLogsQueueSize = 32768
	defaultLogsBatchSize = 2048
)

type logsExporterType string

// Logger holds the OpenTelemetry logs provider, exporter, and log hook.
type Logger struct {
	provider *log.LoggerProvider
	exporter *countingExporter
	hook     *otelLogHook
}

// NewLogger creates the logs collection with batch sizes from env; it returns nil when no logs exporter is set.
func NewLogger(
	ctx context.Context,
	appName, appVersion string,
	writer io.Writer,
	opts *Options,
	env map[string]string,
) (*Logger, error) {
	exporter, err := NewLogsExporter(ctx, writer, opts)
	if err != nil {
		return nil, err
	}

	if exporter == nil {
		return nil, nil
	}

	counted := &countingExporter{Exporter: exporter}

	provider, err := newLogsProvider(ctx, counted, appName, appVersion, env)
	if err != nil {
		return nil, errors.Join(err, exporter.Shutdown(ctx))
	}

	otel.SetLoggerProvider(provider)

	return &Logger{
		provider: provider,
		exporter: counted,
		hook:     newOtelLogHook(appName, appVersion, provider),
	}, nil
}

// NewLogsExporter creates a new logs exporter based on the telemetry options.
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
		return nil, &ErrorUnknownLogsExporter{Value: opts.LogsExporter}
	}
}

// lostRecords returns [ErrorLogRecordsNotExported] when the exporter confirmed fewer records than the hook emitted.
func (logger *Logger) lostRecords() error {
	emitted := logger.hook.emitted.Load()
	exported := logger.exporter.exported.Load()

	if exported >= emitted {
		return nil
	}

	return &ErrorLogRecordsNotExported{Lost: emitted - exported, Emitted: emitted}
}

// countingExporter counts the records that its exporter accepted.
type countingExporter struct {
	log.Exporter
	exported atomic.Uint64
}

// Export implements the [log.Exporter] interface method.
func (e *countingExporter) Export(ctx context.Context, records []log.Record) error {
	if err := e.Exporter.Export(ctx, records); err != nil {
		return err
	}

	e.exported.Add(uint64(len(records)))

	return nil
}

// newLogsProvider creates a new logs provider with the terragrunt resource attributes.
func newLogsProvider(
	ctx context.Context,
	exp log.Exporter,
	appName, appVersion string,
	env map[string]string,
) (*log.LoggerProvider, error) {
	queueSize, err := sizeFromEnv(env, maxQueueSizeEnvName, defaultLogsQueueSize)
	if err != nil {
		return nil, err
	}

	batchSize, err := sizeFromEnv(env, maxBatchSizeEnvName, defaultLogsBatchSize)
	if err != nil {
		return nil, err
	}

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
		log.WithProcessor(log.NewBatchProcessor(
			exp,
			log.WithMaxQueueSize(queueSize),
			log.WithExportMaxBatchSize(batchSize),
		)),
	), nil
}

// sizeFromEnv returns the size that env sets for name, or fallback when env does not set it.
func sizeFromEnv(env map[string]string, name string, fallback int) (int, error) {
	value := env[name]
	if value == "" {
		return fallback, nil
	}

	size, err := strconv.Atoi(value)
	if err != nil || size < 1 {
		return 0, &ErrorInvalidEnvVariable{Name: name, Value: value}
	}

	return size, nil
}
