package scaffold_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const vpcModule = `variable "name" {
  type = string
}
`

func TestScaffoldRelativeOutputFolderLandsUnderWorkingDir(t *testing.T) {
	t.Parallel()

	source := writeComponent(t, "modules/vpc", map[string]string{"main.tf": vpcModule})
	workingDir := helpers.TmpDirWOSymlinks(t)

	opts := newScaffoldOptions(t, workingDir)
	opts.ScaffoldOutputFolder = filepath.Join("live", "vpc")

	runScaffoldWithOptions(t, opts, source)

	assert.FileExists(t, filepath.Join(workingDir, "live", "vpc", "terragrunt.hcl"))
	assert.NoFileExists(t, filepath.Join(workingDir, "terragrunt.hcl"))
}

func TestScaffoldAbsoluteOutputFolderIsKept(t *testing.T) {
	t.Parallel()

	source := writeComponent(t, "modules/vpc", map[string]string{"main.tf": vpcModule})
	workingDir := helpers.TmpDirWOSymlinks(t)
	outputDir := helpers.TmpDirWOSymlinks(t)

	opts := newScaffoldOptions(t, workingDir)
	opts.ScaffoldOutputFolder = outputDir

	runScaffoldWithOptions(t, opts, source)

	assert.FileExists(t, filepath.Join(outputDir, "terragrunt.hcl"))
	assert.NoFileExists(t, filepath.Join(workingDir, "terragrunt.hcl"))
}

// TestScaffoldChecksRootFileUnderOutputFolder pins root-terragrunt-hcl for a root file found as terragrunt.hcl.
func TestScaffoldChecksRootFileUnderOutputFolder(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		file     string
		rootFile string
		strict   bool
	}{
		{
			name:     "spelling of terragrunt.hcl under the strict control",
			file:     config.DefaultTerragruntConfigPath,
			rootFile: "TERRAGRUNT.HCL",
			strict:   true,
		},
		{
			name:     "spelling of terragrunt.hcl with the control disabled",
			file:     config.DefaultTerragruntConfigPath,
			rootFile: "TERRAGRUNT.HCL",
		},
		{
			name:     "terragrunt.hcl set without the flag under the strict control",
			file:     config.DefaultTerragruntConfigPath,
			rootFile: config.DefaultTerragruntConfigPath,
			strict:   true,
		},
		{name: "separate file under the strict control", file: "TERRAGRUNT.HCL", rootFile: "TERRAGRUNT.HCL", strict: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source := writeComponent(t, "modules/vpc", map[string]string{"main.tf": vpcModule})
			fsys := vfs.NewOSFS()
			root := helpers.TmpDirWOSymlinks(t)
			defaultConfig := filepath.Join(root, config.DefaultTerragruntConfigPath)
			otherCase := filepath.Join(root, "TERRAGRUNT.HCL")

			writeFile(t, filepath.Join(root, tc.file), "")

			if !vfs.Exists(fsys, otherCase) {
				require.NoError(t, vfs.Symlink(fsys, defaultConfig, otherCase))
			}

			wantControl := vfs.Exists(fsys, defaultConfig)

			opts := newScaffoldOptions(t, helpers.TmpDirWOSymlinks(t))
			opts.ScaffoldOutputFolder = filepath.Join(root, "unit")
			opts.ScaffoldRootFileName = tc.rootFile

			if tc.strict {
				require.NoError(t, opts.StrictControls.EnableControl(controls.RootTerragruntHCL))
			}

			ctrl, ok := opts.StrictControls.Find(controls.RootTerragruntHCL).(*controls.Control)
			require.True(t, ok, "control %s should be a *controls.Control", controls.RootTerragruntHCL)

			var out bytes.Buffer

			err := scaffold.Run(
				config.WithCaches(t.Context()),
				logger.CreateLogger().WithOptions(log.WithOutput(&out)),
				venvtest.NewOSWithEmptyEnv(),
				opts,
				source,
				"",
			)

			if wantControl && tc.strict {
				require.ErrorIs(t, err, ctrl.Error, "the enabled control should reject a terragrunt.hcl root")
				assert.NoFileExists(t, filepath.Join(opts.ScaffoldOutputFolder, config.DefaultTerragruntConfigPath))

				return
			}

			require.NoError(t, err)
			assert.FileExists(t, filepath.Join(opts.ScaffoldOutputFolder, config.DefaultTerragruntConfigPath))
			assert.Equal(
				t,
				wantControl,
				bytes.Contains(out.Bytes(), []byte(ctrl.Warning)),
				"the disabled control should warn only for a terragrunt.hcl root",
			)
		})
	}
}

func TestScaffoldVarFileIsReadFromWorkingDir(t *testing.T) {
	t.Parallel()

	source := writeComponent(t, "modules/vpc", map[string]string{"main.tf": vpcModule})
	workingDir := helpers.TmpDirWOSymlinks(t)
	writeFile(t, filepath.Join(workingDir, "vars.yml"), "RootFileName: from-var-file.hcl\n")

	opts := newScaffoldOptions(t, workingDir)
	opts.ScaffoldVarFiles = []string{"vars.yml"}

	runScaffoldWithOptions(t, opts, source)

	got := readFile(t, filepath.Join(workingDir, "terragrunt.hcl"))
	assert.Contains(t, got, `find_in_parent_folders("from-var-file.hcl")`)
}

// TestScaffoldRelativeLocalSourceIsWrittenRelativeToOutputDir pins that a
// module named by a path relative to the working directory is written into
// the unit relative to where the unit lands, so running the unit finds it.
func TestScaffoldRelativeLocalSourceIsWrittenRelativeToOutputDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		moduleURL    string
		outputFolder string
		want         string
	}{
		{
			name:      "output folder is the working directory",
			moduleURL: "./modules/vpc",
			want:      `source = "./modules/vpc"`,
		},
		{
			name:         "output folder below the working directory",
			moduleURL:    "./modules/vpc",
			outputFolder: filepath.Join("live", "vpc"),
			want:         `source = "../../modules/vpc"`,
		},
		{
			name:         "subdirectory selector is kept",
			moduleURL:    "./modules//vpc",
			outputFolder: filepath.Join("live", "vpc"),
			want:         `source = "../../modules//vpc"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			workingDir := helpers.TmpDirWOSymlinks(t)
			writeFile(t, filepath.Join(workingDir, "modules", "vpc", "main.tf"), vpcModule)

			opts := newScaffoldOptions(t, workingDir)
			opts.ScaffoldOutputFolder = tc.outputFolder

			runScaffoldWithOptions(t, opts, tc.moduleURL)

			got := readFile(t, filepath.Join(scaffold.OutputDir(opts), "terragrunt.hcl"))
			assert.Contains(t, got, tc.want)
		})
	}
}

func TestOutputDir(t *testing.T) {
	t.Parallel()

	work := helpers.TmpDirWOSymlinks(t)
	elsewhere := helpers.TmpDirWOSymlinks(t)

	testCases := []struct {
		name         string
		outputFolder string
		want         string
	}{
		{
			name: "unset is the working directory",
			want: work,
		},
		{
			name:         "relative is taken from the working directory",
			outputFolder: filepath.FromSlash("live/vpc"),
			want:         filepath.Join(work, "live", "vpc"),
		},
		{
			name:         "absolute is kept",
			outputFolder: elsewhere + string(filepath.Separator),
			want:         elsewhere,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest("")
			require.NoError(t, err)

			opts.WorkingDir = work
			opts.ScaffoldOutputFolder = tc.outputFolder

			assert.Equal(t, tc.want, scaffold.OutputDir(opts))
		})
	}
}

// newScaffoldOptions returns options for scaffolding non-interactively from
// workingDir.
func newScaffoldOptions(t *testing.T, workingDir string) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(workingDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = workingDir
	opts.NonInteractive = true

	return opts
}

// runScaffoldWithOptions scaffolds source with opts and requires it to succeed.
func runScaffoldWithOptions(t *testing.T, opts *options.TerragruntOptions, source string) {
	t.Helper()

	ctx := config.WithCaches(t.Context())

	require.NoError(
		t,
		scaffold.Run(ctx, logger.CreateLogger(), venvtest.NewOSWithEmptyEnv(), opts, source, ""),
	)
}
