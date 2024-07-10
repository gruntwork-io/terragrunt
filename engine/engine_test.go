package engine

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert.NoError(t, err)
	assert.NotNil(t, protoMeta)
	assert.Equal(t, 2, len(protoMeta))
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
