package engine_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/require"
)

// TestRunEndedContextSkipsDownloadWaitWithRacing pins that a unit whose
// context has ended does not wait on another unit's download of the same
// engine, while the first unit goes on to start the engine.
//
// It runs outside a synctest bubble because Run leaves a goroutine reading the
// engine plugin's log when the plugin fails to start, which a bubble reports
// as a deadlock.
func TestRunEndedContextSkipsDownloadWaitWithRacing(t *testing.T) {
	t.Parallel()

	const (
		cachePath     = "/cache"
		engineType    = "test"
		engineVersion = "v0.0.0"
	)

	v := venvtest.New().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{}
	})

	engineDir := filepath.Join(
		cachePath,
		"terragrunt",
		"plugins",
		"iac-engine",
		engineType,
		engineVersion,
		v.Platform.GOOS,
		v.Platform.GOARCH,
	)
	engineFile := fmt.Sprintf(
		"terragrunt-iac-engine_%s_%s_%s_%s",
		engineType,
		engineVersion,
		v.Platform.GOOS,
		v.Platform.GOARCH,
	)

	require.NoError(t, v.FS.MkdirAll(engineDir, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(engineDir, engineFile), nil, 0o755))

	fsys := &statGateFS{
		FS:      v.FS,
		dir:     engineDir,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	v = v.WithFS(fsys)

	l := logger.CreateLogger()
	ctx := engine.WithEngineValues(t.Context())

	waiterCtx, cancelWaiter := context.WithCancel(ctx)
	cancelWaiter()

	newOpts := func(unitDir string) *engine.ExecutionOptions {
		return &engine.ExecutionOptions{
			EngineOptions: &engine.EngineOptions{
				CachePath:         cachePath,
				SkipChecksumCheck: true,
				LogLevel:          "warn",
			},
			EngineConfig: &engine.EngineConfig{
				Source:  "https://example.com/engine",
				Version: engineVersion,
				Type:    engineType,
			},
			CacheDir: unitDir,
			UnitDir:  unitDir,
		}
	}

	var (
		wg       sync.WaitGroup
		firstErr error
		waiterCh = make(chan error, 1)
	)

	wg.Go(func() {
		_, firstErr = engine.Run(ctx, l, v, newOpts("/first"))
	})

	<-fsys.entered

	wg.Go(func() {
		_, err := engine.Run(waiterCtx, l, v, newOpts("/waiter"))
		waiterCh <- err
	})

	var waiterErr error

	select {
	case waiterErr = <-waiterCh:
	case <-time.After(10 * time.Second):
		close(fsys.release)
		wg.Wait()
		require.FailNow(t, "a unit with an ended context waited on another unit's download")
	}

	require.ErrorIs(t, waiterErr, context.Canceled)

	close(fsys.release)
	wg.Wait()

	require.ErrorIs(t, firstErr, vexec.ErrNotOSBacked)
}

// statGateFS blocks the first Stat of a file in dir until release closes,
// closing entered once that Stat starts.
type statGateFS struct {
	vfs.FS
	entered chan struct{}
	release chan struct{}
	dir     string
	gated   atomic.Bool
}

// Stat waits for release on the first call for a file in dir, then stats name.
func (f *statGateFS) Stat(name string) (os.FileInfo, error) {
	if filepath.Dir(name) == f.dir && f.gated.CompareAndSwap(false, true) {
		close(f.entered)
		<-f.release
	}

	return f.FS.Stat(name)
}
