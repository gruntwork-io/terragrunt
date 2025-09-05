package telemetry_test

import (
	"io"
	"testing"

	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
)

func TestNewMetricsExporter(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	stdout, err := stdoutmetric.New()
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
			expectedType: (*otlpmetrichttp.Exporter)(nil),
		},
		{
			name:         "gRPC HTTP Exporter",
			exporterType: "grpcHttp",
			insecure:     true,
			expectedType: (*otlpmetricgrpc.Exporter)(nil),
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptionsWithWriters(io.Discard, io.Discard)
			opts.Telemetry.MetricExporter = tt.exporterType
			opts.Telemetry.MetricExporterInsecureEndpoint = tt.insecure

			exporter, err := telemetry.NewMetricsExporter(ctx, io.Discard, opts.Telemetry)
			require.NoError(t, err)

			if tt.expectNil {
				assert.Nil(t, exporter)
			} else {
				assert.IsType(t, tt.expectedType, exporter)
			}
		})
	}
}

func TestCleanMetricName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Normal case",
			input:    "metricName_1.2-34",
			expected: "metricName_1.2_34",
		},
		{
			name:     "Starts with invalid characters",
			input:    "!@#metricName",
			expected: "metricName",
		},
		{
			name:     "Ends with invalid characters",
			input:    "metricName!@#",
			expected: "metricName",
		},
		{
			name:     "Only invalid characters",
			input:    "!@#$%^&*()",
			expected: "",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Leading underscore from replacement",
			input:    "!metricName",
			expected: "metricName",
		},
		{
			name:     "Multiple replacements",
			input:    "metric!@#Name",
			expected: "metric_Name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := telemetry.CleanMetricName(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
