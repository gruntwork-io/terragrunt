package config

import (
	"context"
	"encoding/json"
	"errors"
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

			jsonOutputs, err = stateOutputsJSON(reader, location)

			return err
		})
	if err != nil {
		return nil, err
	}

	return jsonOutputs, nil
}

// stateOutputsJSON returns the state's top-level outputs object, reading only as far
// as it must. OpenTofu and Terraform write outputs ahead of resources, so a large
// state normally costs one buffered read instead of a full download.
func stateOutputsJSON(r io.Reader, location string) ([]byte, error) {
	body := &stateBodyReader{r: r}
	d := json.NewDecoder(body)

	tok, err := d.Token()
	if err != nil {
		return nil, stateReadError(body, location, err)
	}

	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, DependencyStateParseError{
			Err:      errors.New("state is not a JSON object"),
			Location: location,
		}
	}

	for d.More() {
		key, err := d.Token()
		if err != nil {
			return nil, stateReadError(body, location, err)
		}

		if name, _ := key.(string); name != "outputs" {
			if err := skipValue(d); err != nil {
				return nil, stateReadError(body, location, err)
			}

			continue
		}

		var outputs json.RawMessage
		if err := d.Decode(&outputs); err != nil {
			return nil, stateReadError(body, location, err)
		}

		if len(outputs) == 0 {
			return []byte("null"), nil
		}

		return outputs, nil
	}

	if _, err := d.Token(); err != nil {
		return nil, stateReadError(body, location, err)
	}

	return []byte("null"), nil
}

// skipValue advances past the next value. Decoding into a RawMessage materializes
// what it skips, which a token walk would avoid, but the token walk costs four times
// as much when it has to step over a large resources array. State files put outputs
// ahead of resources, so this normally skips only scalars.
func skipValue(d *json.Decoder) error {
	var skip json.RawMessage

	return d.Decode(&skip)
}

// stateBodyReader records the last transport failure so a dropped connection is not
// reported as malformed state. The docs point users at a JSON parse error to tell
// them client-side state encryption is on, so that message has to stay specific.
type stateBodyReader struct {
	r   io.Reader
	err error
}

func (r *stateBodyReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		r.err = err
	}

	return n, err
}

func stateReadError(body *stateBodyReader, location string, err error) error {
	if body.err != nil {
		return DependencyStateReadError{
			Err:      body.err,
			Location: location,
		}
	}

	return DependencyStateParseError{
		Err:      err,
		Location: location,
	}
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
