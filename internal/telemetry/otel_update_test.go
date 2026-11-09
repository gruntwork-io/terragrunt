package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

const consoleExporter = "console"

func TestCollectEmitsSpanAndMetric(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.Telemetry.TraceExporter = consoleExporter
	opts.Telemetry.MetricExporter = consoleExporter

	tlm, err := telemetry.NewTelemeter(
		t.Context(), nil, "terragrunt", "v0.0.0-test", &buf, opts.Telemetry, false,
	)
	require.NoError(t, err)
	require.NotNil(t, tlm)

	called := false

	require.NoError(t, tlm.Collect(
		t.Context(), nil, "test_operation", map[string]any{"key": "value"},
		func(_ context.Context, _ log.Logger) error {
			called = true
			return nil
		},
	))

	assert.True(t, called, "wrapped function must be invoked")
	require.NoError(t, tlm.Shutdown(t.Context()))
	assert.Positive(t, buf.Len(), "expected telemetry output")
}

func TestCollectPropagatesError(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.Telemetry.TraceExporter = consoleExporter
	opts.Telemetry.MetricExporter = consoleExporter

	tlm, err := telemetry.NewTelemeter(
		t.Context(), nil, "terragrunt", "v0.0.0-test", io.Discard, opts.Telemetry, false,
	)
	require.NoError(t, err)

	errSentinel := errors.New("intentional failure")

	gotErr := tlm.Collect(
		t.Context(), nil, "failing_op", nil,
		func(_ context.Context, _ log.Logger) error { return errSentinel },
	)
	require.ErrorIs(t, gotErr, errSentinel)
	require.NoError(t, tlm.Shutdown(t.Context()))
}

func TestCollectWithLoggerBindsContext(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.Telemetry.TraceExporter = consoleExporter

	l := logger.CreateLogger()

	tlm, err := telemetry.NewTelemeter(
		t.Context(), l, "terragrunt", "v0.0.0-test", io.Discard, opts.Telemetry, false,
	)
	require.NoError(t, err)

	type result struct{ spanValid bool }

	ch := make(chan result, 1)

	require.NoError(t, tlm.Collect(
		t.Context(), l, "ctx_test", nil,
		telemetry.WithoutLogger(func(ctx context.Context) error {
			span := trace.SpanFromContext(ctx)
			ch <- result{spanValid: span.SpanContext().IsValid()}

			return nil
		}),
	))

	// The child context carries a valid span from the tracer.
	got := <-ch
	assert.True(t, got.spanValid, "child context must carry a valid span")
	require.NoError(t, tlm.Shutdown(t.Context()))
}

func TestShutdownIdempotent(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.Telemetry.TraceExporter = consoleExporter
	opts.Telemetry.MetricExporter = consoleExporter

	tlm, err := telemetry.NewTelemeter(
		t.Context(), nil, "terragrunt", "v0.0.0-test", io.Discard, opts.Telemetry, false,
	)
	require.NoError(t, err)

	require.NoError(t, tlm.Shutdown(t.Context()))
	require.NoError(t, tlm.Shutdown(t.Context()), "second shutdown must not error")
}

func TestShutdownNilTelemeter(t *testing.T) {
	t.Parallel()

	var tlm *telemetry.Telemeter
	require.NoError(t, tlm.Shutdown(t.Context()))
}

func TestTelemeterFromContextNoOp(t *testing.T) {
	t.Parallel()

	// TelemeterFromContext returns a zero-value Telemeter when none is attached.
	tlm := telemetry.TelemeterFromContext(t.Context())
	require.NotNil(t, tlm)

	called := false

	require.NoError(t, tlm.Collect(
		t.Context(), nil, "noop", nil,
		func(_ context.Context, _ log.Logger) error {
			called = true
			return nil
		},
	))

	assert.True(t, called)
}

func TestContextWithTelemeterRoundtrip(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.Telemetry.TraceExporter = consoleExporter

	tlm, err := telemetry.NewTelemeter(
		t.Context(), nil, "terragrunt", "v0.0.0-test", io.Discard, opts.Telemetry, false,
	)
	require.NoError(t, err)

	ctx := telemetry.ContextWithTelemeter(t.Context(), tlm)
	got := telemetry.TelemeterFromContext(ctx)
	assert.Equal(t, tlm, got)
	require.NoError(t, tlm.Shutdown(t.Context()))
}

func TestTelemeterConsoleOutputValidJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.Telemetry.TraceExporter = consoleExporter
	opts.Telemetry.MetricExporter = consoleExporter

	tlm, err := telemetry.NewTelemeter(
		t.Context(), nil, "terragrunt", "v0.0.0-test", &buf, opts.Telemetry, false,
	)
	require.NoError(t, err)

	require.NoError(t, tlm.Collect(
		t.Context(), nil, "json_test", map[string]any{"num": 42, "flag": true},
		func(_ context.Context, _ log.Logger) error { return nil },
	))

	require.NoError(t, tlm.Shutdown(t.Context()))

	dec := json.NewDecoder(&buf)
	entries := 0

	for dec.More() {
		var raw json.RawMessage
		require.NoError(t, dec.Decode(&raw), "console exporter must emit valid JSON")

		entries++
	}

	assert.Positive(t, entries, "expected at least one JSON entry")
}

func TestNewTelemeterNoExporters(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())

	tlm, err := telemetry.NewTelemeter(
		t.Context(), nil, "terragrunt", "v0.0.0-test", io.Discard, opts.Telemetry, false,
	)
	require.NoError(t, err)
	require.NotNil(t, tlm)

	called := false

	require.NoError(t, tlm.Collect(
		t.Context(), nil, "noop_op", nil,
		func(_ context.Context, _ log.Logger) error {
			called = true
			return nil
		},
	))

	assert.True(t, called)
	require.NoError(t, tlm.Shutdown(t.Context()))
}

func TestNewTelemeterSetsScopeVersionOnAllSignals(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.Telemetry.TraceExporter = consoleExporter
	opts.Telemetry.MetricExporter = consoleExporter
	opts.Telemetry.LogsExporter = consoleExporter

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	tlm, err := telemetry.NewTelemeter(
		t.Context(), l, "terragrunt", "v0.0.0-test", util.NewSyncWriter(&buf), opts.Telemetry, true,
	)
	require.NoError(t, err)

	require.NoError(t, tlm.Collect(
		t.Context(), l, "scope_test", nil,
		func(_ context.Context, childL log.Logger) error {
			childL.Infof("scope test record")
			return nil
		},
	))

	require.NoError(t, tlm.Shutdown(t.Context()))

	type scope struct {
		Name    string `json:"Name"`
		Version string `json:"Version"`
	}

	var spanScopes, metricScopes, logScopes []scope

	dec := json.NewDecoder(&buf)

	for dec.More() {
		var entry struct {
			InstrumentationScope *scope `json:"InstrumentationScope"`
			Scope                *scope `json:"Scope"`
			ScopeMetrics         []struct {
				Scope scope `json:"Scope"`
			} `json:"ScopeMetrics"`
		}
		require.NoError(t, dec.Decode(&entry))

		switch {
		case entry.InstrumentationScope != nil:
			spanScopes = append(spanScopes, *entry.InstrumentationScope)
		case entry.Scope != nil:
			logScopes = append(logScopes, *entry.Scope)
		}

		for _, sm := range entry.ScopeMetrics {
			metricScopes = append(metricScopes, sm.Scope)
		}
	}

	want := scope{Name: "terragrunt", Version: "v0.0.0-test"}

	require.NotEmpty(t, spanScopes, "expected a span")
	require.NotEmpty(t, metricScopes, "expected a metric")
	require.NotEmpty(t, logScopes, "expected a log record")

	for _, got := range spanScopes {
		assert.Equal(t, want, got, "span scope")
	}

	for _, got := range metricScopes {
		assert.Equal(t, want, got, "metric scope")
	}

	for _, got := range logScopes {
		assert.Equal(t, want, got, "log scope")
	}
}

func TestCollectRecordsStringSliceAsSliceAttribute(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.Telemetry.TraceExporter = consoleExporter

	tlm, err := telemetry.NewTelemeter(
		t.Context(), nil, "terragrunt", "v0.0.0-test", &buf, opts.Telemetry, false,
	)
	require.NoError(t, err)

	require.NoError(t, tlm.Collect(
		t.Context(), nil, "slice_test", map[string]any{"args": []string{"plan", "-input=false"}},
		telemetry.WithoutLogger(func(context.Context) error { return nil }),
	))

	require.NoError(t, tlm.Shutdown(t.Context()))

	var span struct {
		Attributes []struct {
			Value struct {
				Value any    `json:"Value"`
				Type  string `json:"Type"`
			} `json:"Value"`
			Key string `json:"Key"`
		} `json:"Attributes"`
	}
	require.NoError(t, json.NewDecoder(&buf).Decode(&span))
	require.Len(t, span.Attributes, 1)

	attr := span.Attributes[0]
	assert.Equal(t, "args", attr.Key)
	assert.Equal(t, "STRINGSLICE", attr.Value.Type)
	assert.Equal(t, []any{"plan", "-input=false"}, attr.Value.Value)
}
