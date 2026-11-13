package run_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file drive run.Run repeatedly against a fake tofu and pin which runs
// spawn init: the first one when something is missing, one more after a change init must
// see, and none otherwise.

const (
	nullProviderTF = `terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "~> 3.2"
    }
  }
}

resource "null_resource" "this" {}
`
	randomProviderTF = `terraform {
  required_providers {
    random = {
      source  = "registry.opentofu.org/hashicorp/random"
      version = "~> 3.6"
    }
  }
}

resource "random_pet" "this" {}
`
	moduleCallTF = `module "example" {
  source = "./example"
}
`
	outsideCallTF = `module "outside" {
  source = "../modules/outside"
}
`
	examplesCallTF = `module "example" {
  source = "../.."
}
`
	childOutputTF = `output "ok" {
  value = true
}
`
	autoInitPlanRuns = 3
)

// initArtifacts is what a fake tofu init leaves in the working directory.
type initArtifacts int

const (
	// initInstallsProviders mirrors init for a root module that calls no modules: the lock file
	// and the providers directory appear, and .terraform/modules never does.
	initInstallsProviders initArtifacts = iota
	// initInstallsProvidersAndModules mirrors init for a root module that calls a module.
	initInstallsProvidersAndModules
	// initInstallsModules mirrors init for a root module that calls a module and uses no provider.
	initInstallsModules
	// initInstallsNothing mirrors init for a root module that uses no provider and calls no module:
	// OpenTofu creates neither a lock file nor a .terraform directory.
	initInstallsNothing
)

func (a initArtifacts) providers() bool {
	return a == initInstallsProviders || a == initInstallsProvidersAndModules
}

func (a initArtifacts) modules() bool {
	return a == initInstallsProvidersAndModules || a == initInstallsModules
}

func TestRunAutoInitSpawnsInitOnlyWhenSomethingIsMissing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files               map[string]string
		editAfterFirstRun   map[string]string
		name                string
		source              string
		want                []string
		removeAfterFirstRun []string
		localModules        []string
		artifacts           initArtifacts
		inPlace             bool
		providersInstalled  bool
		autoInitDisabled    bool
		explicitInit        bool
	}{
		{
			// Regression test for https://github.com/gruntwork-io/terragrunt/issues/7220
			name: "module block only in an examples subdirectory",
			files: map[string]string{
				"main.tf":                nullProviderTF,
				"examples/basic/main.tf": examplesCallTF,
			},
			want: []string{tf.CommandNameInit, tf.CommandNamePlan, tf.CommandNamePlan, tf.CommandNamePlan},
		},
		{
			// Regression test for https://github.com/gruntwork-io/terragrunt/issues/7220
			name:   "module block only in an examples subdirectory of a sourced module",
			source: "./src",
			files: map[string]string{
				"src/main.tf":                nullProviderTF,
				"src/examples/basic/main.tf": examplesCallTF,
			},
			want: []string{tf.CommandNameInit, tf.CommandNamePlan, tf.CommandNamePlan, tf.CommandNamePlan},
		},
		{
			name: "commented-out module blocks",
			files: map[string]string{
				"main.tf": nullProviderTF + `
# module "hash" {
#   source = "./example"
# }

/*
module "block" {
  source = "./example"
}
*/
`,
			},
			want: []string{tf.CommandNameInit, tf.CommandNamePlan, tf.CommandNamePlan, tf.CommandNamePlan},
		},
		{
			name: "module text inside a string",
			files: map[string]string{
				"main.tf": `
resource "null_resource" "this" {
  triggers = {
    note = "module \"example\" lives in examples/"
  }
}
`,
			},
			want: []string{tf.CommandNameInit, tf.CommandNamePlan, tf.CommandNamePlan, tf.CommandNamePlan},
		},
		{
			name: "module key nested in a .tf.json resource",
			files: map[string]string{
				"main.tf.json": `{"resource": {"null_resource": {"this": {"triggers": {"module": "example"}}}}}`,
			},
			want: []string{tf.CommandNameInit, tf.CommandNamePlan, tf.CommandNamePlan, tf.CommandNamePlan},
		},
		{
			name: "root module calls a module",
			files: map[string]string{
				"main.tf":         nullProviderTF + moduleCallTF,
				"example/main.tf": childOutputTF,
			},
			artifacts:    initInstallsProvidersAndModules,
			localModules: []string{"example"},
			want:         []string{tf.CommandNameInit, tf.CommandNamePlan, tf.CommandNamePlan, tf.CommandNamePlan},
		},
		{
			name:      "root module uses no provider",
			files:     map[string]string{"main.tf": childOutputTF},
			artifacts: initInstallsNothing,
			want:      []string{tf.CommandNameInit, tf.CommandNamePlan, tf.CommandNamePlan, tf.CommandNamePlan},
		},
		{
			name: "root module uses no provider and calls a module that uses none",
			files: map[string]string{
				"main.tf":         moduleCallTF,
				"example/main.tf": childOutputTF,
			},
			artifacts:    initInstallsModules,
			localModules: []string{"example"},
			want:         []string{tf.CommandNameInit, tf.CommandNamePlan, tf.CommandNamePlan, tf.CommandNamePlan},
		},
		{
			name:  "source change re-inits once",
			files: map[string]string{"main.tf": nullProviderTF},
			editAfterFirstRun: map[string]string{
				"main.tf": nullProviderTF + "\nresource \"null_resource\" \"added\" {}\n",
			},
			want: []string{
				tf.CommandNameInit, tf.CommandNamePlan,
				tf.CommandNameInit, tf.CommandNamePlan,
				tf.CommandNamePlan,
			},
		},
		{
			name: "root module calls a module and providers are installed",
			files: map[string]string{
				"main.tf":         nullProviderTF + moduleCallTF,
				"example/main.tf": childOutputTF,
			},
			inPlace:            true,
			providersInstalled: true,
			artifacts:          initInstallsProvidersAndModules,
			localModules:       []string{"example"},
			want:               []string{tf.CommandNameInit, tf.CommandNamePlan, tf.CommandNamePlan, tf.CommandNamePlan},
		},
		{
			name: "root .tf.json module calls a module and providers are installed",
			files: map[string]string{
				"main.tf.json":    `{"module": {"example": {"source": "./example"}}}`,
				"example/main.tf": childOutputTF,
			},
			inPlace:            true,
			providersInstalled: true,
			artifacts:          initInstallsProvidersAndModules,
			localModules:       []string{"example"},
			want:               []string{tf.CommandNameInit, tf.CommandNamePlan, tf.CommandNamePlan, tf.CommandNamePlan},
		},
		{
			name:               "providers installed and no module calls",
			files:              map[string]string{"main.tf": nullProviderTF},
			inPlace:            true,
			providersInstalled: true,
			want:               []string{tf.CommandNamePlan, tf.CommandNamePlan, tf.CommandNamePlan},
		},
		{
			name:         "explicit init of a root module that uses no provider is remembered",
			files:        map[string]string{"main.tf": childOutputTF},
			inPlace:      true,
			explicitInit: true,
			artifacts:    initInstallsNothing,
			want:         []string{tf.CommandNameInit, tf.CommandNamePlan, tf.CommandNamePlan},
		},
		{
			name:              "in place, root module requires another provider after init",
			files:             map[string]string{"main.tf": nullProviderTF},
			editAfterFirstRun: map[string]string{"main.tf": nullProviderTF + randomProviderTF},
			inPlace:           true,
			want: []string{
				tf.CommandNameInit, tf.CommandNamePlan,
				tf.CommandNameInit, tf.CommandNamePlan,
				tf.CommandNamePlan,
			},
		},
		{
			name: "in place, module inside the unit requires another provider after init",
			files: map[string]string{
				"main.tf":         nullProviderTF + moduleCallTF,
				"example/main.tf": childOutputTF,
			},
			editAfterFirstRun: map[string]string{"example/main.tf": randomProviderTF + childOutputTF},
			inPlace:           true,
			artifacts:         initInstallsProvidersAndModules,
			localModules:      []string{"example"},
			want: []string{
				tf.CommandNameInit, tf.CommandNamePlan,
				tf.CommandNameInit, tf.CommandNamePlan,
				tf.CommandNamePlan,
			},
		},
		{
			name: "in place, module outside the unit requires another provider after init",
			files: map[string]string{
				"main.tf":                    nullProviderTF + outsideCallTF,
				"../modules/outside/main.tf": childOutputTF,
			},
			editAfterFirstRun: map[string]string{"../modules/outside/main.tf": randomProviderTF + childOutputTF},
			inPlace:           true,
			artifacts:         initInstallsProvidersAndModules,
			localModules:      []string{"../modules/outside"},
			want: []string{
				tf.CommandNameInit, tf.CommandNamePlan,
				tf.CommandNameInit, tf.CommandNamePlan,
				tf.CommandNamePlan,
			},
		},
		{
			name:              "in place, lock file changes after init",
			files:             map[string]string{"main.tf": nullProviderTF},
			editAfterFirstRun: map[string]string{tf.TerraformLockFile: "provider \"registry.opentofu.org/hashicorp/null\" {}\n"},
			inPlace:           true,
			want: []string{
				tf.CommandNameInit, tf.CommandNamePlan,
				tf.CommandNameInit, tf.CommandNamePlan,
				tf.CommandNamePlan,
			},
		},
		{
			name:                "in place, providers directory removed after init",
			files:               map[string]string{"main.tf": nullProviderTF},
			removeAfterFirstRun: []string{filepath.Join(".terraform", "providers")},
			inPlace:             true,
			want: []string{
				tf.CommandNameInit, tf.CommandNamePlan,
				tf.CommandNameInit, tf.CommandNamePlan,
				tf.CommandNamePlan,
			},
		},
		{
			name:              "in place, file OpenTofu never reads changes after init",
			files:             map[string]string{"main.tf": nullProviderTF, "README.md": "before"},
			editAfterFirstRun: map[string]string{"README.md": "after"},
			inPlace:           true,
			want:              []string{tf.CommandNameInit, tf.CommandNamePlan, tf.CommandNamePlan, tf.CommandNamePlan},
		},
		{
			name: "auto-init disabled never spawns init",
			files: map[string]string{
				"main.tf":         nullProviderTF + moduleCallTF,
				"example/main.tf": childOutputTF,
			},
			autoInitDisabled: true,
			want:             []string{tf.CommandNamePlan, tf.CommandNamePlan, tf.CommandNamePlan},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewOSFS()
			dir := filepath.Join(t.TempDir(), "unit")
			require.NoError(t, fsys.MkdirAll(dir, 0o755))

			configPath := filepath.Join(dir, "terragrunt.hcl")
			require.NoError(t, vfs.WriteFile(fsys, configPath, nil, 0o644))
			writeUnitFiles(t, fsys, dir, tc.files)

			if tc.providersInstalled {
				require.NoError(t, writeInitArtifacts(fsys, dir, initInstallsProviders, nil))
			}

			rec := &recorder{}
			v := venv.MemVenv().
				WithExec(vexec.NewMemExec(fakeTofu(fsys, rec, tc.artifacts, tc.localModules))).
				WithFS(fsys)
			s := runE2EScaffold{dir: dir, configPath: configPath}

			for i := range autoInitPlanRuns {
				if i == 1 {
					writeUnitFiles(t, fsys, dir, tc.editAfterFirstRun)

					for _, name := range tc.removeAfterFirstRun {
						require.NoError(t, fsys.RemoveAll(filepath.Join(dir, name)))
					}
				}

				command := tf.CommandNamePlan
				if i == 0 && tc.explicitInit {
					command = tf.CommandNameInit
				}

				opts := newRunE2EOpts(t, s, command)
				opts.AutoInit = !tc.autoInitDisabled
				cfg := &runcfg.RunConfig{Terraform: runcfg.TerraformConfig{Source: tc.source}}

				if tc.inPlace {
					require.NoError(t, opts.Experiments.EnableExperiment(experiment.NoDotTerragruntCache))

					cfg.Terraform.NoDotTerragruntCache = true
				}

				require.NoError(
					t,
					run.Run(t.Context(), logger.CreateLogger(), v, opts, report.NewReport(), cfg, creds.NewGetter()),
				)
			}

			assert.Equal(t, tc.want, tofuCommands(rec.snapshot()))
		})
	}
}

func TestRunExplicitInitRemovesInitRequiredFile(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()
	dir := filepath.Join(t.TempDir(), "unit")
	require.NoError(t, fsys.MkdirAll(dir, 0o755))

	configPath := filepath.Join(dir, "terragrunt.hcl")
	require.NoError(t, vfs.WriteFile(fsys, configPath, nil, 0o644))

	initRequiredFile := filepath.Join(dir, run.ModuleInitRequiredFile)
	writeUnitFiles(t, fsys, dir, map[string]string{
		"main.tf":                  nullProviderTF,
		run.ModuleInitRequiredFile: "",
	})

	rec := &recorder{}
	v := venv.MemVenv().
		WithExec(vexec.NewMemExec(fakeTofu(fsys, rec, initInstallsProviders, nil))).
		WithFS(fsys)
	s := runE2EScaffold{dir: dir, configPath: configPath}

	for _, command := range []string{tf.CommandNameInit, tf.CommandNamePlan} {
		opts := newRunE2EOpts(t, s, command)
		opts.AutoInit = true
		require.NoError(t, opts.Experiments.EnableExperiment(experiment.NoDotTerragruntCache))

		cfg := &runcfg.RunConfig{Terraform: runcfg.TerraformConfig{NoDotTerragruntCache: true}}

		require.NoError(
			t,
			run.Run(t.Context(), logger.CreateLogger(), v, opts, report.NewReport(), cfg, creds.NewGetter()),
		)
	}

	assert.NoFileExists(t, initRequiredFile)
	assert.Equal(t, []string{tf.CommandNameInit, tf.CommandNamePlan}, tofuCommands(rec.snapshot()))
}

func writeUnitFiles(t *testing.T, fsys vfs.FS, dir string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, fsys.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
	}
}

// fakeTofu records every invocation and answers init by writing what the real binary leaves
// in the working directory. Other tofu commands succeed without side effects; anything that is
// not tofu fails.
func fakeTofu(fsys vfs.FS, rec *recorder, artifacts initArtifacts, localModules []string) vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		rec.record(&inv)

		if inv.Name != "tofu" {
			return vexec.Result{ExitCode: 127, Stderr: []byte("unexpected invocation: " + inv.Name)}
		}

		if len(inv.Args) == 0 || inv.Args[0] != tf.CommandNameInit {
			return vexec.Result{}
		}

		if err := writeInitArtifacts(fsys, inv.Dir, artifacts, localModules); err != nil {
			return vexec.Result{Err: err}
		}

		return vexec.Result{}
	}
}

// writeInitArtifacts creates in dir the files a tofu init leaves behind, recording localModules,
// relative to dir, in the modules manifest.
func writeInitArtifacts(fsys vfs.FS, dir string, artifacts initArtifacts, localModules []string) error {
	if artifacts.providers() {
		if err := fsys.MkdirAll(filepath.Join(dir, ".terraform", "providers"), 0o755); err != nil {
			return err
		}

		lock := "provider \"registry.opentofu.org/hashicorp/null\" {\n  version = \"3.2.4\"\n}\n"
		if err := vfs.WriteFile(fsys, filepath.Join(dir, tf.TerraformLockFile), []byte(lock), 0o644); err != nil {
			return err
		}
	}

	if !artifacts.modules() {
		return nil
	}

	modulesDir := filepath.Join(dir, ".terraform", "modules")
	if err := fsys.MkdirAll(modulesDir, 0o755); err != nil {
		return err
	}

	manifest, err := modulesManifest(localModules)
	if err != nil {
		return err
	}

	return vfs.WriteFile(fsys, filepath.Join(modulesDir, "modules.json"), manifest, 0o644)
}

// modulesManifest renders the modules.json OpenTofu writes for a root module calling the local
// modules at the given relative directories.
func modulesManifest(localModules []string) ([]byte, error) {
	type entry struct {
		Key    string `json:"Key"`
		Source string `json:"Source"`
		Dir    string `json:"Dir"`
	}

	entries := make([]entry, 0, 1+len(localModules))
	entries = append(entries, entry{Key: "", Source: "", Dir: "."})

	for _, dir := range localModules {
		entries = append(entries, entry{Key: filepath.Base(dir), Source: "./" + dir, Dir: dir})
	}

	return json.Marshal(map[string][]entry{"Modules": entries})
}

// tofuCommands returns the subcommand of every recorded tofu invocation, in order.
func tofuCommands(calls []vexec.Invocation) []string {
	commands := make([]string, 0, len(calls))

	for _, call := range calls {
		if call.Name == "tofu" && len(call.Args) > 0 {
			commands = append(commands, call.Args[0])
		}
	}

	return commands
}
