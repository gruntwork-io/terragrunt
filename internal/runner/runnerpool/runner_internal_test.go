package runnerpool

import (
	"bytes"
	"errors"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestSyncUnitCliArgs(t *testing.T) {
	t.Parallel()

	const root = "/repo"

	planFile := filepath.Join("/out", "unit", tf.TerraformPlanFile)

	testCases := []struct {
		unitDiscoveryCtx *component.DiscoveryContext
		name             string
		command          string
		outputFolder     string
		jsonOutputFolder string
		unitArgs         []string
		stackArgs        []string
		expected         []string
	}{
		{
			name:      "no discovery args clones stack args",
			unitArgs:  []string{"plan", "-lock=false"},
			stackArgs: []string{"plan", "-input=false"},
			command:   tf.CommandNamePlan,
			expected:  []string{"plan", "-input=false"},
		},
		{
			name:             "discovery args merge stack flags",
			unitDiscoveryCtx: &component.DiscoveryContext{Cmd: "plan", Args: []string{"-lock=false"}},
			unitArgs:         []string{"plan", "-lock=false"},
			stackArgs:        []string{"plan", "-input=false"},
			command:          tf.CommandNamePlan,
			expected:         []string{"plan", "-lock=false", "-input=false"},
		},
		{
			name:         "plan command gets out flag",
			unitArgs:     []string{"plan"},
			stackArgs:    []string{"plan"},
			command:      tf.CommandNamePlan,
			outputFolder: "/out",
			expected:     []string{"plan", "-out=" + planFile},
		},
		{
			name:         "show command gets plan file argument",
			unitArgs:     []string{"show"},
			stackArgs:    []string{"show"},
			command:      tf.CommandNameShow,
			outputFolder: "/out",
			expected:     []string{"show", planFile},
		},
		{
			name:             "json output folder yields default plan file",
			unitArgs:         []string{"plan"},
			stackArgs:        []string{"plan"},
			command:          tf.CommandNamePlan,
			jsonOutputFolder: "/json",
			expected:         []string{"plan", "-out=" + tf.TerraformPlanFile},
		},
		{
			name:         "existing plan file is left alone",
			unitArgs:     []string{"plan"},
			stackArgs:    []string{"plan", "-out=existing.tfplan"},
			command:      tf.CommandNamePlan,
			outputFolder: "/out",
			expected:     []string{"plan", "-out=existing.tfplan"},
		},
		{
			name:      "no plan file for other commands",
			unitArgs:  []string{"apply"},
			stackArgs: []string{"apply"},
			command:   tf.CommandNameApply,
			expected:  []string{"apply"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			discoveryCtx := tc.unitDiscoveryCtx
			if discoveryCtx == nil {
				discoveryCtx = &component.DiscoveryContext{}
			}

			discoveryCtx.WorkingDir = root

			unit := component.NewUnit(filepath.Join(root, "unit"))
			unit.SetDiscoveryContext(discoveryCtx)

			stackOpts := &options.TerragruntOptions{
				RootWorkingDir:   root,
				OutputFolder:     tc.outputFolder,
				TerraformCliArgs: iacargs.New(tc.stackArgs...),
			}
			unitOpts := &options.TerragruntOptions{
				TerraformCommand: tc.command,
				JSONOutputFolder: tc.jsonOutputFolder,
				TerraformCliArgs: iacargs.New(tc.unitArgs...),
			}

			syncUnitCliArgs(thlogger.CreateLogger(), stackOpts, unitOpts, unit)

			assert.Equal(t, tc.expected, unitOpts.TerraformCliArgs.Slice())
		})
	}
}

func TestLogTaskOutcome(t *testing.T) {
	t.Parallel()

	l := thlogger.CreateLogger()

	logTaskOutcome(t.Context(), l, "/repo/unit", tf.CommandNamePlan, nil)
	logTaskOutcome(t.Context(), l, "/repo/unit", tf.CommandNamePlan, errors.New("boom"))
}

func TestCollectDependencies(t *testing.T) {
	t.Parallel()

	t.Run("walks the transitive graph", func(t *testing.T) {
		t.Parallel()

		vpc := component.NewUnit("/repo/vpc")
		db := component.NewUnit("/repo/db")
		app := component.NewUnit("/repo/app")

		db.AddDependency(vpc)
		app.AddDependency(db)

		paths := map[string]bool{}
		collectDependencies(app, paths)

		assert.Equal(t, map[string]bool{"/repo/db": true, "/repo/vpc": true}, paths)
	})

	t.Run("tolerates cycles", func(t *testing.T) {
		t.Parallel()

		a := component.NewUnit("/repo/a")
		b := component.NewUnit("/repo/b")

		a.AddDependency(b)
		b.AddDependency(a)

		paths := map[string]bool{}
		collectDependencies(a, paths)

		assert.Equal(t, map[string]bool{"/repo/a": true, "/repo/b": true}, paths)
	})

	t.Run("skips dependencies that are not units", func(t *testing.T) {
		t.Parallel()

		app := component.NewUnit("/repo/app")
		app.AddDependency(component.NewStack("/repo/stack"))

		paths := map[string]bool{}
		collectDependencies(app, paths)

		assert.Empty(t, paths)
	})

	t.Run("stops at the traversal depth cap", func(t *testing.T) {
		t.Parallel()

		const chainLen = maxDependencyTraversalDepth + 10

		units := make([]*component.Unit, chainLen)
		for i := range units {
			units[i] = component.NewUnit("/repo/unit" + strconv.Itoa(i))

			if i > 0 {
				units[i-1].AddDependency(units[i])
			}
		}

		paths := map[string]bool{}
		collectDependencies(units[0], paths)

		assert.Len(t, paths, maxDependencyTraversalDepth)
	})
}

func TestSummarizePlanAllErrors(t *testing.T) {
	t.Parallel()

	const remoteStateErr = "Error running plan: something failed: Resource 'data.terraform_remote_state.vpc' does not have attribute"

	testCases := []struct {
		name       string
		unitConfig *config.TerragruntConfig
		output     string
		withDep    bool
	}{
		{
			name:   "empty buffer is skipped",
			output: "",
		},
		{
			name:   "unrelated output is skipped",
			output: "Error running plan: some unrelated failure",
		},
		{
			name:   "remote state error without dependencies",
			output: remoteStateErr,
		},
		{
			name:    "remote state error with unresolved dependency paths",
			output:  remoteStateErr,
			withDep: true,
		},
		{
			name:    "remote state error with configured dependency paths",
			output:  remoteStateErr,
			withDep: true,
			unitConfig: &config.TerragruntConfig{
				Dependencies: &config.ModuleDependencies{Paths: []string{"../vpc"}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			unit := component.NewUnit("/repo/app")
			if tc.unitConfig != nil {
				unit = unit.WithConfig(tc.unitConfig)
			}

			if tc.withDep {
				unit.AddDependency(component.NewUnit("/repo/vpc"))
			}

			stack := component.NewStack("/repo")
			stack.Units = []*component.Unit{unit}
			rnr := &Runner{Stack: stack}

			buffers := map[string]*bytes.Buffer{unit.Path(): bytes.NewBufferString(tc.output)}
			rnr.summarizePlanAllErrors(thlogger.CreateLogger(), buffers)
		})
	}
}

func TestSummarizePlanAllErrorsSkipsUnitsWithoutBuffer(t *testing.T) {
	t.Parallel()

	stack := component.NewStack("/repo")
	stack.Units = []*component.Unit{component.NewUnit("/repo/app")}

	rnr := &Runner{Stack: stack}

	rnr.summarizePlanAllErrors(thlogger.CreateLogger(), map[string]*bytes.Buffer{})
}

func TestCheckLocalStateWithGitRefs(t *testing.T) {
	t.Parallel()

	withRefAndLocalState := component.NewUnit("/repo/local").WithConfig(&config.TerragruntConfig{
		RemoteState: remotestate.New(&remotestate.Config{BackendName: "local"}),
	})
	withRefAndLocalState.SetDiscoveryContext(&component.DiscoveryContext{Ref: "HEAD~1"})

	withRefAndRemoteState := component.NewUnit("/repo/remote").WithConfig(&config.TerragruntConfig{
		RemoteState: remotestate.New(&remotestate.Config{BackendName: "s3"}),
	})
	withRefAndRemoteState.SetDiscoveryContext(&component.DiscoveryContext{Ref: "HEAD~1"})

	withRefNoConfig := component.NewUnit("/repo/noconfig")
	withRefNoConfig.SetDiscoveryContext(&component.DiscoveryContext{Ref: "HEAD~1"})

	withoutRef := component.NewUnit("/repo/noref").WithConfig(&config.TerragruntConfig{})
	withoutRef.SetDiscoveryContext(&component.DiscoveryContext{})

	noDiscoveryCtx := component.NewUnit("/repo/nodctx").WithConfig(&config.TerragruntConfig{})

	testCases := []struct {
		name  string
		units []*component.Unit
	}{
		{name: "no units"},
		{name: "no discovery context", units: []*component.Unit{noDiscoveryCtx}},
		{name: "no git ref", units: []*component.Unit{withoutRef}},
		{name: "no parsed config", units: []*component.Unit{withRefNoConfig}},
		{name: "remote state configured", units: []*component.Unit{withRefAndRemoteState}},
		{name: "local state warns", units: []*component.Unit{withRefAndLocalState}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			checkLocalStateWithGitRefs(thlogger.CreateLogger(), tc.units)
		})
	}
}

func TestUnitsWithDependentsNilQueue(t *testing.T) {
	t.Parallel()

	assert.Empty(t, UnitsWithDependents(nil))
}

func TestNewRunnerPoolStackIgnoresStackComponents(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("/repo/terragrunt.hcl")
	require.NoError(t, err)

	discovered := component.Components{
		component.NewStack("/repo/stack"),
		component.NewUnit("/repo/vpc").WithConfig(&config.TerragruntConfig{}),
	}

	rnr, err := NewRunnerPoolStack(t.Context(), thlogger.CreateLogger(), opts, discovered)
	require.NoError(t, err)

	units := rnr.GetStack().Units
	require.Len(t, units, 1)
	assert.Equal(t, "/repo/vpc", units[0].Path())
}
