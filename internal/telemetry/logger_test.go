package telemetry_test

import (
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
)

func TestNewLogsExporter(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	stdout, err := stdoutlog.New()
	require.NoError(t, err)

	tests := []struct {
		expectedType any
		name         string
		exporterType string
		insecure     bool
		expectNil    bool
	}{
		{
			name:         "OTLP HTTP Exporter",
			exporterType: "otlpHttp",
			insecure:     false,
			expectedType: (*otlploghttp.Exporter)(nil),
		},
		{
			name:         "OTLP gRPC Exporter",
			exporterType: "otlpGrpc",
			insecure:     true,
			expectedType: (*otlploggrpc.Exporter)(nil),
		},
		{
			name:         "Console Exporter",
			exporterType: "console",
			insecure:     false,
			expectedType: stdout,
		},
		{
			name:         "None Exporter",
			exporterType: "none",
			insecure:     false,
			expectNil:    true,
		},
		{
			name:         "Empty Exporter",
			exporterType: "",
			insecure:     false,
			expectNil:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptionsWithWriters(io.Discard, io.Discard)
			opts.Telemetry.LogsExporter = tt.exporterType
			opts.Telemetry.LogsExporterInsecureEndpoint = tt.insecure

			exporter, err := telemetry.NewLogsExporter(ctx, io.Discard, opts.Telemetry)
			require.NoError(t, err)

			if tt.expectNil {
				assert.Nil(t, exporter)
				return
			}

			assert.IsType(t, tt.expectedType, exporter)
		})
	}
}

func TestNewLogger(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	opts := options.NewTerragruntOptionsWithWriters(io.Discard, io.Discard)
	opts.Telemetry.LogsExporter = "console"

	logger, err := telemetry.NewLogger(ctx, "terragrunt", "test", io.Discard, opts.Telemetry)
	require.NoError(t, err)
	require.NotNil(t, logger)

	opts.Telemetry.LogsExporter = "none"

	logger, err = telemetry.NewLogger(ctx, "terragrunt", "test", io.Discard, opts.Telemetry)
	require.NoError(t, err)
	assert.Nil(t, logger)
}
