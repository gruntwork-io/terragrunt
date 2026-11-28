package venv_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/venv"
)

// TestOSVenvTerminalWithoutTerminal checks every terminal probe reports false when the std streams are files.
//
//nolint:paralleltest // swaps the process-wide os.Stdin, os.Stdout and os.Stderr
func TestOSVenvTerminalWithoutTerminal(t *testing.T) {
	file := createStreamFile(t)
	swapStdStreams(t, file, file, file)

	terminal := venv.OSVenv().Terminal

	require.NotNil(t, terminal)
	assert.False(t, terminal.StdinIsTTY())
	assert.False(t, terminal.StdoutIsTTY())
	assert.False(t, terminal.StderrIsTTY())
	assert.Zero(t, terminal.Width())
	assert.Zero(t, terminal.ErrWidth())
}

// createStreamFile returns a regular file for the std streams, closed when the test ends.
func createStreamFile(t *testing.T) *os.File {
	t.Helper()

	file, err := os.Create(filepath.Join(t.TempDir(), "stream"))
	require.NoError(t, err)

	t.Cleanup(func() { file.Close() })

	return file
}

// swapStdStreams points os.Stdin, os.Stdout and os.Stderr at the given files until the test ends.
func swapStdStreams(t *testing.T, stdin, stdout, stderr *os.File) {
	t.Helper()

	origStdin, origStdout, origStderr := os.Stdin, os.Stdout, os.Stderr

	t.Cleanup(func() {
		os.Stdin, os.Stdout, os.Stderr = origStdin, origStdout, origStderr
	})

	os.Stdin, os.Stdout, os.Stderr = stdin, stdout, stderr
}
