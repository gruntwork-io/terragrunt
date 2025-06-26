package telemetry_test

import (
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
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

			opts := options.NewTerragruntOptionsWithWriters(io.Discard, io.Discard)
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
