package telemetry_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

const (
	remoteTraceID     = "b2ff1f9c0d4a4e8fa9a1c3e5d7b9f1a3"
	remoteSpanID      = "0e6f631d793c718a"
	remoteTraceParent = "00-" + remoteTraceID + "-" + remoteSpanID + "-01"

	currentTraceID     = "aa11bb22cc33dd44ee55ff6677889900"
	currentSpanID      = "43aad9a8f32c6dea"
	currentTraceParent = "00-" + currentTraceID + "-" + currentSpanID + "-01"
)

// ctxWithSpanContext returns a context carrying a valid span context with the
// given trace ID, span ID and trace flags.
func ctxWithSpanContext(
	t *testing.T, traceIDHex, spanIDHex string, traceFlags trace.TraceFlags,
) context.Context {
	t.Helper()

	traceID, err := trace.TraceIDFromHex(traceIDHex)
	require.NoError(t, err)

	spanID, err := trace.SpanIDFromHex(spanIDHex)
	require.NoError(t, err)

	return trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(
		trace.SpanContextConfig{
			TraceID:    traceID,
			SpanID:     spanID,
			TraceFlags: traceFlags,
		},
	))
}

func TestTraceParentFromContextPrefersCurrentSpan(t *testing.T) {
	t.Parallel()

	// The remote parent belongs to the root span alone. Handing it to a child
	// process would make every span in the run a sibling of the root.
	got := telemetry.TraceParentFromContext(
		ctxWithSpanContext(t, currentTraceID, currentSpanID, trace.FlagsSampled),
		&telemetry.Options{TraceParent: remoteTraceParent},
	)
	assert.Equal(t, currentTraceParent, got)
}

func TestTraceParentFromContextWithoutOptions(t *testing.T) {
	t.Parallel()

	got := telemetry.TraceParentFromContext(
		ctxWithSpanContext(t, currentTraceID, currentSpanID, trace.FlagsSampled), nil,
	)
	assert.Equal(t, currentTraceParent, got)
}

func TestTraceParentFromContextFallsBackToRemoteParent(t *testing.T) {
	t.Parallel()

	got := telemetry.TraceParentFromContext(
		t.Context(), &telemetry.Options{TraceParent: remoteTraceParent},
	)
	assert.Equal(t, remoteTraceParent, got)
}

func TestTraceParentFromContextEmpty(t *testing.T) {
	t.Parallel()

	assert.Empty(t, telemetry.TraceParentFromContext(t.Context(), nil))
	assert.Empty(t, telemetry.TraceParentFromContext(t.Context(), &telemetry.Options{}))
}

func TestTraceParentFromContextUnsampled(t *testing.T) {
	t.Parallel()

	got := telemetry.TraceParentFromContext(
		ctxWithSpanContext(t, currentTraceID, currentSpanID, 0), nil,
	)
	assert.Equal(t, "00-"+currentTraceID+"-"+currentSpanID+"-00", got)
}
