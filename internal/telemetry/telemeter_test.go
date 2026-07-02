package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTelemeterEnvOverrides(t *testing.T) {
	cases := []struct {
		name             string
		envServiceName   string
		envResourceAttrs string
		wantServiceName  string
	}{
		{
			name:            "default",
			wantServiceName: "terragrunt",
		},
		{
			name:            "OTEL_SERVICE_NAME overrides default",
			envServiceName:  "Service-Name-From-Env",
			wantServiceName: "Service-Name-From-Env",
		},
		{
			name:             "OTEL_RESOURCE_ATTRIBUTES service.name overrides default",
			envResourceAttrs: "service.name=Service-Name-From-Resource-Attributes",
			wantServiceName:  "Service-Name-From-Resource-Attributes",
		},
		{
			name:             "OTEL_SERVICE_NAME overrides OTEL_RESOURCE_ATTRIBUTES service.name",
			envServiceName:   "Service-Name-From-Env",
			envResourceAttrs: "service.name=Service-Name-From-Resource-Attributes",
			wantServiceName:  "Service-Name-From-Env",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OTEL_SERVICE_NAME", tc.envServiceName)
			t.Setenv("OTEL_RESOURCE_ATTRIBUTES", tc.envResourceAttrs)

			var buf bytes.Buffer

			opts := options.NewTerragruntOptionsWithWriters(io.Discard, io.Discard)
			opts.Telemetry.TraceExporter = "console"
			opts.Telemetry.MetricExporter = "console"

			tlm, err := telemetry.NewTelemeter(t.Context(), nil, "terragrunt", "v0.0.0-test", &buf, opts.Telemetry, false)
			require.NoError(t, err)
			require.NotNil(t, tlm)

			require.NoError(t, tlm.Trace(t.Context(), "test_span", nil, func(context.Context) error { return nil }))
			tlm.Count(t.Context(), "test_metric", 1)
			require.NoError(t, tlm.Shutdown(t.Context()))

			type entry struct {
				Resource []struct {
					Key   string `json:"Key"`
					Value struct {
						Value string `json:"Value"`
					} `json:"Value"`
				} `json:"Resource"`
				ScopeMetrics []json.RawMessage `json:"ScopeMetrics,omitempty"`
			}

			var spanFound, metricFound bool

			dec := json.NewDecoder(&buf)
			for dec.More() {
				var e entry
				require.NoError(t, dec.Decode(&e))

				got := map[string]string{}
				for _, kv := range e.Resource {
					got[kv.Key] = kv.Value.Value
				}

				assert.Equal(t, tc.wantServiceName, got["service.name"])

				if len(e.ScopeMetrics) > 0 {
					metricFound = true
				} else {
					spanFound = true
				}
			}

			assert.True(t, spanFound, "expected span emission")
			assert.True(t, metricFound, "expected metric emission")
		})
	}
}
