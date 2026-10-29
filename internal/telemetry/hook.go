package telemetry

import (
	"context"
	"sync/atomic"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/bridges/otellogrus"
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
// through the given OpenTelemetry logger provider.
//
// Terragrunt registers its levels with logrus shifted past Panic and Fatal
// (see [log.Level.ToLogrusLevel]), so entries reach hooks with distorted
// levels: Terragrunt's error level arrives as [logrus.InfoLevel], and the
// debug and trace levels arrive as values logrus doesn't define at all. Used
// directly, the otellogrus bridge would export errors with info severity and
// never fire for debug or trace records. This hook registers for the shifted
// levels and restores the standard logrus level on each entry before handing
// it to the bridge.
func NewOtelLogHook(name string, provider otellog.LoggerProvider) logrus.Hook {
	return newOtelLogHook(name, provider)
}

func newOtelLogHook(name string, provider otellog.LoggerProvider) *otelLogHook {
	return &otelLogHook{
		inner: otellogrus.NewHook(name, otellogrus.WithLoggerProvider(provider)),
	}
}

type otelLogHook struct {
	// inner is the otellogrus bridge that turns entries into log records.
	inner *otellogrus.Hook
	// rootSpan holds the root span used for entries whose logger is not bound
	// to a span.
	rootSpan atomic.Pointer[trace.SpanContext]
}

// Levels implements the [logrus.Hook] interface method.
func (hook *otelLogHook) Levels() []logrus.Level {
	return log.AllLevels.ToLogrusLevels()
}

// Fire implements the [logrus.Hook] interface method.
func (hook *otelLogHook) Fire(entry *logrus.Entry) error {
	e := *entry
	e.Level = stdLogrusLevels[log.FromLogrusLevel(entry.Level)]

	if rootSpan := hook.rootSpan.Load(); rootSpan != nil && !trace.SpanContextFromContext(e.Context).IsValid() {
		e.Context = trace.ContextWithSpanContext(context.Background(), *rootSpan)
	}

	return hook.inner.Fire(&e)
}

// bindRootSpan correlates entries whose logger is not bound to a span with
// span, until [otelLogHook.unbindRootSpan] is called.
func (hook *otelLogHook) bindRootSpan(span trace.SpanContext) {
	hook.rootSpan.Store(&span)
}

// unbindRootSpan stops correlating entries whose logger is not bound to a span.
func (hook *otelLogHook) unbindRootSpan() {
	hook.rootSpan.Store(nil)
}
