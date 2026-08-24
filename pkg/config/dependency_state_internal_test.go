package config

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadDependencyStateOutputs covers the paths the shared reader owns for
// every backend: opener failure, read failure, parse failure, and the happy path.
func TestReadDependencyStateOutputs(t *testing.T) {
	t.Parallel()

	errOpen := errors.New("open failed")
	errRead := errors.New("read failed")

	testCases := []struct {
		open    openDependencyState
		wantErr error
		name    string
		want    string
	}{
		{
			name: "outputs are extracted from the state",
			open: func(context.Context, log.Logger) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader(`{"outputs":{"a":{"value":1}}}`)), nil
			},
			want: `{"a":{"value":1}}`,
		},
		{
			name: "state without outputs yields null",
			open: func(context.Context, log.Logger) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader(`{"version":4}`)), nil
			},
			want: "null",
		},
		{
			name: "opener failure propagates",
			open: func(context.Context, log.Logger) (io.ReadCloser, error) {
				return nil, errOpen
			},
			wantErr: errOpen,
		},
		{
			name: "read failure propagates",
			open: func(context.Context, log.Logger) (io.ReadCloser, error) {
				return io.NopCloser(errReader{err: errRead}), nil
			},
			wantErr: errRead,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := readDependencyStateOutputs(
				t.Context(), log.New(), "test_metric", nil, "test://location", testCase.open)

			if testCase.wantErr != nil {
				require.ErrorIs(t, err, testCase.wantErr)

				return
			}

			require.NoError(t, err)
			assert.JSONEq(t, testCase.want, string(got))
		})
	}
}

// TestReadDependencyStateOutputsMalformedState pins that unparsable state is an
// error rather than empty outputs, which would silently feed a dependent unit.
func TestReadDependencyStateOutputsMalformedState(t *testing.T) {
	t.Parallel()

	open := func(context.Context, log.Logger) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("not json")), nil
	}

	_, err := readDependencyStateOutputs(t.Context(), log.New(), "test_metric", nil, "test://location", open)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "test://location")
}

// TestReadDependencyStateOutputsClosesStream pins that the stream is always
// closed, including when parsing fails after a successful read.
func TestReadDependencyStateOutputsClosesStream(t *testing.T) {
	t.Parallel()

	for _, body := range []string{`{"outputs":{}}`, "not json"} {
		stream := &trackedCloser{Reader: strings.NewReader(body)}

		_, _ = readDependencyStateOutputs(t.Context(), log.New(), "test_metric", nil, "test://location",
			func(context.Context, log.Logger) (io.ReadCloser, error) { return stream, nil })

		assert.True(t, stream.closed, "the stream must be closed for body %q", body)
	}
}

// TestStateStreamClosesEveryCloser pins that a stream releases its client as
// well as its reader, and reports every failure rather than the first.
func TestStateStreamClosesEveryCloser(t *testing.T) {
	t.Parallel()

	errFirst := errors.New("first close failed")
	errSecond := errors.New("second close failed")

	first := &trackedCloser{Reader: strings.NewReader(""), err: errFirst}
	second := &trackedCloser{Reader: strings.NewReader(""), err: errSecond}

	err := stateStream{Reader: first, closers: []io.Closer{first, second}}.Close()

	require.ErrorIs(t, err, errFirst)
	require.ErrorIs(t, err, errSecond, "a failing closer must not hide the ones after it")
	assert.True(t, first.closed)
	assert.True(t, second.closed)
}

// TestStateStreamSkipsNilClosers pins that a nil closer cannot panic the run.
func TestStateStreamSkipsNilClosers(t *testing.T) {
	t.Parallel()

	tracked := &trackedCloser{Reader: strings.NewReader("")}

	require.NotPanics(t, func() {
		require.NoError(t, stateStream{Reader: tracked, closers: []io.Closer{nil, tracked, nil}}.Close())
	})

	assert.True(t, tracked.closed, "a nil entry must not stop the closers after it")
}

// TestCloserFuncRunsCleanup pins the adapter used to release the Azure timeout.
func TestCloserFuncRunsCleanup(t *testing.T) {
	t.Parallel()

	ran := false

	require.NoError(t, closerFunc(func() error { ran = true; return nil }).Close())
	assert.True(t, ran)
}

// errReader fails every read, standing in for a truncated download.
type errReader struct {
	err error
}

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// trackedCloser records whether it was closed and can fail on demand.
type trackedCloser struct {
	io.Reader
	err    error
	closed bool
}

func (c *trackedCloser) Close() error {
	c.closed = true

	return c.err
}
