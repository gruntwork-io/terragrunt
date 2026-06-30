package runnerpool_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
)

func TestUnitWriter_WriteErrorPropagation(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("write failed")
	failingWriter := &failingWriter{err: writeErr}

	writer := runnerpool.NewUnitWriter(failingWriter)

	data := []byte("line 1\nline 2\n")
	n, err := writer.Write(data)

	require.Error(t, err)
	require.Equal(t, writeErr, err)
	require.Equal(t, len(data), n)

	err = writer.Flush()
	require.Error(t, err)
	require.Equal(t, writeErr, err)
}

func TestUnitWriter_FlushCompleteLines(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	writer := runnerpool.NewUnitWriter(&buf)

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

func TestUnitWriter_Unwrap(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	writer := runnerpool.NewUnitWriter(&buf)
	assert.Equal(t, &buf, writer.Unwrap())
}

func TestUnitWriter_NilOutput(t *testing.T) {
	t.Parallel()

	writer := runnerpool.NewUnitWriter(nil)

	n, err := writer.Write([]byte("data\n"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	err = writer.Flush()
	require.NoError(t, err)
}

func TestUnitWriter_NoNewline(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	writer := runnerpool.NewUnitWriter(&buf)

	_, err := writer.Write([]byte("no newline"))
	require.NoError(t, err)
	require.Empty(t, buf.String(), "partial line should not be flushed")

	err = writer.Flush()
	require.NoError(t, err)
	require.Equal(t, "no newline", buf.String())
}

type failingWriter struct {
	err error
}

func (w *failingWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}
