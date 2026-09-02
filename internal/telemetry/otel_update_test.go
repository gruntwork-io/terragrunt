package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
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

func TestOptionsRootSpanKind(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		opts     *telemetry.Options
		name     string
		expected trace.SpanKind
	}{
		{name: "nil receiver", opts: nil, expected: trace.SpanKindInternal},
		{name: "unset", opts: &telemetry.Options{}, expected: trace.SpanKindInternal},
		{
			name:     "server",
			opts:     &telemetry.Options{TraceRootSpanKind: "server"},
			expected: trace.SpanKindServer,
		},
		{
			name:     "case and space insensitive",
			opts:     &telemetry.Options{TraceRootSpanKind: "  SERVER "},
			expected: trace.SpanKindServer,
		},
		{
			name:     "consumer",
			opts:     &telemetry.Options{TraceRootSpanKind: "consumer"},
			expected: trace.SpanKindConsumer,
		},
		{
			name:     "unrecognized value degrades to internal",
			opts:     &telemetry.Options{TraceRootSpanKind: "garbage"},
			expected: trace.SpanKindInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, tc.opts.RootSpanKind())
		})
	}
}

func TestCollectHonorsSpanKind(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.Telemetry.TraceExporter = consoleExporter

	tlm, err := telemetry.NewTelemeter(
		t.Context(), nil, "terragrunt", "v0.0.0-test", &buf, opts.Telemetry, false,
	)
	require.NoError(t, err)
	require.NotNil(t, tlm)

	require.NoError(t, tlm.Collect(
		t.Context(), nil, "entry_span", nil,
		func(_ context.Context, _ log.Logger) error { return nil },
		trace.WithSpanKind(trace.SpanKindServer),
	))
	require.NoError(t, tlm.Shutdown(t.Context()))

	// SpanKindServer serializes as 2 in the console exporter's JSON.
	assert.Contains(t, buf.String(), `"SpanKind":2`)
}

func TestCollectSetsErrorStatusOnFailure(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.Telemetry.TraceExporter = consoleExporter

	tlm, err := telemetry.NewTelemeter(
		t.Context(), nil, "terragrunt", "v0.0.0-test", &buf, opts.Telemetry, false,
	)
	require.NoError(t, err)

	errSentinel := errors.New("intentional failure")

	gotErr := tlm.Collect(
		t.Context(), nil, "failing_span", nil,
		func(_ context.Context, _ log.Logger) error { return errSentinel },
	)
	require.ErrorIs(t, gotErr, errSentinel)
	require.NoError(t, tlm.Shutdown(t.Context()))

	// Backends derive an errored transaction from the span status, not from the
	// recorded exception event, so both must be present.
	assert.Contains(t, buf.String(), `"Code":"Error"`)
	assert.Contains(t, buf.String(), errSentinel.Error())
}
