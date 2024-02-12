package telemetry

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
)

func TestNewMetricsExporter(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		exporterType string
		insecure     bool
		expectedType interface{}
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
			opts := &TelemetryOptions{
				Vars: map[string]string{
					"TERRAGRUNT_TELEMETRY_METRIC_EXPORTER":                   tt.exporterType,
					"TERRAGRUNT_TELEMERTY_METRIC_EXPORTER_INSECURE_ENDPOINT": fmt.Sprintf("%v", tt.insecure),
				},
				Writer: io.Discard,
			}

			exporter, err := newMetricsExporter(ctx, opts)
			assert.NoError(t, err)

			if tt.expectNil {
				assert.Nil(t, exporter)
			} else {
				assert.IsType(t, tt.expectedType, exporter)
			}
		})
	}
}

func TestCleanMetricName(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Normal case",
			input:    "metricName_1.2-3/4",
			expected: "metricName_1.2-3/4",
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
			result := cleanMetricName(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
