package telemetry

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	noneTraceExporterType     traceExporterType = "none"
	consoleTraceExporterType  traceExporterType = "console"
	otlpHTTPTraceExporterType traceExporterType = "otlpHttp"
	otlpGrpcTraceExporterType traceExporterType = "otlpGrpc"
	httpTraceExporterType     traceExporterType = "http"

	traceParentParts = 4
)

type traceExporterType string

type Tracer struct {
	trace.Tracer
	provider         *sdktrace.TracerProvider
	spanExporter     sdktrace.SpanExporter
	parentTraceID    *trace.TraceID
	parentSpanID     *trace.SpanID
	parentTraceFlags *trace.TraceFlags
}

// NewTracer creates and configures the traces collection.
func NewTracer(ctx context.Context, appName, appVersion string, writer io.Writer, opts *Options) (*Tracer, error) {
	spanExporter, err := NewTraceExporter(ctx, writer, opts)
	if err != nil {
		return nil, errors.New(err)
	}

	if spanExporter == nil { // no exporter
		return nil, nil
	}

	provider, err := newTraceProvider(spanExporter, appName, appVersion)
	if err != nil {
		return nil, errors.New(err)
	}

	otel.SetTracerProvider(provider)

	var (
		parentTraceID    *trace.TraceID
		parentSpanID     *trace.SpanID
		parentTraceFlags *trace.TraceFlags
	)

	if opts.TraceParent != "" {
		// parse trace parent values
		parts := strings.Split(opts.TraceParent, "-")
		if len(parts) != traceParentParts {
			return nil, fmt.Errorf("invalid TRACEPARENT value %s", opts.TraceParent)
		}

		_, traceIDHex, spanIDHex, traceFlagsStr := parts[0], parts[1], parts[2], parts[3]

		parsedFlag, err := strconv.Atoi(traceFlagsStr)
		if err != nil {
			return nil, errors.Errorf("invalid trace flags: %w", err)
		}

		traceFlags := trace.FlagsSampled
		if parsedFlag == 0 {
			traceFlags = 0
		}

		traceID, err := trace.TraceIDFromHex(traceIDHex)
		if err != nil {
			return nil, errors.New(err)
		}

		spanID, err := trace.SpanIDFromHex(spanIDHex)
		if err != nil {
			return nil, errors.New(err)
		}

		parentTraceID = &traceID
		parentSpanID = &spanID
		parentTraceFlags = &traceFlags
	}

	tracer := &Tracer{
		Tracer:           provider.Tracer(appName),
		provider:         provider,
		spanExporter:     spanExporter,
		parentTraceID:    parentTraceID,
		parentSpanID:     parentSpanID,
		parentTraceFlags: parentTraceFlags,
	}

	return tracer, nil
}

// newTraceProvider creates a new trace tracer with terragrunt version.
func newTraceProvider(exp sdktrace.SpanExporter, appName, appVersion string) (*sdktrace.TracerProvider, error) {
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

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(r),
	), nil
}

// NewTraceExporter creates a new exporter based on the telemetry options.
func NewTraceExporter(ctx context.Context, writer io.Writer, opts *Options) (sdktrace.SpanExporter, error) {
	exporterType := traceExporterType(opts.TraceExporter)
	if exporterType == "" {
		exporterType = noneTraceExporterType
	}

	// TODO: Remove lint suppression
	switch exporterType { //nolint:exhaustive
	case httpTraceExporterType:
		if opts.TraceExporterHTTPEndpoint == "" {
			return nil, &ErrorMissingEnvVariable{
				Vars: []string{"TG_TELEMETRY_TRACE_EXPORTER_HTTP_ENDPOINT"},
			}
		}

		endpointOpt := otlptracehttp.WithEndpoint(opts.TraceExporterHTTPEndpoint)
		config := []otlptracehttp.Option{endpointOpt}

		if opts.TraceExporterInsecureEndpoint {
			config = append(config, otlptracehttp.WithInsecure())
		}

		return otlptracehttp.New(ctx, config...)
	case otlpHTTPTraceExporterType:
		var config []otlptracehttp.Option
		if opts.TraceExporterInsecureEndpoint {
			config = append(config, otlptracehttp.WithInsecure())
		}

		return otlptracehttp.New(ctx, config...)
	case otlpGrpcTraceExporterType:
		var config []otlptracegrpc.Option
		if opts.TraceExporterInsecureEndpoint {
			config = append(config, otlptracegrpc.WithInsecure())
		}

		return otlptracegrpc.New(ctx, config...)
	case consoleTraceExporterType:
		return stdouttrace.New(stdouttrace.WithWriter(writer))
	default:
		return nil, nil
	}
}

// Trace collects traces for method execution.
func (tracer *Tracer) Trace(ctx context.Context, name string, attrs map[string]any, fn func(childCtx context.Context) error) error {
	if tracer == nil || tracer.spanExporter == nil || tracer.provider == nil { // invoke function without tracing
		return fn(ctx)
	}

	ctx, span := tracer.openSpan(ctx, name, attrs)
	defer span.End()

	if err := fn(ctx); err != nil {
		// record error in span
		span.RecordError(err)
		return err
	}

	return nil
}

// openSpan creates a new span with attributes.
func (tracer *Tracer) openSpan(ctx context.Context, name string, attrs map[string]any) (context.Context, trace.Span) {
	if tracer.provider == nil {
		return ctx, nil
	}

	if tracer.parentTraceID != nil && tracer.parentSpanID != nil {
		spanContext := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    *tracer.parentTraceID,
			SpanID:     *tracer.parentSpanID,
			Remote:     true,
			TraceFlags: *tracer.parentTraceFlags,
		})

		// create a new context with the parent span context
		ctx = trace.ContextWithSpanContext(ctx, spanContext)
	}

	// This lint is suppressed because we definitely do close the span
	// in a defer statement everywhere openSpan is called. It seems like
	// a useful lint, though. We should consider removing the suppression
	// and fixing the lint.

	ctx, span := tracer.Start(ctx, name) // nolint:spancheck
	// convert attrs map to span.SetAttributes
	span.SetAttributes(mapToAttributes(attrs)...)

	return ctx, span //nolint:spancheck
}
