package telemetry

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/bridges/otellogrus"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"
)

// stdLogrusLevels maps Terragrunt levels to the standard logrus levels that
// the otellogrus bridge derives OpenTelemetry severities from. The stderr and
// stdout passthrough levels carry OpenTofu/Terraform's error and regular
// output, so they map to the error and info severities respectively.
var stdLogrusLevels = map[log.Level]logrus.Level{
	log.StderrLevel: logrus.ErrorLevel,
	log.StdoutLevel: logrus.InfoLevel,
	log.ErrorLevel:  logrus.ErrorLevel,
	log.WarnLevel:   logrus.WarnLevel,
	log.InfoLevel:   logrus.InfoLevel,
	log.DebugLevel:  logrus.DebugLevel,
	log.TraceLevel:  logrus.TraceLevel,
}

// NewOtelLogHook returns a logrus hook that exports Terragrunt log entries
// through the given OpenTelemetry logger provider, under an instrumentation
// scope with the given name and version.
//
// Terragrunt registers its levels with logrus shifted past Panic and Fatal
// (see [log.Level.ToLogrusLevel]), so entries reach hooks with distorted
// levels: Terragrunt's error level arrives as [logrus.InfoLevel], and the
// debug and trace levels arrive as values logrus doesn't define at all. Used
// directly, the otellogrus bridge would export errors with info severity and
// never fire for debug or trace records. This hook registers for the shifted
// levels and restores the standard logrus level on each entry before handing
// it to the bridge, and strips ANSI color codes.
func NewOtelLogHook(name, version string, provider otellog.LoggerProvider) logrus.Hook {
	return newOtelLogHook(name, version, provider)
}

func newOtelLogHook(name, version string, provider otellog.LoggerProvider) *otelLogHook {
	return &otelLogHook{
		inner: otellogrus.NewHook(
			name,
			otellogrus.WithVersion(version),
			otellogrus.WithLoggerProvider(provider),
		),
		logger: provider.Logger(name, otellog.WithInstrumentationVersion(version)),
	}
}

type otelLogHook struct {
	// inner is the otellogrus bridge.
	inner *otellogrus.Hook
	// logger emits the records that have no log entry.
	logger otellog.Logger
	// rootSpan is the root command span, set while the root command runs.
	rootSpan atomic.Pointer[trace.SpanContext]
	// emitted counts the records given to the logger provider.
	emitted atomic.Uint64
}

// Levels implements the [logrus.Hook] interface method.
func (hook *otelLogHook) Levels() []logrus.Level {
	return log.AllLevels.ToLogrusLevels()
}

// Fire implements the [logrus.Hook] interface method.
func (hook *otelLogHook) Fire(entry *logrus.Entry) error {
	e := *entry
	e.Level = stdLogrusLevels[log.FromLogrusLevel(entry.Level)]
	e.Message = recordBody(entry.Message)

	rootSpan := hook.rootSpan.Load()
	if rootSpan != nil && rootSpan.IsValid() && !trace.SpanContextFromContext(e.Context).IsValid() {
		e.Context = trace.ContextWithSpanContext(context.Background(), *rootSpan)
	}

	hook.emitted.Add(1)

	return hook.inner.Fire(&e)
}

// emitError exports msg as an error record of the span in ctx.
func (hook *otelLogHook) emitError(ctx context.Context, msg string) {
	var record otellog.Record

	record.SetTimestamp(time.Now())
	record.SetBody(attribute.StringValue(recordBody(msg)))
	record.SetSeverity(otellog.SeverityError)
	record.SetSeverityText(logrus.ErrorLevel.String())

	hook.emitted.Add(1)
	hook.logger.Emit(ctx, record)
}

// bindRootSpan sets the root command span and reports false when one is set.
func (hook *otelLogHook) bindRootSpan(span trace.SpanContext) bool {
	return hook.rootSpan.CompareAndSwap(nil, &span)
}

// unbindRootSpan clears the span set by bindRootSpan.
func (hook *otelLogHook) unbindRootSpan() {
	hook.rootSpan.Store(nil)
}

// recordBody returns msg without ANSI color codes and with invalid UTF-8 replaced.
func recordBody(msg string) string {
	return strings.ToValidUTF8(log.RemoveAllASCISeq(msg), "\uFFFD")
}
