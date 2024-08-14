package engine_test

import (
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsEngineEnabled(t *testing.T) {
	t.Setenv("TG_EXPERIMENTAL_ENGINE", "true")

	assert.True(t, engine.IsEngineEnabled())

	t.Setenv("TG_EXPERIMENTAL_ENGINE", "false")
	assert.False(t, engine.IsEngineEnabled())

	t.Setenv("TG_EXPERIMENTAL_ENGINE", "")
	assert.False(t, engine.IsEngineEnabled())
}

func TestConvertMetaToProtobuf(t *testing.T) {
	t.Parallel()
	meta := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	}

	protoMeta, err := engine.ConvertMetaToProtobuf(meta)
	require.NoError(t, err)
	assert.NotNil(t, protoMeta)
	assert.Len(t, protoMeta, 2)
}

func TestReadEngineOutput(t *testing.T) {
	t.Parallel()
	runOptions := &engine.ExecutionOptions{
		CmdStdout: io.Discard,
		CmdStderr: io.Discard,
	}

	outputReturned := false
	outputFn := func() (*engine.OutputLine, error) {
		if outputReturned {
			return nil, nil
		}
		outputReturned = true
		return &engine.OutputLine{
			Stdout: "stdout output",
			Stderr: "stderr output",
		}, nil
	}

	err := engine.ReadEngineOutput(runOptions, outputFn)
	assert.NoError(t, err)
}
