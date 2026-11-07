package writer_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriters_WithWriterReplacesOnlyStdout(t *testing.T) {
	t.Parallel()

	stdout, stderr, replacement := new(bytes.Buffer), new(bytes.Buffer), new(bytes.Buffer)
	ws := &writer.Writers{Writer: stdout, ErrWriter: stderr}

	got := ws.WithWriter(replacement)

	require.NotSame(t, ws, got, "WithWriter should return a copy")
	assert.Same(t, replacement, got.Writer)
	assert.Same(t, stderr, got.ErrWriter, "the error writer should be preserved")
	assert.Same(t, stdout, ws.Writer, "the original should be left unchanged")
}

func TestWriters_WithErrWriterReplacesOnlyStderr(t *testing.T) {
	t.Parallel()

	stdout, stderr, replacement := new(bytes.Buffer), new(bytes.Buffer), new(bytes.Buffer)
	ws := &writer.Writers{Writer: stdout, ErrWriter: stderr}

	got := ws.WithErrWriter(replacement)

	require.NotSame(t, ws, got, "WithErrWriter should return a copy")
	assert.Same(t, replacement, got.ErrWriter)
	assert.Same(t, stdout, got.Writer, "the output writer should be preserved")
	assert.Same(t, stderr, ws.ErrWriter, "the original should be left unchanged")
}

func TestOriginalWriter_DelegatesAndUnwraps(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	ow := writer.NewOriginalWriter(buf)

	n, err := ow.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", buf.String(), "writes should reach the wrapped writer")
	assert.Same(t, buf, ow.Unwrap())
}

func TestWrappedWriter_WritesToWrappedAndUnwrapsToOriginal(t *testing.T) {
	t.Parallel()

	wrapped, original := new(bytes.Buffer), new(bytes.Buffer)
	ww := writer.NewWrappedWriter(wrapped, original)

	n, err := ww.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", wrapped.String(), "writes should reach the wrapped writer")
	assert.Empty(t, original.String(), "writes should not reach the original writer")
	assert.Same(t, original, ww.Unwrap())
}

func TestExtractOriginalWriter(t *testing.T) {
	t.Parallel()

	base := new(bytes.Buffer)

	testCases := []struct {
		in   io.Writer
		want io.Writer
		name string
	}{
		{
			name: "nil writer",
		},
		{
			name: "plain writer is returned as is",
			in:   base,
			want: base,
		},
		{
			name: "original writer unwraps to its target",
			in:   writer.NewOriginalWriter(base),
			want: base,
		},
		{
			name: "nested wrappers unwrap to the innermost writer",
			in:   writer.NewWrappedWriter(io.Discard, writer.NewOriginalWriter(base)),
			want: base,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, writer.ExtractOriginalWriter(tc.in))
		})
	}
}
