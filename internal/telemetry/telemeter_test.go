package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errfmt"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
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

			opts := options.NewTerragruntOptions()
			opts.Telemetry.TraceExporter = "console"
			opts.Telemetry.MetricExporter = "console"

			tlm, err := telemetry.NewTelemeter(
				t.Context(),
				nil,
				"terragrunt",
				"v0.0.0-test",
				&buf,
				opts.Telemetry,
				nil,
				false,
			)
			require.NoError(t, err)
			require.NotNil(t, tlm)

			require.NoError(
				t,
				tlm.Trace(
					t.Context(),
					"test_span",
					nil,
					func(context.Context) error { return nil },
				),
			)
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

func TestCollectExportsRootErrorAsLogRecord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		traceExporter string
	}{
		{name: "with a trace exporter", traceExporter: consoleExporter},
		{name: "without a trace exporter", traceExporter: "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			opts := options.NewTerragruntOptions()
			opts.Telemetry.TraceExporter = tt.traceExporter
			opts.Telemetry.LogsExporter = consoleExporter

			l := discardLogger()

			tlm, err := telemetry.NewTelemeter(
				t.Context(), l, "terragrunt", "v0.0.0-test", util.NewSyncWriter(&buf), opts.Telemetry, nil, true,
			)
			require.NoError(t, err)

			runErr := errors.New("unit failed")

			var rootSpan trace.SpanContext

			err = tlm.Collect(t.Context(), l, "command", nil, func(ctx context.Context, l log.Logger) error {
				rootSpan = trace.SpanContextFromContext(ctx)

				return tlm.Collect(ctx, l, "unit", nil, func(context.Context, log.Logger) error {
					return runErr
				})
			})
			require.ErrorIs(t, err, runErr)
			require.NoError(t, tlm.Shutdown(t.Context()))

			records := consoleLogRecords(t, &buf)
			require.Len(t, records, 1)
			assert.Equal(t, errfmt.Format(runErr), records[0].Body.Value)
			assert.Equal(t, "error", records[0].SeverityText)
			assert.Equal(t, rootSpan.TraceID().String(), records[0].TraceID)
			assert.Equal(t, rootSpan.SpanID().String(), records[0].SpanID)
		})
	}
}

func TestCollectKeepsRootSpanAfterCollectWithNoSpan(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := options.NewTerragruntOptions()
	opts.Telemetry.TraceExporter = consoleExporter
	opts.Telemetry.LogsExporter = consoleExporter

	l := discardLogger()

	tlm, err := telemetry.NewTelemeter(
		t.Context(), l, "terragrunt", "v0.0.0-test", util.NewSyncWriter(&buf), opts.Telemetry, nil, true,
	)
	require.NoError(t, err)

	var rootSpan trace.SpanContext

	require.NoError(t, tlm.Collect(t.Context(), l, "command", nil, func(ctx context.Context, _ log.Logger) error {
		rootSpan = trace.SpanContextFromContext(ctx)

		noSpanCtx := trace.ContextWithSpanContext(ctx, trace.SpanContext{})

		err := tlm.Collect(noSpanCtx, l, "detached", nil, func(context.Context, log.Logger) error {
			return nil
		})
		if err != nil {
			return err
		}

		l.Infof("record after detached span")

		return nil
	}))
	require.NoError(t, tlm.Shutdown(t.Context()))

	require.True(t, rootSpan.IsValid())

	records := consoleLogRecords(t, &buf)
	require.Len(t, records, 1)
	assert.Equal(t, rootSpan.TraceID().String(), records[0].TraceID)
	assert.Equal(t, rootSpan.SpanID().String(), records[0].SpanID)
}

// consoleLogRecord is one log record as the console exporter prints it.
type consoleLogRecord struct {
	Body *struct {
		Value string `json:"Value"`
	} `json:"Body"`
	SeverityText string `json:"SeverityText"`
	TraceID      string `json:"TraceID"`
	SpanID       string `json:"SpanID"`
}

// consoleLogRecords returns the log records in console exporter output, without the spans.
func consoleLogRecords(t *testing.T, output *bytes.Buffer) []consoleLogRecord {
	t.Helper()

	var records []consoleLogRecord

	dec := json.NewDecoder(output)

	for dec.More() {
		var record consoleLogRecord
		require.NoError(t, dec.Decode(&record))

		if record.Body != nil {
			records = append(records, record)
		}
	}

	return records
}
