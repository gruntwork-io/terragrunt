package run_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunPipelineEndToEndPlan exercises run.Run from auth all the way
// to the terraform plan invocation against a mem-backed exec. The mem
// backend captures the terraform subprocess args so we can assert that
// the full pipeline produces the expected `tofu plan` call.
//
// Before this work, the same assertion required a real `tofu` binary
// on PATH and a real terragrunt integration test.
func TestRunPipelineEndToEndPlan(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)

	var planCalls atomic.Int32

	rec := &invocationRecorder{}
	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		rec.record(&inv)

		if inv.Name == "tofu" && slices.Contains(inv.Args, "plan") {
			planCalls.Add(1)
			return vexec.Result{Stdout: []byte("Plan: 0 to add, 0 to change, 0 to destroy.\n")}
		}

		// Any other invocation (e.g. a stray init) should fail loudly so
		// the test surfaces a pipeline regression rather than silently
		// passing.
		return vexec.Result{ExitCode: 127, Stderr: []byte("unexpected invocation: " + inv.Name)}
	})

	// FS uses NewOSFS because DownloadTerraformSource still copies real
	// files (the temp scaffolding). The mem backend is only for exec
	// virtualization; fs virtualization remains a future item.
	v := &run.Venv{
		Exec:    exec,
		FS:      vfs.NewOSFS(),
		Env:     map[string]string{},
		Writers: &writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
	}
	l := logger.CreateLogger()

	opts := newRunE2EOpts(t, s, "plan")
	cfg := &runcfg.RunConfig{}
	credsGetter := creds.NewGetter()

	err := run.Run(t.Context(), l, v, opts, report.NewReport(), cfg, credsGetter)
	require.NoError(t, err, "run.Run must succeed when the mem backend returns a clean plan")

	assert.Equal(t, int32(1), planCalls.Load(), "exactly one tofu plan call expected")

	// The plan invocation must use the configured TFPath as the binary
	// name and carry `plan` as the first arg.
	calls := rec.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, "tofu", calls[0].Name)
	assert.Contains(t, calls[0].Args, "plan")
}

// TestRunPipelineEndToEndPropagatesPlanFailure pins the contract that
// a non-zero terraform exit causes run.Run to return an error. The mem
// backend lets us trigger the failure deterministically without
// terraform-specific state setup.
func TestRunPipelineEndToEndPropagatesPlanFailure(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{ExitCode: 1, Stderr: []byte("Error: state lock acquired by another process\n")}
	})

	// FS uses NewOSFS because DownloadTerraformSource still copies real
	// files (the temp scaffolding). The mem backend is only for exec
	// virtualization; fs virtualization remains a future item.
	v := &run.Venv{
		Exec:    exec,
		FS:      vfs.NewOSFS(),
		Env:     map[string]string{},
		Writers: &writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
	}
	l := logger.CreateLogger()

	opts := newRunE2EOpts(t, s, "plan")
	opts.AutoRetry = false

	err := run.Run(t.Context(), l, v, opts, report.NewReport(), &runcfg.RunConfig{}, creds.NewGetter())
	require.Error(t, err, "non-zero terraform exit must surface from run.Run")
}

// TestRunPipelineEndToEndFiresHooks pins the contract that before and
// after hooks fire in the right order around the terraform command.
// Combined with the e2e plan test, this confirms the hook system is
// reachable through the full run.Run pipeline rather than only through
// the unit tests at the ProcessHooks layer.
func TestRunPipelineEndToEndFiresHooks(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)

	var dispatched []string

	mu := sync.Mutex{}

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		mu.Lock()
		defer mu.Unlock()

		dispatched = append(dispatched, inv.Name)

		if inv.Name == "tofu" {
			return vexec.Result{Stdout: []byte("plan ok\n")}
		}

		return vexec.Result{}
	})

	// FS uses NewOSFS because DownloadTerraformSource still copies real
	// files (the temp scaffolding). The mem backend is only for exec
	// virtualization; fs virtualization remains a future item.
	v := &run.Venv{
		Exec:    exec,
		FS:      vfs.NewOSFS(),
		Env:     map[string]string{},
		Writers: &writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
	}
	l := logger.CreateLogger()

	opts := newRunE2EOpts(t, s, "plan")
	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			BeforeHooks: []runcfg.Hook{
				{Name: "before-plan", Commands: []string{"plan"}, Execute: []string{"step-before"}, If: true},
			},
			AfterHooks: []runcfg.Hook{
				{Name: "after-plan", Commands: []string{"plan"}, Execute: []string{"step-after"}, If: true},
			},
		},
	}

	require.NoError(t, run.Run(t.Context(), l, v, opts, report.NewReport(), cfg, creds.NewGetter()))

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, []string{"step-before", "tofu", "step-after"}, dispatched,
		"before hook must fire, then tofu plan, then after hook")
}

// runE2EScaffold builds the minimal real-filesystem scaffolding required
// for run.Run to traverse its pipeline without spawning real tofu. The
// terraform-data-dir markers (.terraform/providers, .terraform/modules,
// .terraform.lock.hcl) suppress the init path so run.Run goes straight
// to the configured terraform command.
//
// DownloadTerraformSource, CheckFolderContainsTerraformCode, and
// providersNeedInit still use the real filesystem; the mem-exec
// virtualization only intercepts subprocess spawns. Once util/file.go
// is threaded through vfs.FS this can become fs-pure.
type runE2EScaffold struct {
	dir        string
	configPath string
}

func setupRunE2EScaffold(t *testing.T) runE2EScaffold {
	t.Helper()

	dir := t.TempDir()

	// Minimal terragrunt.hcl. run.Run doesn't parse it; the path is
	// only used by Options.CloneWithConfigPath and to derive WorkingDir.
	configPath := filepath.Join(dir, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(configPath, []byte(""), 0o644))

	// .tf file: satisfies CheckFolderContainsTerraformCode.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(""), 0o644))

	// Provider/module markers: keep needsInitRunCfg from forcing an init
	// recursion before the terraform command actually runs.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".terraform", "providers"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".terraform", "modules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".terraform.lock.hcl"), []byte(""), 0o644))

	return runE2EScaffold{dir: dir, configPath: configPath}
}

func newRunE2EOpts(t *testing.T, s runE2EScaffold, command string, extraArgs ...string) *run.Options {
	t.Helper()

	args := iacargs.New(append([]string{command}, extraArgs...)...)

	return &run.Options{
		WorkingDir:                   s.dir,
		RootWorkingDir:               s.dir,
		DownloadDir:                  filepath.Join(s.dir, ".terragrunt-cache"),
		TerragruntConfigPath:         s.configPath,
		OriginalTerragruntConfigPath: s.configPath,
		TerraformCommand:             command,
		TerraformCliArgs:             args,
		TFPath:                       "tofu",
		SourceMap:                    map[string]string{},
		Experiments:                  experiment.NewExperiments(),
		StrictControls:               controls.New(),
		MaxFoldersToCheck:            5,
		Telemetry:                    &telemetry.Options{},
		OriginalIAMRoleOptions:       iam.RoleOptions{},
		IAMRoleOptions:               iam.RoleOptions{},
	}
}

// invocationRecorder is a thread-safe accumulator for mem-exec
// invocations. It records the name and args of each call.
type invocationRecorder struct {
	calls []vexec.Invocation
	mu    sync.Mutex
}

func (r *invocationRecorder) record(inv *vexec.Invocation) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, vexec.Invocation{
		Name: inv.Name,
		Dir:  inv.Dir,
		Args: slices.Clone(inv.Args),
	})
}

func (r *invocationRecorder) snapshot() []vexec.Invocation {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.calls)
}
