package telemetry

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"

	"github.com/gruntwork-io/go-commons/env"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"

	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

type traceExporterType string

const (
	noneTraceExporterType     traceExporterType = "none"
	consoleTraceExporterType  traceExporterType = "console"
	otlpHTTPTraceExporterType traceExporterType = "otlpHttp"
	otlpGrpcTraceExporterType traceExporterType = "otlpGrpc"
	httpTraceExporterType     traceExporterType = "http"

	traceParentParts = 4
)

// Trace - collect traces for method execution
func Trace(ctx context.Context, name string, attrs map[string]interface{}, fn func(childCtx context.Context) error) error {
	if spanExporter == nil || traceProvider == nil { // invoke function without tracing
		return fn(ctx)
	}

	ctx, span := openSpan(ctx, name, attrs)
	defer span.End()

	if err := fn(ctx); err != nil {
		// record error in span
		span.RecordError(err)
		return err
	}

	return nil
}

// configureTraceCollection - configure the traces collection
func configureTraceCollection(ctx context.Context, opts *TelemetryOptions) error {
	exp, err := NewTraceExporter(ctx, opts)
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

	traceParent := env.GetString(opts.Vars["TRACEPARENT"], "")

	if traceParent != "" {
		// parse trace parent values
		parts := strings.Split(traceParent, "-")
		if len(parts) != traceParentParts {
			return fmt.Errorf("invalid TRACEPARENT value %s", traceParent)
		}

		_, traceIDHex, spanIDHex, traceFlagsStr := parts[0], parts[1], parts[2], parts[3]

		parsedFlag, err := strconv.Atoi(traceFlagsStr)
		if err != nil {
			return fmt.Errorf("invalid trace flags: %w", err)
		}

		traceFlags := trace.FlagsSampled
		if parsedFlag == 0 {
			traceFlags = 0
		}

		traceID, err := trace.TraceIDFromHex(traceIDHex)
		if err != nil {
			return errors.WithStack(err)
		}

		spanID, err := trace.SpanIDFromHex(spanIDHex)
		if err != nil {
			return errors.WithStack(err)
		}

		parentTraceID = &traceID
		parentSpanID = &spanID
		parentTraceFlags = &traceFlags
	}

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

// NewTraceExporter - create a new exporter based on the telemetry options.
func NewTraceExporter(ctx context.Context, opts *TelemetryOptions) (sdktrace.SpanExporter, error) {
	exporterType := traceExporterType(env.GetString(opts.Vars["TERRAGRUNT_TELEMETRY_TRACE_EXPORTER"], string(noneTraceExporterType)))
	insecure := env.GetBool(opts.GetValue("TERRAGRUNT_TELEMETRY_TRACE_EXPORTER_INSECURE_ENDPOINT", "TERRAGRUNT_TELEMERTY_TRACE_EXPORTER_INSECURE_ENDPOINT"), false)

	// TODO: Remove lint suppression
	switch exporterType { //nolint:exhaustive
	case httpTraceExporterType:
		endpoint := env.GetString(opts.GetValue("TERRAGRUNT_TELEMETRY_TRACE_EXPORTER_HTTP_ENDPOINT", "TERRAGRUNT_TELEMERTY_TRACE_EXPORTER_HTTP_ENDPOINT"), "")
		if endpoint == "" {
			return nil, &ErrorMissingEnvVariable{
				Vars: []string{"TERRAGRUNT_TELEMETRY_TRACE_EXPORTER_HTTP_ENDPOINT"},
			}
		}

		endpointOpt := otlptracehttp.WithEndpoint(endpoint)
		config := []otlptracehttp.Option{endpointOpt}

		if insecure {
			config = append(config, otlptracehttp.WithInsecure())
		}

		return otlptracehttp.New(ctx, config...)
	case otlpHTTPTraceExporterType:
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
	default:
		return nil, nil
	}
}

// openSpan - create a new span with attributes.
func openSpan(ctx context.Context, name string, attrs map[string]interface{}) (context.Context, trace.Span) {
	if traceProvider == nil {
		return ctx, nil
	}

	if parentTraceID != nil && parentSpanID != nil {
		spanContext := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    *parentTraceID,
			SpanID:     *parentSpanID,
			Remote:     true,
			TraceFlags: *parentTraceFlags,
		})

		// create a new context with the parent span context
		ctx = trace.ContextWithSpanContext(ctx, spanContext)
	}

	// This lint is suppressed because we definitely do close the span
	// in a defer statement everywhere openSpan is called. It seems like
	// a useful lint, though. We should consider removing the suppression
	// and fixing the lint.

	ctx, span := rootTracer.Start(ctx, name) // nolint:spancheck
	// convert attrs map to span.SetAttributes
	span.SetAttributes(mapToAttributes(attrs)...)

	return ctx, span //nolint:spancheck
}
