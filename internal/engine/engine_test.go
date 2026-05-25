package engine_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertMetaToProtobuf(t *testing.T) {
	t.Parallel()

	meta := map[string]any{
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
		Writers: writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
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

func TestRun_NonOSBackedExecReturnsSentinel(t *testing.T) {
	t.Parallel()

	sourceFile := filepath.Join(t.TempDir(), "fake-engine")
	require.NoError(t, os.WriteFile(sourceFile, []byte("not-a-real-engine"), 0o600))

	ctx := engine.WithEngineValues(context.Background())

	memExec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{}
	})

	opts := &engine.ExecutionOptions{
		Writers: writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
		EngineOptions: &engine.EngineOptions{
			SkipChecksumCheck: true,
			LogLevel:          "warn",
		},
		EngineConfig: &engine.EngineConfig{
			Source:  sourceFile,
			Version: "v0.0.0",
			Type:    "test",
		},
		WorkingDir: t.TempDir(),
	}

	_, err := engine.Run(ctx, log.New(), memExec, opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, vexec.ErrNotOSBacked)
}
