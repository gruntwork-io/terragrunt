package run_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	runcommand "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/semver"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tfVersionOutput is what the fake binary prints for a version probe.
const tfVersionOutput = "OpenTofu v1.9.0\n"

// dependentUnits is a git repository where app depends on vpc.
var dependentUnits = map[string]string{
	".git/HEAD":          "ref: refs/heads/main\n",
	"vpc/terragrunt.hcl": "",
	"vpc/main.tf":        "",
	"app/terragrunt.hcl": "dependencies {\n  paths = [\"../vpc\"]\n}\n",
	"app/main.tf":        "",
}

var (
	memRoot = venvtest.Root("/run")

	errWrite = errors.New("write refused")
)

// TestRunRunsTheCommandInTheUnit pins that a single-unit run initializes and runs the command in the unit cache.
func TestRunRunsTheCommandInTheUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		hcl        string
		wantBinary string
	}{
		{name: "default binary", wantBinary: "tofu"},
		{name: "binary from the config", hcl: "terraform_binary = \"custom-tofu\"\n", wantBinary: "custom-tofu"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			v := rec.venv(t, map[string]string{"unit/terragrunt.hcl": tc.hcl, "unit/main.tf": ""})

			require.NoError(
				t,
				runcommand.Run(newCtx(t), logger.CreateLogger(), newOpts(t, v, "unit", tf.CommandNamePlan), v, new(rewrite.Store)),
			)

			assert.Equal(t, [][]string{{tf.CommandNameInit, "-no-color"}, {tf.CommandNamePlan}}, rec.commands())

			plan := rec.find(tf.CommandNamePlan)
			require.NotNil(t, plan)
			assert.Equal(t, tc.wantBinary, plan.Name)
			assert.True(
				t,
				strings.HasPrefix(plan.Dir, filepath.Join(memRoot, "unit", ".terragrunt-cache")+string(filepath.Separator)),
				"plan runs in the download dir: %s", plan.Dir,
			)
		})
	}
}

// TestRunWritesTheRequestedReports pins the report and report schema files a single-unit run writes.
func TestRunWritesTheRequestedReports(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		wantResult report.Result
		exitCode   int
		wantErr    bool
	}{
		{name: "successful run", wantResult: report.ResultSucceeded},
		{name: "failed run", exitCode: 1, wantResult: report.ResultFailed, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{failures: map[string]int{tf.CommandNamePlan: tc.exitCode}}
			v := rec.venv(t, map[string]string{"unit/terragrunt.hcl": "", "unit/main.tf": ""})

			reportPath := filepath.Join(memRoot, "report.json")
			schemaPath := filepath.Join(memRoot, "schema.json")

			opts := newOpts(t, v, "unit", tf.CommandNamePlan)
			opts.ReportFile = reportPath
			opts.ReportFormat = report.FormatJSON
			opts.ReportSchemaFile = schemaPath

			err := runcommand.Run(newCtx(t), logger.CreateLogger(), opts, v, new(rewrite.Store))
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			data, err := vfs.ReadFile(v.FS, reportPath)
			require.NoError(t, err)

			runs, err := report.ParseJSONRuns(data)
			require.NoError(t, err)
			require.Len(t, runs, 1)
			assert.Equal(t, string(tc.wantResult), runs[0].Result)

			exists, err := vfs.FileExists(v.FS, schemaPath)
			require.NoError(t, err)
			assert.True(t, exists, "the report schema is written")
		})
	}
}

// TestRunSkipsUnwritableReports pins that reports that cannot be written do not fail the run.
func TestRunSkipsUnwritableReports(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	v := rec.venv(t, map[string]string{"unit/terragrunt.hcl": "", "unit/main.tf": ""})

	reportPath := filepath.Join(memRoot, "report.yaml")

	opts := newOpts(t, v, "unit", tf.CommandNamePlan)
	opts.ReportFile = reportPath
	opts.ReportFormat = "yaml"
	opts.ReportSchemaFile = filepath.Join(memRoot, "unit")

	require.NoError(t, runcommand.Run(newCtx(t), logger.CreateLogger(), opts, v, new(rewrite.Store)))
	assert.NotNil(t, rec.find(tf.CommandNamePlan), "the run went ahead")

	exists, err := vfs.FileExists(v.FS, reportPath)
	require.NoError(t, err)
	assert.False(t, exists, "an unsupported report format writes nothing")
}

// TestRunFailsWithoutACommand pins the typed error a run with no OpenTofu/Terraform command returns.
func TestRunFailsWithoutACommand(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	v := rec.venv(t, map[string]string{"unit/terragrunt.hcl": "", "unit/main.tf": ""})

	err := runcommand.Run(newCtx(t), logger.CreateLogger(), newOpts(t, v, "unit", ""), v, new(rewrite.Store))

	var missing run.MissingCommand

	require.ErrorAs(t, err, &missing)
	assert.Empty(t, rec.invocations(), "nothing runs without a command")
}

// TestRunVersionUsesTheConfiguredBinary pins which binary `run -- version` asks for its version.
func TestRunVersionUsesTheConfiguredBinary(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files      map[string]string
		name       string
		tfPath     string
		wantBinary string
		explicitTF bool
	}{
		{
			name:       "binary from the config",
			files:      map[string]string{"unit/terragrunt.hcl": "terraform_binary = \"custom-tofu\"\n"},
			wantBinary: "custom-tofu",
		},
		{
			name:       "config without a binary keeps the default",
			files:      map[string]string{"unit/terragrunt.hcl": ""},
			wantBinary: "tofu",
		},
		{
			name:       "missing config keeps the default",
			files:      map[string]string{"unit/main.tf": ""},
			wantBinary: "tofu",
		},
		{
			name:       "explicit binary wins over the config",
			files:      map[string]string{"unit/terragrunt.hcl": "terraform_binary = \"custom-tofu\"\n"},
			tfPath:     "flag-tofu",
			explicitTF: true,
			wantBinary: "flag-tofu",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			v := rec.venv(t, tc.files)

			opts := newOpts(t, v, "unit", tf.CommandNameVersion)
			opts.TFPathExplicitlySet = tc.explicitTF

			if tc.tfPath != "" {
				opts.TFPath = tc.tfPath
			}

			require.NoError(t, runcommand.Run(newCtx(t), logger.CreateLogger(), opts, v, new(rewrite.Store)))

			version := rec.find(tf.CommandNameVersion)
			require.NotNil(t, version, "the version command ran")
			assert.Equal(t, tc.wantBinary, version.Name)
			assert.Len(t, rec.invocations(), 1, "version skips init")
		})
	}
}

// TestRunVersionFailsOnAnInvalidConfig pins that `run -- version` reports a config it cannot parse.
func TestRunVersionFailsOnAnInvalidConfig(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	v := rec.venv(t, map[string]string{"unit/terragrunt.hcl": "this is ) not ( valid hcl"})

	err := runcommand.Run(
		newCtx(t),
		logger.CreateLogger(),
		newOpts(t, v, "unit", tf.CommandNameVersion),
		v,
		new(rewrite.Store),
	)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	assert.Empty(t, rec.invocations())
}

// TestRunRefusesUnmetVersionConstraints pins the typed errors for constraints the binaries do not meet.
func TestRunRefusesUnmetVersionConstraints(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		check func(t *testing.T, err error)
		name  string
		hcl   string
	}{
		{
			name: "OpenTofu/Terraform version constraint",
			hcl:  "terraform_version_constraint = \">= 99.0\"\n",
			check: func(t *testing.T, err error) {
				t.Helper()

				var invalid run.InvalidTerraformVersion

				require.ErrorAs(t, err, &invalid)
			},
		},
		{
			name: "Terragrunt version constraint",
			hcl:  "terragrunt_version_constraint = \">= 99.0\"\n",
			check: func(t *testing.T, err error) {
				t.Helper()

				var invalid run.InvalidTerragruntVersion

				require.ErrorAs(t, err, &invalid)
			},
		},
		{
			name: "unparsable config",
			hcl:  "this is ) not ( valid hcl",
			check: func(t *testing.T, err error) {
				t.Helper()

				var diags hcl.Diagnostics

				require.ErrorAs(t, err, &diags)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			v := rec.venv(t, map[string]string{"unit/terragrunt.hcl": tc.hcl, "unit/main.tf": ""})

			opts := newOpts(t, v, "unit", tf.CommandNamePlan)
			opts.TerragruntVersion = semver.MustParse("1.0.0")

			tc.check(t, runcommand.Run(newCtx(t), logger.CreateLogger(), opts, v, new(rewrite.Store)))
			assert.Nil(t, rec.find(tf.CommandNamePlan), "plan never ran")
		})
	}
}

// TestRunStopsBeforeTheCommand pins the failures that stop a run before OpenTofu/Terraform runs.
func TestRunStopsBeforeTheCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		check      func(t *testing.T, err error)
		name       string
		hcl        string
		authCmd    string
		versionOut string
	}{
		{
			name:    "auth provider command that cannot run",
			authCmd: "no-such-auth-provider",
			check: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, vexec.ErrNoSpawn)
			},
		},
		{
			name:       "binary that reports no version",
			versionOut: "not a version\n",
			check: func(t *testing.T, err error) {
				t.Helper()

				var syntaxErr run.InvalidTerraformVersionSyntax

				require.ErrorAs(t, err, &syntaxErr)
			},
		},
		{
			name: "engine meta that is not an object",
			hcl:  "engine {\n  source = \"engine\"\n  meta   = \"not an object\"\n}\n",
			check: func(t *testing.T, err error) {
				t.Helper()

				var typeErr *json.UnmarshalTypeError

				require.ErrorAs(t, err, &typeErr)
			},
		},
		{
			name: "inputs that fail to evaluate",
			hcl:  "inputs = {\n  name = get_env(\"TG_TEST_RUN_UNSET_VARIABLE\")\n}\n",
			check: func(t *testing.T, err error) {
				t.Helper()

				var diags hcl.Diagnostics

				require.ErrorAs(t, err, &diags)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{versionOutput: tc.versionOut, unspawnable: tc.authCmd}
			v := rec.venv(t, map[string]string{"unit/terragrunt.hcl": tc.hcl, "unit/main.tf": ""})

			opts := newOpts(t, v, "unit", tf.CommandNamePlan)
			opts.AuthProviderCmd = tc.authCmd

			tc.check(t, runcommand.Run(newCtx(t), logger.CreateLogger(), opts, v, new(rewrite.Store)))
			assert.Nil(t, rec.find(tf.CommandNamePlan), "plan never ran")
		})
	}
}

// TestRunDestroyConfirmsDependentUnits pins when destroying a unit other units depend on goes ahead.
func TestRunDestroyConfirmsDependentUnits(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		stdin          string
		writesAllowed  int
		nonInteractive bool
		wantDestroy    bool
	}{
		{name: "non-interactive runs go ahead", nonInteractive: true, wantDestroy: true, writesAllowed: -1},
		{name: "a yes goes ahead", stdin: "y\n", wantDestroy: true, writesAllowed: -1},
		{name: "a no stops", stdin: "n\n", writesAllowed: -1},
		{name: "failing to list the dependents stops"},
		{name: "failing to name a dependent stops", writesAllowed: 1},
		{name: "failing to prompt stops", writesAllowed: 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			errOut := &limitedWriter{remaining: tc.writesAllowed}

			rec := &recorder{}
			v := rec.venv(t, dependentUnits).WithErrWriter(errOut)
			v.Stdin = strings.NewReader(tc.stdin)

			opts := newOpts(t, v, "vpc", tf.CommandNameDestroy)
			opts.DestroyDependenciesCheck = true
			opts.NonInteractive = tc.nonInteractive

			require.NoError(t, runcommand.Run(newCtx(t), logger.CreateLogger(), opts, v, new(rewrite.Store)))
			assert.Equal(t, tc.wantDestroy, rec.find(tf.CommandNameDestroy) != nil, "destroy ran")

			if tc.writesAllowed < 0 {
				assert.Contains(t, errOut.String(), filepath.Join(memRoot, "app"), "the dependent unit is named")
			}
		})
	}
}

// TestRunDestroyWithoutDependentsGoesAhead pins that a destroy with no dependent units needs no confirmation.
func TestRunDestroyWithoutDependentsGoesAhead(t *testing.T) {
	t.Parallel()

	errOut := &limitedWriter{remaining: -1}

	rec := &recorder{}
	v := rec.venv(t, dependentUnits).WithErrWriter(errOut)

	opts := newOpts(t, v, "app", tf.CommandNameDestroy)
	opts.DestroyDependenciesCheck = true
	opts.NonInteractive = false

	require.NoError(t, runcommand.Run(newCtx(t), logger.CreateLogger(), opts, v, new(rewrite.Store)))
	assert.NotNil(t, rec.find(tf.CommandNameDestroy), "destroy ran")
	assert.Empty(t, errOut.String(), "no dependents, no prompt")
}

// TestRunAllAndGraphRunEveryUnit pins that --all and --graph hand the run to the stack runner.
func TestRunAllAndGraphRunEveryUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		workDir string
		all     bool
		graph   bool
	}{
		{name: "all", all: true},
		{name: "graph from the dependency", graph: true, workDir: "vpc"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			v := rec.venv(t, dependentUnits)

			opts := newOpts(t, v, tc.workDir, tf.CommandNamePlan)
			opts.RunAll = tc.all
			opts.Graph = tc.graph
			opts.GraphRoot = memRoot

			require.NoError(t, runcommand.Run(newCtx(t), logger.CreateLogger(), opts, v, new(rewrite.Store)))
			assert.Equal(
				t,
				[]string{filepath.Join(memRoot, "app"), filepath.Join(memRoot, "vpc")},
				rec.unitsRun(tf.CommandNamePlan),
			)
		})
	}
}

// TestRunGivesTheOutputsBlockTip expects a run to tip once a dependency reads a unit that declares no outputs block.
func TestRunGivesTheOutputsBlockTip(t *testing.T) {
	t.Parallel()

	const dependent = `
dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    vpc_id = "vpc-mock"
  }
}
`

	const declaring = `
outputs {
  output "vpc_id" {
    mock_value = "vpc-mock"
  }
}
`

	testCases := []struct {
		name        string
		vpcConfig   string
		workDir     string
		experiments []string
		all         bool
		tipped      bool
	}{
		{
			name:        "a unit run",
			workDir:     "app",
			experiments: []string{experiment.OutputsBlock},
			tipped:      true,
		},
		{
			name:        "a run of every unit",
			experiments: []string{experiment.OutputsBlock},
			all:         true,
			tipped:      true,
		},
		{
			name:        "a dependency that declares its outputs",
			vpcConfig:   declaring,
			workDir:     "app",
			experiments: []string{experiment.OutputsBlock},
		},
		{
			name:    "without the experiment",
			workDir: "app",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			v := rec.venv(t, map[string]string{
				"vpc/terragrunt.hcl": tc.vpcConfig,
				"vpc/main.tf":        "",
				"app/terragrunt.hcl": dependent,
				"app/main.tf":        "",
			})

			opts := newOpts(t, v, tc.workDir, tf.CommandNamePlan)
			opts.RunAll = tc.all

			for _, name := range tc.experiments {
				require.NoError(t, opts.Experiments.EnableExperiment(name))
			}

			require.NoError(t, runcommand.Run(newCtx(t), logger.CreateLogger(), opts, v, new(rewrite.Store)))

			tip := opts.Tips.Find(tips.OutputsBlock)
			require.NotNil(t, tip)

			pending := false

			tip.OnceShow.Do(func() { pending = true })

			assert.Equal(t, tc.tipped, !pending)
		})
	}
}

// recorder answers version probes and records every other command a venv runs.
type recorder struct {
	failures      map[string]int
	versionOutput string
	unspawnable   string
	recorded      []vexec.Invocation
	mu            sync.Mutex
}

// venv returns an in-memory venv holding files under memRoot whose commands rec answers.
func (rec *recorder) venv(t *testing.T, files map[string]string) *venv.Venv {
	t.Helper()

	return venv.MemVenv().
		WithFS(venvtest.NewFS(t, memRoot, files)).
		WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
			if inv.Name == rec.unspawnable {
				return vexec.Result{Err: vexec.ErrNoSpawn}
			}

			if slices.Equal(inv.Args, []string{tf.FlagNameVersion}) {
				if rec.versionOutput != "" {
					return vexec.Result{Stdout: []byte(rec.versionOutput)}
				}

				return vexec.Result{Stdout: []byte(tfVersionOutput)}
			}

			rec.mu.Lock()
			defer rec.mu.Unlock()

			rec.recorded = append(rec.recorded, inv)

			if len(inv.Args) == 0 {
				return vexec.Result{}
			}

			if inv.Args[0] == tf.CommandNameOutput {
				return vexec.Result{Stdout: []byte("{}")}
			}

			return vexec.Result{ExitCode: rec.failures[inv.Args[0]]}
		})
}

// invocations returns every recorded command.
func (rec *recorder) invocations() []vexec.Invocation {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	return slices.Clone(rec.recorded)
}

// commands returns the arguments of every recorded command.
func (rec *recorder) commands() [][]string {
	invocations := rec.invocations()
	commands := make([][]string, 0, len(invocations))

	for _, inv := range invocations {
		commands = append(commands, inv.Args)
	}

	return commands
}

// find returns the first recorded invocation of the OpenTofu/Terraform command, or nil when it never ran.
func (rec *recorder) find(command string) *vexec.Invocation {
	for _, inv := range rec.invocations() {
		if len(inv.Args) > 0 && inv.Args[0] == command {
			return &inv
		}
	}

	return nil
}

// unitsRun returns, sorted, the unit directories the command ran in.
func (rec *recorder) unitsRun(command string) []string {
	var units []string

	for _, inv := range rec.invocations() {
		if len(inv.Args) == 0 || inv.Args[0] != command {
			continue
		}

		units = append(units, unitDir(inv.Dir))
	}

	slices.Sort(units)

	return units
}

// commandsIn returns, in order, the OpenTofu/Terraform commands that ran in the unit directory.
func (rec *recorder) commandsIn(unit string) []string {
	var commands []string

	for _, inv := range rec.invocations() {
		if len(inv.Args) > 0 && unitDir(inv.Dir) == unit {
			commands = append(commands, inv.Args[0])
		}
	}

	return commands
}

// unitDir returns the unit directory of dir, above any .terragrunt-cache copy.
func unitDir(dir string) string {
	dir, _, _ = strings.Cut(dir, string(filepath.Separator)+".terragrunt-cache")

	return dir
}

// limitedWriter accepts a number of writes, then refuses the rest; a negative limit accepts every write.
type limitedWriter struct {
	buf       bytes.Buffer
	remaining int
	mu        sync.Mutex
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.remaining == 0 {
		return 0, errWrite
	}

	if w.remaining > 0 {
		w.remaining--
	}

	return w.buf.Write(p)
}

func (w *limitedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.buf.String()
}

// newCtx returns the test context with the config caches a command run expects.
func newCtx(t *testing.T) context.Context {
	t.Helper()

	return config.WithCaches(t.Context())
}

// newOpts returns options for running command in the unit dir under memRoot, or in memRoot itself.
func newOpts(t *testing.T, v *venv.Venv, unit, command string) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsWithConfigPath(filepath.Join(memRoot, unit, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.TFPath = "tofu"
	opts.NonInteractive = true
	opts.TofuCommand = command
	opts.TerraformCliArgs = iacargs.New()

	if command != "" {
		opts.TerraformCliArgs = iacargs.New(command)
	}

	return opts
}

// runCommand runs cmd with args the way the app does and returns what it printed.
func runCommand(t *testing.T, cmd *clihelper.Command, args ...string) (string, error) {
	t.Helper()

	out := new(bytes.Buffer)

	app := clihelper.NewApp(map[string]string{})
	app.Writer = out
	app.ExitErrHandler = func(_ *clihelper.Context, err error) error { return err }

	err := cmd.Run(newCtx(t), clihelper.NewAppContext(app, args), args)

	return out.String(), err
}
