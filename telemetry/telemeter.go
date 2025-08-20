// Package telemetry provides a way to collect telemetry from function execution - metrics and traces.
package telemetry

import (
	"context"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

type Telemeter struct {
	*Tracer
	*Meter
}

// NewTelemeter initializes the telemetry collector.
func NewTelemeter(ctx context.Context, appName, appVersion string, writer io.Writer, opts *Options) (*Telemeter, error) {
	tracer, err := NewTracer(ctx, appName, appVersion, writer, opts)
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
	}, nil
}

// Shutdown shutdowns the telemetry provider.
func (tlm *Telemeter) Shutdown(ctx context.Context) error {
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
	// wrap telemetry collection with trace and time metric
	return tlm.Trace(ctx, name, attrs, func(ctx context.Context) error {
		return tlm.Time(ctx, name, attrs, fn)
	})
}
