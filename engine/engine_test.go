package engine_test

import (
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	err := engine.ReadEngineOutput(runOptions, false, outputFn)
	assert.NoError(t, err)
}
