// Package telemetry provides a way to collect telemetry from function execution - metrics and traces.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/errfmt"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"go.opentelemetry.io/otel/trace"
)

// Telemeter collects traces, metrics, and logs for Terragrunt operations. Its zero value collects nothing.
type Telemeter struct {
	*Tracer
	*Meter
	*Logger
	l log.Logger
}

// NewTelemeter initializes the telemetry collector. The logs signal is gated by
// enableLogs (wired to the otel-logs experiment) and stays inert when disabled,
// regardless of any configured logs exporter.
func NewTelemeter(
	ctx context.Context,
	l log.Logger,
	appName, appVersion string,
	writer io.Writer,
	opts *Options,
	env map[string]string,
	enableLogs bool,
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

	if enableLogs {
		logger, err := NewLogger(ctx, appName, appVersion, writer, opts, env)
		if err != nil {
			return nil, errors.Join(err, tlm.Shutdown(ctx))
		}

		tlm.Logger = logger

		// Bridge Terragrunt's logrus-backed logger into the OpenTelemetry logs signal.
		// We attach to the root logger before any command loggers are cloned from it
		// (clones copy hooks by reference at clone time), so every downstream logger
		// inherits the hook and emits records correlated to the active span.
		if logger != nil {
			l.SetOptions(log.WithHooks(logger.hook))
		}
	}

	return tlm, nil
}

// Shutdown flushes all providers at once, so one hung flush cannot use up the others' deadline.
func (tlm *Telemeter) Shutdown(ctx context.Context) error {
	if tlm == nil {
		return nil
	}

	var shutdowns []signalShutdown

	if tlm.Tracer != nil && tlm.Tracer.provider != nil {
		shutdowns = append(shutdowns, signalShutdown{signal: "traces", shutdown: tlm.Tracer.provider.Shutdown})
		tlm.Tracer.provider = nil
	}

	if tlm.Meter != nil && tlm.Meter.provider != nil {
		shutdowns = append(shutdowns, signalShutdown{signal: "metrics", shutdown: tlm.Meter.provider.Shutdown})
		tlm.Meter.provider = nil
	}

	logsShutdown := tlm.Logger != nil && tlm.Logger.provider != nil
	if logsShutdown {
		shutdowns = append(shutdowns, signalShutdown{signal: "logs", shutdown: tlm.Logger.provider.Shutdown})
		tlm.Logger.provider = nil
	}

	errs := make([]error, len(shutdowns)+1)

	var wg sync.WaitGroup

	for i, s := range shutdowns {
		wg.Go(func() {
			if err := s.shutdown(ctx); err != nil {
				errs[i] = fmt.Errorf("%s: %w", s.signal, err)
			}
		})
	}

	wg.Wait()

	if logsShutdown {
		errs[len(shutdowns)] = tlm.Logger.lostRecords()
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
		boundRoot := isRoot && tlm.Logger != nil && tlm.Logger.hook.bindRootSpan(trace.SpanContextFromContext(ctx))
		if boundRoot {
			defer tlm.Logger.hook.unbindRootSpan()
		}

		err := tlm.Time(ctx, name, attrs, func(ctx context.Context) error {
			// Pure-data spans (e.g. discovery, filtering) have no logger to correlate
			// and pass nil; only bind the span context when a logger is supplied.
			childL := l
			if childL != nil {
				childL = childL.WithContext(ctx)
			}

			return fn(ctx, childL)
		})

		if err != nil && boundRoot && tlm.l.Level() >= log.ErrorLevel {
			tlm.Logger.hook.emitError(ctx, errfmt.Format(err))
		}

		return err
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

// signalShutdown is the shutdown function of one telemetry signal.
type signalShutdown struct {
	shutdown func(context.Context) error
	signal   string
}
