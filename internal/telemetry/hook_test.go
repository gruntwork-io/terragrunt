package telemetry_test

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// recordingExporter captures exported records so tests can assert on them.
type recordingExporter struct {
	records []sdklog.Record
	mu      sync.Mutex
}

func (e *recordingExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.records = append(e.records, records...)

	return nil
}

func (e *recordingExporter) Shutdown(context.Context) error { return nil }

func (e *recordingExporter) ForceFlush(context.Context) error { return nil }

func (e *recordingExporter) Records() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()

	return append([]sdklog.Record(nil), e.records...)
}

func TestOtelLogHookSeverities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		level            log.Level
		expectedSeverity otellog.Severity
	}{
		{name: "stderr", level: log.StderrLevel, expectedSeverity: otellog.SeverityError},
		{name: "stdout", level: log.StdoutLevel, expectedSeverity: otellog.SeverityInfo},
		{name: "error", level: log.ErrorLevel, expectedSeverity: otellog.SeverityError},
		{name: "warn", level: log.WarnLevel, expectedSeverity: otellog.SeverityWarn},
		{name: "info", level: log.InfoLevel, expectedSeverity: otellog.SeverityInfo},
		{name: "debug", level: log.DebugLevel, expectedSeverity: otellog.SeverityDebug},
		{name: "trace", level: log.TraceLevel, expectedSeverity: otellog.SeverityTrace},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			exporter := &recordingExporter{}
			provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)))

			t.Cleanup(func() {
				require.NoError(t, provider.Shutdown(context.Background()))
			})

			l := logger.CreateLogger()
			l.SetOptions(
				log.WithLevel(log.TraceLevel),
				log.WithOutput(io.Discard),
				log.WithHooks(telemetry.NewOtelLogHook("terragrunt", provider)),
			)

			l.Logf(tt.level, "message at %s", tt.level)

			records := exporter.Records()
			require.Len(t, records, 1)
			assert.Equal(t, tt.expectedSeverity, records[0].Severity())
			assert.Equal(t, "message at "+tt.level.String(), records[0].Body().AsString())
		})
	}
}
