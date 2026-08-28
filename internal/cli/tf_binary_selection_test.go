package cli_test

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	tofuBinary      = options.TofuDefaultPath
	terraformBinary = options.TerraformDefaultPath

	tfPathEnvVar           = "TG_TF_PATH"
	deprecatedTFPathEnvVar = "TERRAGRUNT_TFPATH"

	// mockBinDir is where the fake binaries claim to live when a PATH lookup
	// asks.
	mockBinDir = "/mock/bin"

	// fixtureRoot holds every case's units. Each case gets its own in-memory
	// filesystem, so one path serves them all.
	fixtureRoot = "/fixture"
)

// tfBinaryVersions are the banners the fake binaries answer a version probe
// with, keyed by file name. Both sit below the floor for the automatic
// provider cache so a case exercises binary selection alone.
var tfBinaryVersions = map[string]string{
	tofuBinary:      "OpenTofu v1.9.0\n",
	terraformBinary: "Terraform v1.9.0\n",
}

// terraformBinaryPath addresses an installed Terraform by path rather than by
// name. It has to be absolute. Terragrunt resolves a relative --tf-path
// against the working directory, which would put the fixture's own path in
// front of it.
var terraformBinaryPath = filepath.Join(mockBinDir, terraformBinary)

// tfBinaryPathLayout is which of the two binaries a run finds on PATH.
type tfBinaryPathLayout struct {
	want   func(tfBinaryExpectation) string
	name   string
	onPath []string
}

// tfBinaryExpectation names the binary that must execute, one entry per PATH
// layout. Only a case that leaves the choice to Terragrunt varies across the
// four, so most are filled in by alwaysRuns.
type tfBinaryExpectation struct {
	both          string
	tofuOnly      string
	terraformOnly string
	neither       string
}

// bothOnPath is the layout that hides a resolver ignoring the selection.
// OpenTofu is what Terragrunt falls back to, and it is installed, so a run
// gets all the way through the wrong binary.
var bothOnPath = tfBinaryPathLayout{
	name:   "both on PATH",
	onPath: []string{tofuBinary, terraformBinary},
	want:   func(e tfBinaryExpectation) string { return e.both },
}

var tfBinaryPathLayouts = []tfBinaryPathLayout{
	bothOnPath,
	{
		name:   "only tofu on PATH",
		onPath: []string{tofuBinary},
		want:   func(e tfBinaryExpectation) string { return e.tofuOnly },
	},
	{
		name:   "only terraform on PATH",
		onPath: []string{terraformBinary},
		want:   func(e tfBinaryExpectation) string { return e.terraformOnly },
	},
	{
		name:   "neither on PATH",
		onPath: nil,
		want:   func(e tfBinaryExpectation) string { return e.neither },
	},
}

// tfBinarySelection is one way of telling Terragrunt which binary to use.
type tfBinarySelection struct {
	env     map[string]string
	name    string
	rootHCL string
	unitHCL string
	flag    string
	want    tfBinaryExpectation
}

// autoDetected is what Terragrunt picks when nothing selects a binary. It
// prefers OpenTofu and falls back to Terraform.
var autoDetected = tfBinaryExpectation{
	both:          tofuBinary,
	tofuOnly:      tofuBinary,
	terraformOnly: terraformBinary,
	neither:       terraformBinary,
}

// alwaysRuns is the expectation for a case that names a binary outright.
func alwaysRuns(binary string) tfBinaryExpectation {
	return tfBinaryExpectation{
		both:          binary,
		tofuOnly:      binary,
		terraformOnly: binary,
		neither:       binary,
	}
}

var tfBinarySelections = []tfBinarySelection{
	{
		name: "nothing selects a binary",
		want: autoDetected,
	},
	{
		name:    "unit sets tofu",
		unitHCL: tofuBinary,
		want:    alwaysRuns(tofuBinary),
	},
	{
		name:    "unit sets terraform",
		unitHCL: terraformBinary,
		want:    alwaysRuns(terraformBinary),
	},
	{
		name:    "included config sets tofu",
		rootHCL: tofuBinary,
		want:    alwaysRuns(tofuBinary),
	},
	{
		name:    "included config sets terraform",
		rootHCL: terraformBinary,
		want:    alwaysRuns(terraformBinary),
	},
	{
		name:    "unit sets tofu over an included terraform",
		rootHCL: terraformBinary,
		unitHCL: tofuBinary,
		want:    alwaysRuns(tofuBinary),
	},
	{
		name:    "unit sets terraform over an included tofu",
		rootHCL: tofuBinary,
		unitHCL: terraformBinary,
		want:    alwaysRuns(terraformBinary),
	},
	{
		name:    "unit sets a terraform path",
		unitHCL: terraformBinaryPath,
		want:    alwaysRuns(terraformBinaryPath),
	},
	{
		name: "tf-path flag picks tofu",
		flag: tofuBinary,
		want: alwaysRuns(tofuBinary),
	},
	{
		name: "tf-path flag picks terraform",
		flag: terraformBinary,
		want: alwaysRuns(terraformBinary),
	},
	{
		name: "tf-path flag holds a terraform path",
		flag: terraformBinaryPath,
		want: alwaysRuns(terraformBinaryPath),
	},
	{
		name:    "tf-path flag picks tofu over a unit terraform",
		unitHCL: terraformBinary,
		flag:    tofuBinary,
		want:    alwaysRuns(tofuBinary),
	},
	{
		name:    "tf-path flag picks terraform over a unit tofu",
		unitHCL: tofuBinary,
		flag:    terraformBinary,
		want:    alwaysRuns(terraformBinary),
	},
	{
		name: "env var picks tofu",
		env:  map[string]string{tfPathEnvVar: tofuBinary},
		want: alwaysRuns(tofuBinary),
	},
	{
		name: "env var picks terraform",
		env:  map[string]string{tfPathEnvVar: terraformBinary},
		want: alwaysRuns(terraformBinary),
	},
	{
		name:    "env var picks tofu over a unit terraform",
		unitHCL: terraformBinary,
		env:     map[string]string{tfPathEnvVar: tofuBinary},
		want:    alwaysRuns(tofuBinary),
	},
	{
		name:    "env var picks terraform over a unit tofu",
		unitHCL: tofuBinary,
		env:     map[string]string{tfPathEnvVar: terraformBinary},
		want:    alwaysRuns(terraformBinary),
	},
	{
		name: "tf-path flag over the env var",
		flag: terraformBinary,
		env:  map[string]string{tfPathEnvVar: tofuBinary},
		want: alwaysRuns(terraformBinary),
	},
	{
		name: "deprecated env var picks terraform",
		env:  map[string]string{deprecatedTFPathEnvVar: terraformBinary},
		want: alwaysRuns(terraformBinary),
	},
}

// tfBinaryUnit is one unit written into a fixture. Body is appended after the
// preamble that carries the case's include and terraform_binary.
type tfBinaryUnit struct {
	dir  string
	body string
}

// tfBinaryShape is a unit layout and the command run against it. Each shape
// reaches the binary through a different resolver.
type tfBinaryShape struct {
	name       string
	workingDir string
	args       []string
	units      []tfBinaryUnit
}

var tfBinaryShapes = []tfBinaryShape{
	{
		name:       "single unit",
		workingDir: "standalone",
		args:       []string{"run", "init"},
		units:      []tfBinaryUnit{{dir: "standalone"}},
	},
	{
		// `version` short-circuits ahead of the rest of the run and resolves
		// the binary on its own.
		name:       "single unit version command",
		workingDir: "standalone",
		args:       []string{"run", "version"},
		units:      []tfBinaryUnit{{dir: "standalone"}},
	},
	{
		name:       "run all",
		workingDir: ".",
		args:       []string{"run", "--all", "init"},
		units:      []tfBinaryUnit{{dir: "first"}, {dir: "second"}},
	},
	{
		name:       "run all resolving dependency outputs",
		workingDir: ".",
		args:       []string{"run", "--all", "init"},
		units: []tfBinaryUnit{
			{dir: "dependency"},
			{dir: "dependent", body: `
dependency "upstream" {
  config_path = "../dependency"
}

inputs = {
  foo = dependency.upstream.outputs.foo
}
`},
		},
	},
	{
		// A dependency whose backend is declared in a remote_state block is read
		// through that block instead of a full run of the unit.
		name:       "run all resolving dependency outputs through remote state",
		workingDir: ".",
		args:       []string{"run", "--all", "init"},
		units: []tfBinaryUnit{
			{dir: "dependency", body: `
remote_state {
  backend = "local"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }

  config = {
    path = "terraform.tfstate"
  }
}
`},
			{dir: "dependent", body: `
dependency "upstream" {
  config_path = "../dependency"
}

inputs = {
  foo = dependency.upstream.outputs.foo
}
`},
		},
	},
	{
		// The middle unit reads a dependency output from a block that the
		// lightweight parse behind output resolution cannot decode, which sends
		// the downstream unit's resolution of middle down the full-run fallback.
		name:       "run all resolving outputs of a unit that reads outputs",
		workingDir: ".",
		args:       []string{"run", "--all", "init"},
		units: []tfBinaryUnit{
			{dir: "upstream"},
			{dir: "middle", body: `
dependency "upstream" {
  config_path = "../upstream"
}

terraform {
  extra_arguments "from_dependency" {
    commands = ["init"]
    env_vars = {
      FROM_DEPENDENCY = dependency.upstream.outputs.foo
    }
  }
}
`},
			{dir: "downstream", body: `
dependency "middle" {
  config_path = "../middle"
}

inputs = {
  foo = dependency.middle.outputs.foo
}
`},
		},
	},
}

// tfBinaryCase is one cell of the matrix: a PATH layout, a way of selecting a
// binary, and the run it is selected for.
type tfBinaryCase struct {
	name   string
	layout tfBinaryPathLayout
	sel    tfBinarySelection
	shape  tfBinaryShape
}

// want is the binary this case must run its commands through.
func (tc *tfBinaryCase) want() string {
	return tc.layout.want(tc.sel.want)
}

// wantInstalled reports whether the binary this case selects is on PATH. A case
// that selects a binary Terragrunt cannot find fails the run instead.
func (tc *tfBinaryCase) wantInstalled() bool {
	return slices.Contains(tc.layout.onPath, filepath.Base(tc.want()))
}

// tfBinaryMatrix pairs every PATH layout with every selection and every shape.
func tfBinaryMatrix() []tfBinaryCase {
	cases := make([]tfBinaryCase, 0, len(tfBinaryPathLayouts)*len(tfBinarySelections)*len(tfBinaryShapes))

	for _, layout := range tfBinaryPathLayouts {
		for i := range tfBinarySelections {
			sel := tfBinarySelections[i]

			for _, shape := range tfBinaryShapes {
				cases = append(cases, tfBinaryCase{
					// Slashes give the subtest the same nesting -run takes, so a
					// single case is still addressable by layout and selection.
					name:   strings.Join([]string{layout.name, sel.name, shape.name}, "/"),
					layout: layout,
					sel:    sel,
					shape:  shape,
				})
			}
		}
	}

	return cases
}

// TestTFBinarySelectionWithRacing pins which binary Terragrunt spawns across
// every way of selecting one. Nothing holds that choice in one place. A PATH
// probe at startup, the --tf-path flag and its env vars, and the
// terraform_binary attribute all feed it, and the single-unit path, the runner
// pool and dependency output resolution each read the attribute separately. A
// resolver that misses it says nothing and falls back to the auto-detected
// binary, so the only place the miss shows is the name Terragrunt spawns.
// Running the whole CLI in process against an in-memory exec reads that name
// for every combination.
func TestTFBinarySelectionWithRacing(t *testing.T) {
	t.Parallel()

	for _, tc := range tfBinaryMatrix() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assertSelectedBinaryRuns(t, &tc)
		})
	}
}

// assertSelectedBinaryRuns runs tc and checks that the binary it selects is the
// one that ran, and that no other binary ran a command.
func assertSelectedBinaryRuns(t *testing.T, tc *tfBinaryCase) {
	t.Helper()

	want := tc.want()

	_, commands, err := runTFBinaryCase(t, tc)

	var spawned, ranCommands []string

	for _, cmd := range commands {
		spawned = appendOnce(spawned, cmd.name)

		if isTFVersionProbe(cmd) {
			continue
		}

		ranCommands = appendOnce(ranCommands, cmd.name)

		assert.Equal(t, want, cmd.name, "%s ran through %q", strings.Join(cmd.args, " "), cmd.name)
	}

	if !tc.wantInstalled() {
		require.Error(t, err)

		// Terragrunt cannot get past probing a binary that is not installed, so
		// the probe is all the evidence there is that it picked the right one.
		assert.Contains(t, spawned, want)

		return
	}

	require.NoError(t, err)
	assert.Equal(t, []string{want}, ranCommands, "only %q should have run commands", want)
}

// TestTFBinarySelectionIsPerUnitWithRacing pins that each unit in a run brings
// its own binary, including the unit whose outputs another one reads. A
// resolver that carried the binary from the run rather than from the config it
// is resolving would pass this run's units through one binary.
//
// Both assignments are run because either one on its own agrees with the
// auto-detected default for one of the two units, which would let a resolver
// reading the wrong config pass anyway.
func TestTFBinarySelectionIsPerUnitWithRacing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		dependency string
		dependent  string
	}{
		{dependency: tofuBinary, dependent: terraformBinary},
		{dependency: terraformBinary, dependent: tofuBinary},
	}

	for _, tc := range testCases {
		t.Run(tc.dependency+" read by "+tc.dependent, func(t *testing.T) {
			t.Parallel()

			assertEachUnitRunsItsOwnBinary(t, tc.dependency, tc.dependent)
		})
	}
}

// assertEachUnitRunsItsOwnBinary runs a dependent unit over a dependency, each
// with its own terraform_binary, and checks that every command in a unit's
// directory went through that unit's binary.
func assertEachUnitRunsItsOwnBinary(t *testing.T, dependencyBinary, dependentBinary string) {
	t.Helper()

	tc := tfBinaryCase{
		layout: bothOnPath,
		shape: tfBinaryShape{
			workingDir: ".",
			args:       []string{"run", "--all", "init"},
			units: []tfBinaryUnit{
				{dir: "dependency", body: fmt.Sprintf("terraform_binary = %q\n", dependencyBinary)},
				{dir: "dependent", body: fmt.Sprintf(`terraform_binary = %q

dependency "upstream" {
  config_path = "../dependency"
}

inputs = {
  foo = dependency.upstream.outputs.foo
}
`, dependentBinary)},
			},
		},
	}

	root, commands, err := runTFBinaryCase(t, &tc)
	require.NoError(t, err)

	want := map[string]string{"dependency": dependencyBinary, "dependent": dependentBinary}
	ran := groupCommandsByUnit(root, commands, slices.Collect(maps.Keys(want)))

	for unit, binary := range want {
		assert.Equal(t, []string{binary}, ran[unit],
			"unit %q should have run every command through %q", unit, binary)
	}
}

// groupCommandsByUnit maps each unit directory to the binaries that ran a
// command in it. Version probes are left out, and so is any command that ran
// outside every named unit.
func groupCommandsByUnit(root string, commands []tfCommand, units []string) map[string][]string {
	ran := map[string][]string{}

	for _, cmd := range commands {
		if isTFVersionProbe(cmd) {
			continue
		}

		for _, unit := range units {
			if isUnder(cmd.dir, filepath.Join(root, unit)) {
				ran[unit] = appendOnce(ran[unit], cmd.name)
			}
		}
	}

	return ran
}

// isUnder reports whether path is dir or sits inside it.
func isUnder(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)

	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// appendOnce appends name to names unless it is already there. Order is the
// order each name was first seen.
func appendOnce(names []string, name string) []string {
	if slices.Contains(names, name) {
		return names
	}

	return append(names, name)
}

// tfCommand is one command the in-memory exec was asked to run.
type tfCommand struct {
	name string
	dir  string
	args []string
}

// isTFVersionProbe reports whether cmd is Terragrunt reading a binary's version
// rather than running a command the user asked for. Terragrunt probes before
// parsing the config that may name a different binary, so a probe of the
// auto-detected binary is expected even when another binary runs the commands.
func isTFVersionProbe(cmd tfCommand) bool {
	return len(cmd.args) > 0 && (cmd.args[0] == "-version" || cmd.args[0] == "--version")
}

// runTFBinaryCase runs one CLI invocation against fake binaries. It returns the
// fixture root, so a caller can tell which unit a command ran in, every command
// the run spawned, in order, and what the run itself returned.
func runTFBinaryCase(t *testing.T, tc *tfBinaryCase) (string, []tfCommand, error) {
	t.Helper()

	var (
		mu       sync.Mutex
		commands []tfCommand
	)

	// A binary may be named or addressed by path. Either way it is the file at
	// the end that has to be installed.
	installed := func(name string) bool {
		return slices.Contains(tc.layout.onPath, filepath.Base(name))
	}

	memExec := vexec.NewMemExec(
		func(_ context.Context, inv vexec.Invocation) vexec.Result {
			mu.Lock()

			commands = append(commands, tfCommand{
				name: inv.Name,
				dir:  inv.Dir,
				args: slices.Clone(inv.Args),
			})
			mu.Unlock()

			if !installed(inv.Name) {
				return vexec.Result{Err: exec.ErrNotFound}
			}

			if len(inv.Args) == 0 {
				return vexec.Result{}
			}

			switch inv.Args[0] {
			case "-version", "--version", "version":
				return vexec.Result{Stdout: []byte(tfBinaryVersions[filepath.Base(inv.Name)])}
			case "output":
				return vexec.Result{Stdout: []byte(`{"foo":{"sensitive":false,"type":"string","value":"from-dependency"}}`)}
			}

			return vexec.Result{}
		},
		vexec.WithLookPath(func(file string) (string, error) {
			if !installed(file) {
				return "", exec.ErrNotFound
			}

			return filepath.Join(mockBinDir, file), nil
		}),
	)

	env := map[string]string{}
	maps.Copy(env, tc.sel.env)

	v := venvtest.New().WithExec(memExec).WithEnv(env)

	root := writeTFBinaryFixture(t, v.FS, tc)

	args := append([]string{"terragrunt"}, tc.shape.args...)
	args = append(args, "--non-interactive", "--working-dir", filepath.Join(root, tc.shape.workingDir))

	if tc.sel.flag != "" {
		args = append(args, "--tf-path", tc.sel.flag)
	}

	// Every case logs at debug level, which buries the one failure a reader came
	// for. The spawned commands are the record that matters.
	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	err := cli.NewApp(l, options.NewTerragruntOptions(v.Exec), v).RunContext(t.Context(), l, v, args)

	mu.Lock()
	defer mu.Unlock()

	return root, slices.Clone(commands), err
}

// writeTFBinaryFixture lays out the case's units under a temp directory, giving
// each the include and terraform_binary the case selects, and returns the
// directory.
func writeTFBinaryFixture(t *testing.T, fsys vfs.FS, tc *tfBinaryCase) string {
	t.Helper()

	root := fixtureRoot

	if tc.sel.rootHCL != "" {
		writeFixtureFile(t, fsys, filepath.Join(root, "root.hcl"), fmt.Sprintf("terraform_binary = %q\n", tc.sel.rootHCL))
	}

	for _, unit := range tc.shape.units {
		var hcl strings.Builder

		if tc.sel.rootHCL != "" {
			hcl.WriteString("include \"root\" {\n  path = find_in_parent_folders(\"root.hcl\")\n}\n\n")
		}

		if tc.sel.unitHCL != "" {
			fmt.Fprintf(&hcl, "terraform_binary = %q\n\n", tc.sel.unitHCL)
		}

		hcl.WriteString(unit.body)

		dir := filepath.Join(root, unit.dir)
		require.NoError(t, fsys.MkdirAll(dir, 0o755))
		writeFixtureFile(t, fsys, filepath.Join(dir, "terragrunt.hcl"), hcl.String())
		writeFixtureFile(t, fsys, filepath.Join(dir, "main.tf"), "")
	}

	return root
}

func writeFixtureFile(t *testing.T, fsys vfs.FS, path, contents string) {
	t.Helper()

	require.NoError(t, vfs.WriteFile(fsys, path, []byte(contents), 0o644))
}
