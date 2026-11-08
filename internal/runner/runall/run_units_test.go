package runall_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

const summaryHeader = "Run Summary"

// runRoot is the stack root every run test uses, absolute on the host OS.
var runRoot = venvtest.Root("/repo")

func TestRun_RejectsDisabledCommands(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"import", "taint", "untaint", "console", "force-unlock"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			opts := newRunOptions(t, command)

			err := runall.Run(t.Context(), logger.CreateLogger(), venvtest.New(), opts)

			var disabled runall.RunAllDisabledErr

			require.ErrorAs(t, err, &disabled)
			assert.Contains(t, disabled.Error(), command, "the error should name the rejected command")
		})
	}
}

func TestRun_SummaryFollowsOutputMode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		configure   func(opts *options.TerragruntOptions)
		name        string
		command     string
		args        []string
		wantSummary bool
	}{
		{
			name:        "plan prints a summary",
			command:     "plan",
			wantSummary: true,
		},
		{
			name:    "disabled summary is not printed",
			command: "plan",
			configure: func(opts *options.TerragruntOptions) {
				opts.SummaryDisable = true
			},
		},
		{
			name:    "output skips the summary",
			command: "output",
		},
		{
			name:    "json output skips the summary",
			command: "plan",
			args:    []string{"-json"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &tofuRecorder{}
			v, out := newRunVenv(t, rec, oneUnit())

			opts := newRunOptions(t, tc.command, tc.args...)
			if tc.configure != nil {
				tc.configure(opts)
			}

			require.NoError(t, runall.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, opts))

			require.True(t, rec.ran(tc.command), "the %s command should run in the unit", tc.command)
			assert.Equal(t, tc.wantSummary, strings.Contains(out.String(), summaryHeader))
		})
	}
}

func TestRun_ReturnsUnitFailure(t *testing.T) {
	t.Parallel()

	rec := &tofuRecorder{fail: "plan"}
	v, out := newRunVenv(t, rec, oneUnit())

	err := runall.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, newRunOptions(t, "plan"))

	var execErr *util.ProcessExecutionError

	require.ErrorAs(t, err, &execErr, "a failing unit should fail the run")
	assert.Contains(t, out.String(), summaryHeader, "the summary should still be printed after a failure")
}

func TestRun_DeclinedConfirmationCancels(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"apply", "destroy", "state"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			rec := &tofuRecorder{}
			v, out := newRunVenv(t, rec, oneUnit())
			v.Stdin = strings.NewReader("n\n")

			opts := newRunOptions(t, command)
			opts.NonInteractive = false

			err := runall.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, opts)

			require.ErrorIs(t, err, runall.ErrUserCancelled)
			assert.False(t, rec.ran(command), "a declined %s should not reach any unit", command)
			assert.NotContains(t, out.String(), summaryHeader, "a cancelled run should not print a summary")
		})
	}
}

func TestRun_ConfirmationPromptFailureFails(t *testing.T) {
	t.Parallel()

	rec := &tofuRecorder{}
	v, _ := newRunVenv(t, rec, oneUnit())
	v.Writers.ErrWriter = failingWriter{}

	opts := newRunOptions(t, "apply")
	opts.NonInteractive = false

	err := runall.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, opts)

	require.ErrorIs(t, err, errWrite, "a prompt that cannot be shown should fail the run")
	assert.False(t, rec.ran("apply"), "no unit should run without confirmation")
}

func TestRun_WritesReportFiles(t *testing.T) {
	t.Parallel()

	reportFile := filepath.Join(runRoot, "report.json")
	schemaFile := filepath.Join(runRoot, "report.schema.json")

	rec := &tofuRecorder{}
	v, _ := newRunVenv(t, rec, oneUnit())

	opts := newRunOptions(t, "plan")
	opts.ReportFile = reportFile
	opts.ReportSchemaFile = schemaFile
	opts.ReportFormat = "json"
	opts.SummaryPerUnit = true

	require.NoError(t, runall.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, opts))

	reportData, err := vfs.ReadFile(v.FS, reportFile)
	require.NoError(t, err, "the report should be written")

	schemaData, err := vfs.ReadFile(v.FS, schemaFile)
	require.NoError(t, err, "the schema should be written")

	var schema struct {
		Type string `json:"type"`
	}

	require.NoError(t, json.Unmarshal(schemaData, &schema))
	assert.Equal(t, "array", schema.Type, "the schema should describe a list of runs")

	result, err := gojsonschema.Validate(gojsonschema.NewBytesLoader(schemaData), gojsonschema.NewBytesLoader(reportData))
	require.NoError(t, err)
	assert.True(t, result.Valid(), "the report should match its schema: %v", result.Errors())

	var runs []report.JSONRun

	require.NoError(t, json.Unmarshal(reportData, &runs))
	require.Len(t, runs, 1, "the report should hold the one unit run")
	assert.Equal(t, "unit", runs[0].Name)
	assert.Equal(t, string(report.ResultSucceeded), runs[0].Result)
}

func TestRun_StackGeneration(t *testing.T) {
	t.Parallel()

	generated := filepath.Join(runRoot, ".terragrunt-stack", "vpc", "terragrunt.hcl")

	files := map[string]string{
		config.DefaultStackFile: `
unit "vpc" {
  source = "./units/vpc"
  path   = "vpc"
}
`,
		"units/vpc/terragrunt.hcl": "",
		"units/vpc/main.tf":        "",
	}

	testCases := []struct {
		name          string
		noGenerate    bool
		wantGenerated bool
	}{
		{
			name:          "stacks are generated before the run",
			wantGenerated: true,
		},
		{
			name:       "generation can be skipped",
			noGenerate: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &tofuRecorder{}
			v, _ := newRunVenv(t, rec, files)

			opts := newRunOptions(t, "plan")
			opts.NoStackGenerate = tc.noGenerate

			require.NoError(t, runall.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, opts))

			exists, err := vfs.FileExists(v.FS, generated)
			require.NoError(t, err)
			assert.Equal(t, tc.wantGenerated, exists)
			assert.Equal(
				t,
				tc.wantGenerated,
				rec.ranUnder("plan", filepath.Dir(generated)),
				"the generated unit should be planned",
			)
		})
	}
}

func TestRun_InvalidStackFileFails(t *testing.T) {
	t.Parallel()

	rec := &tofuRecorder{}
	v, _ := newRunVenv(t, rec, map[string]string{config.DefaultStackFile: `unit "vpc" {`})

	err := runall.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, newRunOptions(t, "plan"))

	require.Error(t, err, "an invalid stack file should fail the run")
	assert.False(t, rec.ran("plan"), "no unit should run when generation fails")
}

func TestRun_SourceUpdateCleansGeneratedStacks(t *testing.T) {
	t.Parallel()

	stale := filepath.Join(runRoot, "old", ".terragrunt-stack", "gone", "main.tf")

	rec := &tofuRecorder{}
	v, _ := newRunVenv(t, rec, map[string]string{"old/.terragrunt-stack/gone/main.tf": ""})

	exists, err := vfs.FileExists(v.FS, stale)
	require.NoError(t, err)
	require.True(t, exists, "the stale stack directory should be seeded")

	opts := newRunOptions(t, "plan")
	opts.SourceUpdate = true

	require.NoError(t, runall.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, opts))

	exists, err = vfs.FileExists(v.FS, stale)
	require.NoError(t, err)
	assert.False(t, exists, "--source-update should remove generated stack directories")
}

func TestRun_SourceUpdateCleanFailureFails(t *testing.T) {
	t.Parallel()

	opts := newRunOptions(t, "plan")
	opts.SourceUpdate = true
	opts.WorkingDir = "/missing"

	err := runall.Run(config.WithCaches(t.Context()), logger.CreateLogger(), venvtest.New(), opts)

	require.ErrorIs(t, err, fs.ErrNotExist, "a failed clean should stop the run")
}

func TestRun_GitFilterNeedsGit(t *testing.T) {
	t.Parallel()

	gitFilter, err := filter.Parse("[main...HEAD]")
	require.NoError(t, err)

	opts := newRunOptions(t, "plan")
	opts.Filters = filter.Filters{gitFilter}

	err = runall.Run(config.WithCaches(t.Context()), logger.CreateLogger(), venvtest.New(), opts)

	require.ErrorIs(t, err, vexec.ErrNoSpawn, "a git filter should build worktrees with git first")
}

// errWrite is the error failingWriter reports.
var errWrite = errors.New("write failed")

// failingWriter is a writer whose writes always fail.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWrite
}

// lockedWriter collects output from concurrent writers.
type lockedWriter struct {
	buf strings.Builder
	mu  sync.Mutex
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.buf.Write(p)
}

func (w *lockedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.buf.String()
}

// tofuRecorder answers tofu invocations and records the commands it saw.
type tofuRecorder struct {
	fail     string
	commands []string
	dirs     []string
	mu       sync.Mutex
}

// handler returns the exec handler that answers and records each invocation.
func (rec *tofuRecorder) handler() vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if len(inv.Args) == 0 {
			return vexec.Result{}
		}

		command := inv.Args[0]

		rec.mu.Lock()
		rec.commands = append(rec.commands, command)
		rec.dirs = append(rec.dirs, inv.Dir)
		rec.mu.Unlock()

		if command == "-version" {
			return vexec.Result{Stdout: []byte("OpenTofu v1.9.0\non linux_amd64\n")}
		}

		if command == rec.fail {
			return vexec.Result{ExitCode: 1, Stderr: []byte(command + " failed\n")}
		}

		return vexec.Result{Stdout: []byte("{}")}
	}
}

func (rec *tofuRecorder) ran(command string) bool {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	return slices.Contains(rec.commands, command)
}

// ranUnder reports whether command ran in dir or a directory below it.
func (rec *tofuRecorder) ranUnder(command, dir string) bool {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	prefix := filepath.Clean(dir) + string(filepath.Separator)

	for i, recorded := range rec.commands {
		if recorded == command && strings.HasPrefix(filepath.Clean(rec.dirs[i])+string(filepath.Separator), prefix) {
			return true
		}
	}

	return false
}

// oneUnit is a working directory holding a single unit.
func oneUnit() map[string]string {
	return map[string]string{"unit/terragrunt.hcl": "", "unit/main.tf": ""}
}

func newRunVenv(t *testing.T, rec *tofuRecorder, files map[string]string) (*venv.Venv, *lockedWriter) {
	t.Helper()

	out := &lockedWriter{}

	v := venvtest.New().
		WithExec(vexec.NewMemExec(rec.handler())).
		WithFS(venvtest.NewFS(t, runRoot, files))
	v.Writers.Writer = out

	return v, out
}

func newRunOptions(t *testing.T, command string, args ...string) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(runRoot, config.DefaultTerragruntConfigPath))
	require.NoError(t, err)

	opts.WorkingDir = runRoot
	opts.TFPath = "tofu"
	opts.TofuCommand = command
	opts.TerraformCliArgs = iacargs.New(append([]string{command}, args...)...)

	return opts
}
