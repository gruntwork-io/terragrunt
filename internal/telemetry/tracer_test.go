package telemetry_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestNewTraceExporter(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	http, err := otlptracehttp.New(ctx)
	require.NoError(t, err)

	grpc, err := otlptracegrpc.New(ctx)
	require.NoError(t, err)

	stdoutrace, err := stdouttrace.New()
	require.NoError(t, err)

	tests := []struct {
		expectedType              any
		traceExporter             string
		traceExporterHTTPEndpoint string
		name                      string
		expectError               bool
	}{
		{
			name:          "HTTP Trace Exporter",
			traceExporter: "otlpHttp",
			expectedType:  http,
			expectError:   false,
		},
		{
			name:                      "Custom HTTP endpoint",
			traceExporter:             "http",
			traceExporterHTTPEndpoint: "http://localhost:4317",
			expectedType:              http,
			expectError:               false,
		},
		{
			name:          "Custom HTTP endpoint without endpoint",
			traceExporter: "http",
			expectedType:  http,
			expectError:   true,
		},
		{
			name:          "Grpc Trace Exporter",
			traceExporter: "otlpGrpc",
			expectedType:  grpc,
			expectError:   false,
		},
		{
			name:          "Console Trace Exporter",
			traceExporter: "console",
			expectedType:  stdoutrace,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.Telemetry.TraceExporter = tt.traceExporter
			opts.Telemetry.TraceExporterHTTPEndpoint = tt.traceExporterHTTPEndpoint

			exporter, err := telemetry.NewTraceExporter(ctx, io.Discard, opts.Telemetry)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.IsType(t, tt.expectedType, exporter)
			}
		})
	}
}

func TestNewTracerExtractsTraceParent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		traceParent   string
		expectError   bool
		expectSampled bool
	}{
		{
			name:          "sampled",
			traceParent:   "00-" + remoteTraceID + "-" + remoteSpanID + "-01",
			expectSampled: true,
		},
		{
			name:        "unsampled",
			traceParent: "00-" + remoteTraceID + "-" + remoteSpanID + "-00",
		},
		{
			// Only bit 0 of trace-flags is the sampled flag. Bit 1 is the random
			// trace ID flag and says nothing about sampling.
			name:        "random flag without the sampled flag",
			traceParent: "00-" + remoteTraceID + "-" + remoteSpanID + "-02",
		},
		{
			name:          "random and sampled flags",
			traceParent:   "00-" + remoteTraceID + "-" + remoteSpanID + "-03",
			expectSampled: true,
		},
		{
			// Version 00 reserves the remaining trace-flags bits.
			name:        "reserved trace-flags bits",
			traceParent: "00-" + remoteTraceID + "-" + remoteSpanID + "-ff",
			expectError: true,
		},
		{
			name:        "not a traceparent",
			traceParent: "garbage",
			expectError: true,
		},
		{
			name:        "missing trace-flags field",
			traceParent: "00-" + remoteTraceID + "-" + remoteSpanID,
			expectError: true,
		},
		{
			name:        "all-zero trace ID and parent ID",
			traceParent: "00-" + strings.Repeat("0", 32) + "-" + strings.Repeat("0", 16) + "-01",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.Telemetry.TraceExporter = "console"
			opts.Telemetry.TraceParent = tt.traceParent

			tracer, err := telemetry.NewTracer(
				t.Context(), nil, "terragrunt", "v0.0.0-test", io.Discard, opts.Telemetry,
			)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, tracer)

			// The remote parent is grafted onto the first span started on a context
			// that carries none.
			var got trace.SpanContext

			require.NoError(t, tracer.Trace(
				t.Context(), "root", nil,
				func(childCtx context.Context) error {
					got = trace.SpanContextFromContext(childCtx)
					return nil
				},
			))

			assert.Equal(t, remoteTraceID, got.TraceID().String())
			assert.Equal(t, tt.expectSampled, got.TraceFlags().IsSampled())
		})
	}
}

func TestNewTracerRegistersGlobalPropagator(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.Telemetry.TraceExporter = "console"

	tracer, err := telemetry.NewTracer(
		t.Context(), nil, "terragrunt", "v0.0.0-test", io.Discard, opts.Telemetry,
	)
	require.NoError(t, err)
	require.NotNil(t, tracer)

	// The global default propagator is a no-op, which leaves instrumented clients
	// injecting no traceparent at all.
	ctx, span := tracer.Start(t.Context(), "outbound")
	t.Cleanup(func() { span.End() })

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	assert.Contains(
		t, carrier.Get("traceparent"), span.SpanContext().TraceID().String(),
	)
}
