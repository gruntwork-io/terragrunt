package generate_test

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateStacksWaiterStopsWhenContextEndsWithRacing pins that a caller
// waiting on another generation run in the same working directory returns
// its context's error when the context ends, while the first run finishes.
func TestGenerateStacksWaiterStopsWhenContextEndsWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var (
			liveDir = venvtest.Root("/repo/live")
			unitDir = venvtest.Root("/repo/units/app")
		)

		stackConfig := []byte(`unit "app" {
  source = "../units/app"
  path   = "app"
}
`)

		v := venvtest.New()
		require.NoError(t, v.FS.MkdirAll(liveDir, 0o755))
		require.NoError(t, v.FS.MkdirAll(unitDir, 0o755))
		require.NoError(
			t,
			vfs.WriteFile(v.FS, filepath.Join(liveDir, "terragrunt.stack.hcl"), stackConfig, 0o644),
		)
		require.NoError(
			t,
			vfs.WriteFile(v.FS, filepath.Join(unitDir, "terragrunt.hcl"), nil, 0o644),
		)

		fsys := &openGateFS{FS: v.FS, name: "terragrunt.stack.hcl", release: make(chan struct{})}
		v = v.WithFS(fsys)

		newOpts := func() *options.TerragruntOptions {
			opts := options.NewTerragruntOptions(v.Exec)
			opts.WorkingDir = liveDir
			opts.RootWorkingDir = liveDir
			opts.Parallelism = 1
			opts.NoCAS = true

			return opts
		}

		l := logger.CreateLogger()
		g := generate.NewGenerator()

		waiterCtx, cancelWaiter := context.WithCancel(t.Context())

		var (
			wg        sync.WaitGroup
			firstErr  error
			waiterErr error
		)

		wg.Go(func() {
			firstErr = g.GenerateStacks(t.Context(), l, v, newOpts(), nil)
		})

		synctest.Wait()

		wg.Go(func() {
			waiterErr = g.GenerateStacks(waiterCtx, l, v, newOpts(), nil)
		})

		synctest.Wait()
		cancelWaiter()
		synctest.Wait()

		require.ErrorIs(t, waiterErr, context.Canceled)

		close(fsys.release)
		wg.Wait()

		require.NoError(t, firstErr)
		assert.True(
			t,
			vfs.Exists(v.FS, filepath.Join(liveDir, ".terragrunt-stack", "app", "terragrunt.hcl")),
		)
	})
}

// openGateFS blocks the first Open of a file with base name name until
// release closes.
type openGateFS struct {
	vfs.FS
	release chan struct{}
	name    string
	gated   atomic.Bool
}

// Open waits for release on the first call for a file with base name name,
// then opens path.
func (f *openGateFS) Open(path string) (vfs.File, error) {
	if filepath.Base(path) == f.name && f.gated.CompareAndSwap(false, true) {
		<-f.release
	}

	return f.FS.Open(path)
}
