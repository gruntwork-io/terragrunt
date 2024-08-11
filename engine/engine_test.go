package engine

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsEngineEnabled(t *testing.T) {
	t.Setenv("TG_EXPERIMENTAL_ENGINE", "true")

	assert.True(t, IsEngineEnabled())

	t.Setenv("TG_EXPERIMENTAL_ENGINE", "false")
	assert.False(t, IsEngineEnabled())

	t.Setenv("TG_EXPERIMENTAL_ENGINE", "")
	assert.False(t, IsEngineEnabled())
}

func TestConvertMetaToProtobuf(t *testing.T) {
	t.Parallel()
	meta := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	}

	protoMeta, err := convertMetaToProtobuf(meta)
	require.NoError(t, err)
	require.NotNil(t, protoMeta)
	require.Len(t, protoMeta, 2)
}

func TestReadEngineOutput(t *testing.T) {
	t.Parallel()
	runOptions := &ExecutionOptions{
		CmdStdout: io.Discard,
		CmdStderr: io.Discard,
	}

	outputReturned := false
	outputFn := func() (*outputLine, error) {
		if outputReturned {
			return nil, nil
		}
		outputReturned = true
		return &outputLine{
			Stdout: "stdout output",
			Stderr: "stderr output",
		}, nil
	}

	err := readEngineOutput(runOptions, outputFn)
	assert.NoError(t, err)
}
