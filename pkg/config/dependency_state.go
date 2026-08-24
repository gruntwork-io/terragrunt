package config

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// openDependencyState opens a backend's state stream. The returned reader owns
// every resource it needs, including the client that produced it.
type openDependencyState func(ctx context.Context, l log.Logger) (io.ReadCloser, error)

// readDependencyStateOutputs wraps the telemetry, read, and parse that every
// backend shares around a backend-specific opener, so each reader only has to
// say where its state lives and how to open it.
func readDependencyStateOutputs(
	ctx context.Context,
	l log.Logger,
	metric string,
	attrs map[string]any,
	location string,
	open openDependencyState,
) ([]byte, error) {
	l.Debugf("Fetching outputs directly from %s", location)

	var jsonOutputs []byte

	err := telemetry.TelemeterFromContext(ctx).
		Collect(ctx, l, metric, attrs, func(ctx context.Context, l log.Logger) error {
			reader, err := open(ctx, l)
			if err != nil {
				return err
			}

			defer func() {
				if err := reader.Close(); err != nil {
					l.Warnf("Failed to close dependency state reader for %s: %v", location, err)
				}
			}()

			stateBody, err := io.ReadAll(reader)
			if err != nil {
				return fmt.Errorf("reading dependency state body from %s: %w", location, err)
			}

			jsonOutputs, err = terraformStateOutputsJSON(stateBody, location)

			return err
		})
	if err != nil {
		return nil, err
	}

	return jsonOutputs, nil
}

// stateStream pairs a state reader with the client that produced it, so closing
// the stream releases both.
type stateStream struct {
	io.Reader
	closers []io.Closer
}

// Close releases the stream and its client, reporting every failure.
func (s stateStream) Close() error {
	errs := make([]error, 0, len(s.closers))

	for _, closer := range s.closers {
		if closer == nil {
			continue
		}

		errs = append(errs, closer.Close())
	}

	return errors.Join(errs...)
}

// closerFunc adapts a cleanup function to io.Closer.
type closerFunc func() error

// Close runs the cleanup function.
func (f closerFunc) Close() error { return f() }
