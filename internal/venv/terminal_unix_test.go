//go:build !windows

package venv_test

import (
	"os"
	"testing"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/venv"
)

// TestOSVenvTerminalProbesOwnStream checks each probe reports only its own stream, using a pseudo-terminal.
//
//nolint:paralleltest // swaps the process-wide os.Stdin, os.Stdout and os.Stderr
func TestOSVenvTerminalProbesOwnStream(t *testing.T) {
	const columns = 132

	testCases := []struct {
		name         string
		ttyStream    string
		wantWidth    int
		wantErrWidth int
		wantStdin    bool
		wantStdout   bool
		wantStderr   bool
	}{
		{
			name:      "stdin is the terminal",
			ttyStream: "stdin",
			wantStdin: true,
		},
		{
			name:       "stdout is the terminal",
			ttyStream:  "stdout",
			wantStdout: true,
			wantWidth:  columns,
		},
		{
			name:         "stderr is the terminal",
			ttyStream:    "stderr",
			wantStderr:   true,
			wantErrWidth: columns,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tty := openPTY(t, columns)
			file := createStreamFile(t)

			streams := map[string]*os.File{"stdin": file, "stdout": file, "stderr": file}
			streams[tc.ttyStream] = tty

			swapStdStreams(t, streams["stdin"], streams["stdout"], streams["stderr"])

			terminal := venv.OSVenv().Terminal

			require.NotNil(t, terminal)
			assert.Equal(t, tc.wantStdin, terminal.StdinIsTTY())
			assert.Equal(t, tc.wantStdout, terminal.StdoutIsTTY())
			assert.Equal(t, tc.wantStderr, terminal.StderrIsTTY())
			assert.Equal(t, tc.wantWidth, terminal.Width())
			assert.Equal(t, tc.wantErrWidth, terminal.ErrWidth())
		})
	}
}

// openPTY opens a pseudo-terminal with the given width and returns its terminal end.
func openPTY(t *testing.T, columns uint16) *os.File {
	t.Helper()

	ptmx, tty, err := pty.Open()
	require.NoError(t, err)

	t.Cleanup(func() {
		tty.Close()
		ptmx.Close()
	})

	require.NoError(t, pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: columns}))

	return tty
}
