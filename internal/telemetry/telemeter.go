// Package telemetry provides a way to collect telemetry from function execution - metrics and traces.
package telemetry

import (
	"context"
	"errors"
	"io"
	"runtime/debug"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"go.opentelemetry.io/otel/trace"
)

type Telemeter struct {
	*Tracer
	*Meter
	*Logger
	l log.Logger
}

// NewTelemeter sets up traces, metrics, and logs, skipping signals without an exporter.
func NewTelemeter(
	ctx context.Context,
	l log.Logger,
	appName, appVersion string,
	writer io.Writer,
	opts *Options,
) (*Telemeter, error) {
	tlm := &Telemeter{l: l}

	tracer, err := NewTracer(ctx, l, appName, appVersion, writer, opts)
	if err != nil {
		return nil, err
	}

	tlm.Tracer = tracer

	// Shut down the providers created so far when a later signal fails to
	// initialize, so a partial setup doesn't leak their background workers.
	meter, err := NewMeter(ctx, appName, appVersion, writer, opts)
	if err != nil {
		return nil, errors.Join(err, tlm.Shutdown(ctx))
	}

	tlm.Meter = meter

	logger, err := NewLogger(ctx, appName, appVersion, writer, opts)
	if err != nil {
		return nil, errors.Join(err, tlm.Shutdown(ctx))
	}

	tlm.Logger = logger

	// Attach the hook to the root logger before commands clone it, so every logger exports.
	if logger != nil {
		l.SetOptions(log.WithHooks(logger.hook))
	}

	return tlm, nil
}

// Shutdown flushes every provider, even if one fails, and returns all errors.
func (tlm *Telemeter) Shutdown(ctx context.Context) error {
	if tlm == nil {
		return nil
	}

	var errs []error

	if tlm.Tracer != nil && tlm.Tracer.provider != nil {
		errs = append(errs, tlm.Tracer.provider.Shutdown(ctx))
		tlm.Tracer.provider = nil
	}

	if tlm.Meter != nil && tlm.Meter.provider != nil {
		errs = append(errs, tlm.Meter.provider.Shutdown(ctx))
		tlm.Meter.provider = nil
	}

	if tlm.Logger != nil && tlm.Logger.provider != nil {
		errs = append(errs, tlm.Logger.provider.Shutdown(ctx))
		tlm.Logger.provider = nil
	}

	return errors.Join(errs...)
}

// Collect collects telemetry from function execution metrics and traces.
//
// The callback receives the span's context and a logger bound to that context.
// Logging through this childL is what links a unit's records to its span: the
// OpenTelemetry log bridge derives trace and span IDs from the logger's context,
// so records correlate with their span only when the logger carries it. Pass
// childL onward to any code whose logs should resolve to this span.
// If ctx has no span, records from loggers without a span use the new root span.
func (tlm *Telemeter) Collect(
	ctx context.Context, l log.Logger, name string, attrs map[string]any,
	fn func(childCtx context.Context, childL log.Logger) error,
) error {
	if tlm == nil {
		// This should not happen in normal operation. Log a stack trace to help
		// diagnose if this nil guard is the one preventing a panic.
		if l := telemeterLoggerFromContext(ctx); l != nil {
			l.Debugf(
				"Telemeter.Collect called with nil receiver for %q, bypassing telemetry. Stack:\n%s",
				name,
				debug.Stack(),
			)
		}

		return fn(ctx, l)
	}

	isRoot := !trace.SpanContextFromContext(ctx).IsValid()

	// wrap telemetry collection with trace and time metric
	return tlm.Trace(ctx, name, attrs, func(ctx context.Context) error {
		if span := trace.SpanContextFromContext(ctx); isRoot && tlm.Logger != nil && span.IsValid() {
			tlm.Logger.hook.bindRootSpan(span)
			defer tlm.Logger.hook.unbindRootSpan()
		}

		return tlm.Time(ctx, name, attrs, func(ctx context.Context) error {
			// Pure-data spans (e.g. discovery, filtering) have no logger to correlate
			// and pass nil; only bind the span context when a logger is supplied.
			childL := l
			if childL != nil {
				childL = childL.WithContext(ctx)
			}

			return fn(ctx, childL)
		})
	})
}

// WithoutLogger adapts a logger-less callback to [Telemeter.Collect]'s
// signature, for pure-data spans that have no logger to correlate.
func WithoutLogger(
	fn func(ctx context.Context) error,
) func(ctx context.Context, l log.Logger) error {
	return func(ctx context.Context, _ log.Logger) error {
		return fn(ctx)
	}
}

// telemeterLoggerFromContext attempts to retrieve the logger from a telemeter
// stored in the context. Returns nil if no telemeter or logger is available.
func telemeterLoggerFromContext(ctx context.Context) log.Logger {
	if val := ctx.Value(telemeterContextKey); val != nil {
		if tlm, ok := val.(*Telemeter); ok && tlm != nil {
			return tlm.l
		}
	}

	return nil
}
