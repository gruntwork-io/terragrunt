package commands_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapWithProfilingNoFlagsRunsAction(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())

	called := false
	wrapped := commands.WrapWithProfiling(logger.CreateLogger(), opts, v)
	err := wrapped(t.Context(), &clihelper.Context{}, func(_ context.Context, _ *clihelper.Context) error {
		called = true

		return nil
	})

	require.NoError(t, err)
	assert.True(t, called)
}

func TestWrapWithProfilingWritesProfile(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.ProfileGoroutine = filepath.Join(t.TempDir(), "goroutine.prof")

	wrapped := commands.WrapWithProfiling(logger.CreateLogger(), opts, v)
	err := wrapped(t.Context(), &clihelper.Context{}, func(_ context.Context, _ *clihelper.Context) error {
		return nil
	})
	require.NoError(t, err)

	info, err := v.FS.Stat(opts.ProfileGoroutine)
	require.NoError(t, err)
	assert.Positive(t, info.Size(), "goroutine profile should be non-empty")
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "profile files must be owner-only")
}

func TestWrapWithProfilingTightensExistingFilePermissions(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.ProfileGoroutine = filepath.Join(t.TempDir(), "goroutine.prof")

	require.NoError(t, vfs.WriteFile(v.FS, opts.ProfileGoroutine, []byte("stale"), 0o644))

	wrapped := commands.WrapWithProfiling(logger.CreateLogger(), opts, v)
	err := wrapped(t.Context(), &clihelper.Context{}, func(_ context.Context, _ *clihelper.Context) error {
		return nil
	})
	require.NoError(t, err)

	info, err := v.FS.Stat(opts.ProfileGoroutine)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "a pre-existing profile file must be tightened to owner-only")
}

// TestWrapWithProfilingProfileDirWritesAllProfiles pins profile-dir output, nested-action reuse, and cleanup when a second CPU profiler fails.
func TestWrapWithProfilingProfileDirWritesAllProfiles(t *testing.T) {
	t.Parallel()

	// The CPU profiler is process-wide, so no other test in this package may start it.

	v := venv.MemVenv()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = testWorkDir
	opts.ProfileDir = "profiles"

	competing := options.NewTerragruntOptions(vexec.NewOSExec())
	competing.ProfileDir = hostPath("/competing")

	var nestedRan bool

	wrapped := commands.WrapWithProfiling(logger.CreateLogger(), opts, v)
	err := wrapped(t.Context(), &clihelper.Context{}, func(ctx context.Context, cliCtx *clihelper.Context) error {
		require.NoError(t, wrapped(ctx, cliCtx, func(context.Context, *clihelper.Context) error {
			nestedRan = true

			return nil
		}))

		competingErr := commands.WrapWithProfiling(logger.CreateLogger(), competing, v)(ctx, cliCtx, failIfCalled(t))
		require.Error(t, competingErr, "a second CPU profiler should fail to start")

		return nil
	})
	require.NoError(t, err)

	assert.True(t, nestedRan, "a nested action should run without starting new profiles")

	wantProfileDir := filepath.Join(testWorkDir, "profiles")

	assert.Equal(t, wantProfileDir, opts.ProfileDir, "a relative profile dir should be made absolute")

	for _, name := range []string{"terragrunt_cpu.prof", "terragrunt_mem.prof", "terragrunt_goroutine.prof"} {
		info, statErr := v.FS.Stat(filepath.Join(wantProfileDir, name))
		require.NoError(t, statErr, "profile %s should exist", name)
		assert.Positive(t, info.Size(), "profile %s should be non-empty", name)
	}

	for _, name := range []string{"terragrunt_cpu.prof", "terragrunt_mem.prof", "terragrunt_goroutine.prof"} {
		failedProfile := filepath.Join(competing.ProfileDir, name)

		assert.False(t, vfs.Exists(v.FS, failedProfile), "failed profile %s should be removed", name)
	}
}

// TestWrapWithProfilingStartupFailures pins that a profile that cannot be created fails the command and removes the profiles created before it.
func TestWrapWithProfilingStartupFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		op          string
		path        string
		wantRemoved []string
	}{
		{name: "profile dir", op: "mkdirall", path: "/profiles"},
		{name: "memory profile", op: "openfile", path: "/profiles/terragrunt_mem.prof"},
		{
			name:        "goroutine profile",
			op:          "openfile",
			path:        "/profiles/terragrunt_goroutine.prof",
			wantRemoved: []string{"/profiles/terragrunt_mem.prof"},
		},
		{
			name:        "cpu profile",
			op:          "openfile",
			path:        "/profiles/terragrunt_cpu.prof",
			wantRemoved: []string{"/profiles/terragrunt_mem.prof", "/profiles/terragrunt_goroutine.prof"},
		},
		{name: "profile permissions", op: "chmod", path: "/profiles/terragrunt_mem.prof"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv()
			v = v.WithFS(newFaultFS(v.FS, atPath(tc.op, tc.path)))

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.ProfileDir = "/profiles"

			err := commands.WrapWithProfiling(logger.CreateLogger(), opts, v)(t.Context(), &clihelper.Context{}, failIfCalled(t))
			require.ErrorIs(t, err, errInjected)

			for _, path := range tc.wantRemoved {
				assert.False(t, vfs.Exists(v.FS, path), "profile %s should be removed after the failure", path)
			}
		})
	}
}

// failIfCalled returns an action that fails the test when it runs.
func failIfCalled(t *testing.T) clihelper.ActionFunc {
	t.Helper()

	return func(context.Context, *clihelper.Context) error {
		assert.Fail(t, "action should not run when profiling fails to start")

		return nil
	}
}

// TestWrapWithProfilingFlushFailuresDoNotFailCommand pins that a profile that fails to write or close only warns.
func TestWrapWithProfilingFlushFailuresDoNotFailCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		op        string
		mem       string
		goroutine string
	}{
		{name: "memory profile write", op: "write", mem: hostPath("/profiles/mem.prof")},
		{name: "goroutine profile write", op: "write", goroutine: hostPath("/profiles/goroutine.prof")},
		{name: "memory profile close", op: "close", mem: hostPath("/profiles/mem.prof")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv()
			v = v.WithFS(newFaultFS(v.FS, atPath(tc.op, tc.mem+tc.goroutine)))

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.ProfileMem = tc.mem
			opts.ProfileGoroutine = tc.goroutine

			actionRan := false

			err := commands.WrapWithProfiling(logger.CreateLogger(), opts, v)(t.Context(), &clihelper.Context{},
				func(context.Context, *clihelper.Context) error {
					actionRan = true

					return nil
				})
			require.NoError(t, err, "a profile flush failure should not fail the command")
			assert.True(t, actionRan, "the action should run")
		})
	}
}

// TestWrapWithProfilingCleanupFailuresKeepStartupError pins that failing to clean up after a startup failure still reports that failure.
func TestWrapWithProfilingCleanupFailuresKeepStartupError(t *testing.T) {
	t.Parallel()

	memPath := hostPath("/profiles/mem.prof")
	goroutinePath := hostPath("/profiles/goroutine.prof")

	testCases := []struct {
		name        string
		faults      []fault
		wantMemFile bool
	}{
		{
			name:   "discarded profile fails to close",
			faults: []fault{atPath("openfile", goroutinePath), atPath("close", memPath)},
		},
		{
			name:        "discarded profile fails to remove",
			faults:      []fault{atPath("openfile", goroutinePath), atPath("remove", memPath)},
			wantMemFile: true,
		},
		{
			name:        "profile fails to close after a permission failure",
			faults:      []fault{atPath("chmod", memPath), atPath("close", memPath)},
			wantMemFile: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv()
			v = v.WithFS(newFaultFS(v.FS, tc.faults...))

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.ProfileMem = memPath
			opts.ProfileGoroutine = goroutinePath

			err := commands.WrapWithProfiling(logger.CreateLogger(), opts, v)(t.Context(), &clihelper.Context{}, failIfCalled(t))
			require.ErrorIs(t, err, errInjected)
			assert.Equal(t, tc.wantMemFile, vfs.Exists(v.FS, memPath), "memory profile presence after cleanup")
		})
	}
}
