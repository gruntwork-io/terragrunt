package scaffold_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/pkg/config"
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
