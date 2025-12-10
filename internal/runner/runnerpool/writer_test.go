package runnerpool_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
)

func TestUnitWriter_WriteErrorPropagation(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("write failed")
	failingWriter := &failingWriter{err: writeErr}

	ctx := context.Background()
	writer := runnerpool.NewUnitWriter(ctx, failingWriter)

	data := []byte("line 1\nline 2\n")
	n, err := writer.Write(data)

	require.Error(t, err)
	require.Equal(t, writeErr, err)
	require.Equal(t, len(data), n)

	err = writer.Flush()
	require.Error(t, err)
	require.Equal(t, writeErr, err)
}

func TestUnitWriter_NilContext(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	//nolint:staticcheck // Intentionally testing nil context handling
	writer := runnerpool.NewUnitWriter(nil, &buf)

	data := []byte("test output\n")
	_, err := writer.Write(data)
	require.NoError(t, err)

	require.Contains(t, buf.String(), "test output")

	err = writer.Flush()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	writer2 := runnerpool.NewUnitWriter(ctx, &buf)

	_, err = writer2.Write([]byte("cancelled output\n"))
	require.NoError(t, err)
	require.Contains(t, buf.String(), "cancelled output")
}

func TestUnitWriter_FlushOnCancel(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	ctx, cancel := context.WithCancel(context.Background())
	writer := runnerpool.NewUnitWriter(ctx, &buf)

	data := []byte("partial line")
	_, err := writer.Write(data)
	require.NoError(t, err)
	require.Empty(t, buf.String())

	cancel()

	_, err = writer.Write([]byte(" more\n"))
	require.NoError(t, err)

	require.Contains(t, buf.String(), "partial line more")
}

func TestUnitWriter_FlushCompleteLines(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	writer := runnerpool.NewUnitWriter(context.Background(), &buf)

	data := []byte("line 1\nline 2\npartial")
	_, err := writer.Write(data)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "line 1")
	require.Contains(t, output, "line 2")
	require.NotContains(t, output, "partial")

	err = writer.Flush()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "partial")
}

type failingWriter struct {
	err error
}

func (w *failingWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}
