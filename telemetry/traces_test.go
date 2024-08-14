package telemetry_test

import (
	"context"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
)

func TestNewTraceExporter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	http, err := otlptracehttp.New(ctx)
	require.NoError(t, err)

	grpc, err := otlptracegrpc.New(ctx)
	require.NoError(t, err)

	stdoutrace, err := stdouttrace.New()
	require.NoError(t, err)

	tests := []struct {
		name             string
		telemetryOptions *telemetry.TelemetryOptions
		expectedType     interface{}
		expectError      bool
	}{
		{
			name: "HTTP Trace Exporter",
			telemetryOptions: &telemetry.TelemetryOptions{
				Vars: map[string]string{
					"TERRAGRUNT_TELEMETRY_TRACE_EXPORTER": "otlpHttp",
				},
				Writer: io.Discard,
			},
			expectedType: http,
			expectError:  false,
		},
		{
			name: "Custom HTTP endpoint",
			telemetryOptions: &telemetry.TelemetryOptions{
				Vars: map[string]string{
					"TERRAGRUNT_TELEMETRY_TRACE_EXPORTER":               "http",
					"TERRAGRUNT_TELEMETRY_TRACE_EXPORTER_HTTP_ENDPOINT": "http://localhost:4317",
				},
				Writer: io.Discard,
			},
			expectedType: http,
			expectError:  false,
		},
		{
			name: "Custom HTTP endpoint without endpoint",
			telemetryOptions: &telemetry.TelemetryOptions{
				Vars: map[string]string{
					"TERRAGRUNT_TELEMETRY_TRACE_EXPORTER": "http",
				},
				Writer: io.Discard,
			},
			expectedType: http,
			expectError:  true,
		},
		{
			name: "Grpc Trace Exporter",
			telemetryOptions: &telemetry.TelemetryOptions{
				Vars: map[string]string{
					"TERRAGRUNT_TELEMETRY_TRACE_EXPORTER": "otlpGrpc",
				},
				Writer: io.Discard,
			},
			expectedType: grpc,
			expectError:  false,
		},
		{
			name: "Console Trace Exporter",
			telemetryOptions: &telemetry.TelemetryOptions{
				Vars: map[string]string{
					"TERRAGRUNT_TELEMETRY_TRACE_EXPORTER": "console",
				},
				Writer: io.Discard,
			},
			expectedType: stdoutrace,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			exporter, err := telemetry.NewTraceExporter(ctx, tt.telemetryOptions)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.IsType(t, tt.expectedType, exporter)
			}
		})
	}
}
