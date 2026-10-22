package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetermineToolName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		command  string
		expected string
	}{
		{command: "tofu", expected: "opentofu"},
		{command: "npm", expected: "node"},
		{command: "terraform", expected: "terraform"},
	}

	for _, tc := range testCases {
		t.Run(tc.command, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, determineToolName(tc.command))
		})
	}
}

func TestTestLoggerWrite(t *testing.T) {
	t.Parallel()

	tl := &testLogger{t: t, prefix: "cmd stdout"}

	// Complete lines are logged; a trailing partial line waits for the rest.
	first := []byte("first\n\nsecond")

	n, err := tl.Write(first)
	require.NoError(t, err)
	assert.Equal(t, len(first), n)
	assert.Equal(t, "second", tl.buffer.String())

	rest := []byte(" half\n")

	n, err = tl.Write(rest)
	require.NoError(t, err)
	assert.Equal(t, len(rest), n)
	assert.Zero(t, tl.buffer.Len())
}
