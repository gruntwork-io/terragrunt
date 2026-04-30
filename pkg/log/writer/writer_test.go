package writer_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/writer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriterWrite(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger(log.InfoLevel)
	w := writer.New(
		writer.WithLogger(logger),
	)

	n, err := w.Write([]byte("hello writer"))
	require.NoError(t, err)
	assert.Len(t, "hello writer", n)
	assert.Contains(t, buf.String(), "hello writer")
}

func TestWriterWithMsgSeparator(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger(log.InfoLevel)
	w := writer.New(
		writer.WithLogger(logger),
		writer.WithMsgSeparator("\n"),
	)

	_, err := w.Write([]byte("line1\nline2\nline3"))
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "line1")
	assert.Contains(t, output, "line2")
	assert.Contains(t, output, "line3")
}

func TestWriterWithDefaultLevel(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger(log.TraceLevel)
	w := writer.New(
		writer.WithLogger(logger),
		writer.WithDefaultLevel(log.DebugLevel),
	)

	_, err := w.Write([]byte("debug message"))
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "debug message")
}

func TestWriterWithParseFunc(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger(log.TraceLevel)
	warnLevel := log.WarnLevel

	w := writer.New(
		writer.WithLogger(logger),
		writer.WithParseFunc(func(str string) (string, *time.Time, *log.Level, error) {
			return "parsed: " + str, nil, &warnLevel, nil
		}),
	)

	_, err := w.Write([]byte("raw message"))
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "parsed: raw message")
}

func TestWriterEmptyInput(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger(log.InfoLevel)
	w := writer.New(
		writer.WithLogger(logger),
		writer.WithMsgSeparator("\n"),
	)

	n, err := w.Write([]byte(""))
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Empty(t, buf.String())
}

func newTestLogger(level log.Level) (log.Logger, *bytes.Buffer) {
	buf := new(bytes.Buffer)
	logger := log.New(
		log.WithLevel(level),
		log.WithOutput(buf),
	)

	return logger, buf
}
