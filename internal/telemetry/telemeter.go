// Package telemetry provides a way to collect telemetry from function execution - metrics and traces.
package telemetry

import (
	"context"
	"io"
	"runtime/debug"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

type Telemeter struct {
	*Tracer
	*Meter
	l log.Logger
}

// NewTelemeter initializes the telemetry collector.
func NewTelemeter(ctx context.Context, l log.Logger, appName, appVersion string, writer io.Writer, opts *Options) (*Telemeter, error) {
	tracer, err := NewTracer(ctx, l, appName, appVersion, writer, opts)
	if err != nil {
		return nil, errors.New(err)
	}

	meter, err := NewMeter(ctx, appName, appVersion, writer, opts)
	if err != nil {
		return nil, errors.New(err)
	}

	return &Telemeter{
		Tracer: tracer,
		Meter:  meter,
		l:      l,
	}, nil
}

// Shutdown shutdowns the telemetry provider.
func (tlm *Telemeter) Shutdown(ctx context.Context) error {
	if tlm == nil {
		return nil
	}

	if tlm.Tracer != nil && tlm.Tracer.provider != nil {
		if err := tlm.Tracer.provider.Shutdown(ctx); err != nil {
			return errors.New(err)
		}

		tlm.Tracer.provider = nil
	}

	if tlm.Meter != nil && tlm.Meter.provider != nil {
		if err := tlm.Meter.provider.Shutdown(ctx); err != nil {
			return errors.New(err)
		}

		tlm.Meter.provider = nil
	}

	return nil
}

// Collect collects telemetry from function execution metrics and traces.
func (tlm *Telemeter) Collect(ctx context.Context, name string, attrs map[string]any, fn func(childCtx context.Context) error) error {
	if tlm == nil {
		// This should not happen in normal operation. Log a stack trace to help
		// diagnose if this nil guard is the one preventing a panic.
		if l := telemeterLoggerFromContext(ctx); l != nil {
			l.Debugf("Telemeter.Collect called with nil receiver for %q, bypassing telemetry. Stack:\n%s", name, debug.Stack())
		}

		return fn(ctx)
	}

	// wrap telemetry collection with trace and time metric
	return tlm.Trace(ctx, name, attrs, func(ctx context.Context) error {
		return tlm.Time(ctx, name, attrs, fn)
	})
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
