package awsproviderpatch_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	awsproviderpatch "github.com/gruntwork-io/terragrunt/internal/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/tf"
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

// moduleCode is a downloaded module whose AWS provider region the command overrides.
const moduleCode = "provider \"aws\" {\n  region = var.aws_region\n}\n"

// patchedModuleCode is moduleCode after overriding region with "eu-west-1".
const patchedModuleCode = "provider \"aws\" {\n  region = \"eu-west-1\"\n}\n"

// modulesJSON lists the root module and one module downloaded under the data dir.
const modulesJSON = `{"Modules":[{"Key":"","Source":"","Dir":"."},{"Key":"vpc","Source":"./vpc","Dir":".terraform/modules/vpc"}]}`

// regionOverride is the override every successful patch applies.
const regionOverride = `region="eu-west-1"`

var memRoot = venvtest.Root("/patch")

// TestNewCommandPatchesDownloadedModules pins that the command rewrites the AWS provider in modules init downloaded.
func TestNewCommandPatchesDownloadedModules(t *testing.T) {
	t.Parallel()

	fake := &fakeInit{modulesJSON: modulesJSON, moduleFiles: map[string]string{
		"main.tf":          moduleCode,
		"versions.tf.json": `{"terraform":{}}`,
	}}
	v := fake.venv(t, map[string]string{"unit/terragrunt.hcl": "", "unit/main.tf": ""})

	cmd := awsproviderpatch.NewCommand(logger.CreateLogger(), newOpts(t, v, "unit"), v)

	require.NoError(t, runCommand(t, cmd, "--"+awsproviderpatch.OverrideAttrFlagName, regionOverride))
	assert.Equal(t, patchedModuleCode, fake.read(t, v, "main.tf"))
	assert.JSONEq(t, `{"terraform":{}}`, fake.read(t, v, "versions.tf.json"), "JSON files are left alone")
}

// TestNewCommandWithoutModulesChangesNothing pins that a unit whose init downloaded no modules patches nothing.
func TestNewCommandWithoutModulesChangesNothing(t *testing.T) {
	t.Parallel()

	fake := &fakeInit{}
	v := fake.venv(t, map[string]string{"unit/terragrunt.hcl": "", "unit/main.tf": moduleCode})

	cmd := awsproviderpatch.NewCommand(logger.CreateLogger(), newOpts(t, v, "unit"), v)

	require.NoError(t, runCommand(t, cmd, "--"+awsproviderpatch.OverrideAttrFlagName, regionOverride))
	require.Len(t, fake.initDirs(), 1, "init ran")

	data, err := vfs.ReadFile(v.FS, filepath.Join(memRoot, "unit", "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, moduleCode, string(data), "the root module is never patched")
}

// TestNewCommandReportsPatchFailures pins the typed errors for overrides and modules the command cannot patch.
func TestNewCommandReportsPatchFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		check       func(t *testing.T, err error)
		moduleFiles map[string]string
		name        string
		modulesJSON string
		args        []string
	}{
		{
			name:        "no override",
			modulesJSON: modulesJSON,
			moduleFiles: map[string]string{"main.tf": moduleCode},
			check: func(t *testing.T, err error) {
				t.Helper()

				var missing awsproviderpatch.MissingOverrideAttrError

				require.ErrorAs(t, err, &missing)
				assert.Contains(t, err.Error(), "--"+awsproviderpatch.OverrideAttrFlagName, "the message names the flag")
			},
		},
		{
			name:        "override that is not JSON",
			modulesJSON: modulesJSON,
			moduleFiles: map[string]string{"main.tf": moduleCode},
			args:        []string{"--" + awsproviderpatch.OverrideAttrFlagName, "region=eu-west-1"},
			check: func(t *testing.T, err error) {
				t.Helper()

				var inference awsproviderpatch.TypeInferenceError

				require.ErrorAs(t, err, &inference)
				assert.Contains(t, err.Error(), "eu-west-1", "the message names the value to quote")
			},
		},
		{
			name:        "malformed modules.json",
			modulesJSON: "{",
			args:        []string{"--" + awsproviderpatch.OverrideAttrFlagName, regionOverride},
			check: func(t *testing.T, err error) {
				t.Helper()

				var syntaxErr *json.SyntaxError

				require.ErrorAs(t, err, &syntaxErr)
			},
		},
		{
			name:        "module that is not valid HCL",
			modulesJSON: modulesJSON,
			moduleFiles: map[string]string{"main.tf": "this is ) not ( valid hcl"},
			args:        []string{"--" + awsproviderpatch.OverrideAttrFlagName, regionOverride},
			check: func(t *testing.T, err error) {
				t.Helper()

				var diags hcl.Diagnostics

				require.ErrorAs(t, err, &diags)
			},
		},
		{
			name:        "module dir that does not exist",
			modulesJSON: `{"Modules":[{"Key":"gone","Dir":"/patch/gone"}]}`,
			args:        []string{"--" + awsproviderpatch.OverrideAttrFlagName, regionOverride},
			check: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, fs.ErrNotExist)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeInit{modulesJSON: tc.modulesJSON, moduleFiles: tc.moduleFiles}
			v := fake.venv(t, map[string]string{"unit/terragrunt.hcl": "", "unit/main.tf": ""})

			cmd := awsproviderpatch.NewCommand(logger.CreateLogger(), newOpts(t, v, "unit"), v)

			tc.check(t, runCommand(t, cmd, tc.args...))
		})
	}
}

// TestNewCommandStopsBeforePatching pins the preparation failures that stop the command before it patches anything.
func TestNewCommandStopsBeforePatching(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		check    func(t *testing.T, err error)
		files    map[string]string
		name     string
		wantInit bool
	}{
		{
			name:  "invalid config",
			files: map[string]string{"unit/terragrunt.hcl": "this is ) not ( valid hcl", "unit/main.tf": ""},
			check: func(t *testing.T, err error) {
				t.Helper()

				var diags hcl.Diagnostics

				require.ErrorAs(t, err, &diags)
			},
		},
		{
			name:  "source that fails to download",
			files: map[string]string{"unit/terragrunt.hcl": "terraform {\n  source = \"../modules/missing\"\n}\n"},
			check: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, fs.ErrNotExist)
			},
		},
		{
			name: "generate block that refuses to overwrite",
			files: map[string]string{
				"unit/terragrunt.hcl": "generate \"main\" {\n  path      = \"main.tf\"\n" +
					"  if_exists = \"error\"\n  contents  = \"\"\n}\n",
				"unit/main.tf": "",
			},
			check: func(t *testing.T, err error) {
				t.Helper()

				require.Error(t, err)
			},
		},
		{
			name:  "unit without OpenTofu/Terraform code",
			files: map[string]string{"unit/terragrunt.hcl": ""},
			check: func(t *testing.T, err error) {
				t.Helper()

				var noFiles run.NoTerraformFilesFound

				require.ErrorAs(t, err, &noFiles)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeInit{modulesJSON: modulesJSON, moduleFiles: map[string]string{"main.tf": moduleCode}}
			v := fake.venv(t, tc.files)

			cmd := awsproviderpatch.NewCommand(logger.CreateLogger(), newOpts(t, v, "unit"), v)

			tc.check(t, runCommand(t, cmd, "--"+awsproviderpatch.OverrideAttrFlagName, regionOverride))
			assert.Empty(t, fake.initDirs(), "init never ran")
		})
	}
}

// TestNewCommandAllPatchesEveryUnit pins how --all reports units that fail with and without --fail-fast.
func TestNewCommandAllPatchesEveryUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files      map[string]string
		name       string
		wantJoined int
		failFast   bool
		wantErr    bool
	}{
		{
			name: "every unit is patched",
			files: map[string]string{
				"a/terragrunt.hcl": "", "a/main.tf": "",
				"b/terragrunt.hcl": "", "b/main.tf": "",
			},
		},
		{
			name: "failures are joined",
			files: map[string]string{
				"a/terragrunt.hcl": "", "a/main.tf": "",
				"b/terragrunt.hcl": "",
				"c/terragrunt.hcl": "",
			},
			wantErr:    true,
			wantJoined: 2,
		},
		{
			name: "fail fast stops at the first failure",
			files: map[string]string{
				"a/terragrunt.hcl": "",
				"b/terragrunt.hcl": "",
			},
			failFast: true,
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeInit{modulesJSON: modulesJSON, moduleFiles: map[string]string{"main.tf": moduleCode}}
			v := fake.venv(t, tc.files)

			opts := newOpts(t, v, "")
			opts.RunAll = true
			opts.FailFast = tc.failFast
			opts.AwsProviderPatchOverrides = map[string]string{"region": `"eu-west-1"`}

			err := awsproviderpatch.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, opts)
			if !tc.wantErr {
				require.NoError(t, err)
				assert.Len(t, fake.initDirs(), 2, "both units were initialized and patched")

				for _, dir := range fake.initDirs() {
					data, err := vfs.ReadFile(v.FS, filepath.Join(dir, ".terraform", "modules", "vpc", "main.tf"))
					require.NoError(t, err)
					assert.Equal(t, patchedModuleCode, string(data))
				}

				return
			}

			var noFiles run.NoTerraformFilesFound

			require.ErrorAs(t, err, &noFiles)

			var joined interface{ Unwrap() []error }

			if tc.wantJoined == 0 {
				assert.NotErrorAs(t, err, &joined, "fail fast returns the first error unjoined")

				return
			}

			require.ErrorAs(t, err, &joined)
			assert.Len(t, joined.Unwrap(), tc.wantJoined, "one error per failed unit")
		})
	}
}

// TestRunAllReadsTheConfiguredConfigFilename pins that --all reads each unit's config by the configured filename.
func TestRunAllReadsTheConfiguredConfigFilename(t *testing.T) {
	t.Parallel()

	fake := &fakeInit{}
	v := fake.venv(t, map[string]string{
		"a/terragrunt.hcl": "",
		"a/custom.hcl":     "this is ) not ( valid hcl",
		"a/main.tf":        "",
	})

	opts, err := options.NewTerragruntOptionsWithConfigPath(v.Exec, filepath.Join(memRoot, "custom.hcl"))
	require.NoError(t, err)

	opts.NonInteractive = true
	opts.RunAll = true

	err = awsproviderpatch.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, opts)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags, "the unit's custom.hcl is the config that fails to parse")
}

// TestRunAllFailsOnAMissingWorkingDir pins that --all surfaces the discovery failure.
func TestRunAllFailsOnAMissingWorkingDir(t *testing.T) {
	t.Parallel()

	fake := &fakeInit{}
	v := fake.venv(t, nil)

	opts := newOpts(t, v, "")
	opts.WorkingDir = filepath.Join(memRoot, "missing")
	opts.RunAll = true

	err := awsproviderpatch.Run(config.WithCaches(t.Context()), logger.CreateLogger(), v, opts)
	require.ErrorIs(t, err, fs.ErrNotExist)
}

// TestNewCommandFailsUnderTheDeprecatedCommandsControl pins that the deprecated command stops when its control is on.
func TestNewCommandFailsUnderTheDeprecatedCommandsControl(t *testing.T) {
	t.Parallel()

	fake := &fakeInit{modulesJSON: modulesJSON, moduleFiles: map[string]string{"main.tf": moduleCode}}
	v := fake.venv(t, map[string]string{"unit/terragrunt.hcl": "", "unit/main.tf": ""})

	opts := newOpts(t, v, "unit")
	cmd := awsproviderpatch.NewCommand(logger.CreateLogger(), opts, v)

	require.NoError(t, opts.StrictControls.EnableControl(controls.DeprecatedCommands))

	err := runCommand(t, cmd, "--"+awsproviderpatch.OverrideAttrFlagName, regionOverride)

	var exitErr clihelper.ExitCoder

	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, int(clihelper.ExitCodeGeneralError), exitErr.ExitCode())
	assert.Empty(t, fake.initDirs(), "nothing ran")
}

// TestPatchAwsProviderInTerraformCodeSkipsMissingNestedBlocks pins that a nested override leaves a provider without that block alone.
func TestPatchAwsProviderInTerraformCodeSkipsMissingNestedBlocks(t *testing.T) {
	t.Parallel()

	code, updated, err := awsproviderpatch.PatchAwsProviderInTerraformCode(
		moduleCode,
		"main.tf",
		map[string]string{"assume_role.role_arn": `"arn:aws:iam::123456789012:role/test"`},
	)
	require.NoError(t, err)
	assert.False(t, updated)
	assert.Equal(t, moduleCode, code)
}

// fakeInit answers version probes and, on init, writes the modules.json and module files it holds.
type fakeInit struct {
	moduleFiles map[string]string
	modulesJSON string
	dirs        []string
	mu          sync.Mutex
}

// venv returns an in-memory venv holding files under memRoot whose init f fakes.
func (f *fakeInit) venv(t *testing.T, files map[string]string) *venv.Venv {
	t.Helper()

	fsys := venvtest.NewFS(t, memRoot, files)

	return venvtest.New().WithFS(fsys).WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if !slices.Contains(inv.Args, tf.CommandNameInit) {
			return vexec.Result{Stdout: []byte("OpenTofu v1.9.0\n")}
		}

		f.mu.Lock()
		defer f.mu.Unlock()

		f.dirs = append(f.dirs, inv.Dir)

		if f.modulesJSON == "" {
			return vexec.Result{}
		}

		modulesJSONPath := filepath.Join(inv.Dir, ".terraform", "modules", "modules.json")
		if err := vfs.WriteFile(fsys, modulesJSONPath, []byte(f.modulesJSON), 0o644); err != nil {
			return vexec.Result{Err: err}
		}

		for name, contents := range f.moduleFiles {
			path := filepath.Join(inv.Dir, ".terraform", "modules", "vpc", name)
			if err := vfs.WriteFile(fsys, path, []byte(contents), 0o644); err != nil {
				return vexec.Result{Err: err}
			}
		}

		return vexec.Result{}
	})
}

// initDirs returns every directory init ran in.
func (f *fakeInit) initDirs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.dirs)
}

// read returns the module file name as the single init left it after the run.
func (f *fakeInit) read(t *testing.T, v *venv.Venv, name string) string {
	t.Helper()

	dirs := f.initDirs()
	require.Len(t, dirs, 1, "init ran once")

	data, err := vfs.ReadFile(v.FS, filepath.Join(dirs[0], ".terraform", "modules", "vpc", name))
	require.NoError(t, err)

	return string(data)
}

// newOpts returns options for the unit dir under memRoot, or for memRoot itself when unit is empty.
func newOpts(t *testing.T, v *venv.Venv, unit string) *options.TerragruntOptions {
	t.Helper()

	configPath := ""
	if unit != "" {
		configPath = filepath.Join(memRoot, unit, "terragrunt.hcl")
	}

	opts, err := options.NewTerragruntOptionsWithConfigPath(v.Exec, configPath)
	require.NoError(t, err)

	if unit == "" {
		opts.WorkingDir = memRoot
		opts.DownloadDir = filepath.Join(memRoot, ".terragrunt-cache")
	}

	opts.RootWorkingDir = memRoot
	opts.TFPath = "tofu"
	opts.NonInteractive = true

	return opts
}

// runCommand runs cmd with args the way the app does and returns its error.
func runCommand(t *testing.T, cmd *clihelper.Command, args ...string) error {
	t.Helper()

	app := clihelper.NewApp(map[string]string{})
	app.Writer = new(bytes.Buffer)
	app.ExitErrHandler = func(_ *clihelper.Context, err error) error { return err }

	return cmd.Run(config.WithCaches(t.Context()), clihelper.NewAppContext(app, args), args)
}
