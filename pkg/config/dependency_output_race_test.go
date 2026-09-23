package config_test

import (
	"context"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadOutputsJSONWaiterStopsWhenContextEndsWithRacing pins that a caller
// waiting on another caller's output fetch for the same unit returns its
// context's error when the context ends, while the first fetch finishes.
func TestReadOutputsJSONWaiterStopsWhenContextEndsWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var (
			outputs atomic.Int32
			release = make(chan struct{})
		)

		exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
			if !slices.Contains(inv.Args, "output") {
				return vexec.Result{}
			}

			outputs.Add(1)

			<-release

			return vexec.Result{Stdout: terraformOutput("from-native-output")}
		})

		v := venvtest.New().WithEnv(map[string]string{}).WithExec(exec)

		unitDir := venvtest.Root("/repo/producer")
		require.NoError(t, v.FS.MkdirAll(unitDir, 0o700))
		require.NoError(
			t,
			vfs.WriteFile(
				v.FS,
				filepath.Join(unitDir, config.DefaultTerragruntConfigPath),
				nil,
				0o600,
			),
		)
		require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(unitDir, "main.tf"), nil, 0o600))

		consumerPath := venvtest.Root("/repo/consumer/terragrunt.hcl")

		l := logger.CreateLogger()
		ctx, pctx := newTestParsingContext(t, v, consumerPath)
		ctx = config.WithConfigValues(ctx)
		pctx.OriginalTerragruntConfigPath = consumerPath

		unit := &config.Unit{Name: "producer"}

		waiterCtx, cancelWaiter := context.WithCancel(ctx)

		var (
			wg        sync.WaitGroup
			firstOut  []byte
			firstErr  error
			waiterErr error
		)

		wg.Go(func() {
			firstOut, firstErr = unit.ReadOutputsJSON(ctx, l, pctx, unitDir)
		})

		synctest.Wait()

		wg.Go(func() {
			_, waiterErr = unit.ReadOutputsJSON(waiterCtx, l, pctx, unitDir)
		})

		synctest.Wait()
		cancelWaiter()
		synctest.Wait()

		require.ErrorIs(t, waiterErr, context.Canceled)

		close(release)
		wg.Wait()

		require.NoError(t, firstErr)
		assert.Contains(t, string(firstOut), "from-native-output")
		assert.Equal(t, int32(1), outputs.Load())
	})
}
