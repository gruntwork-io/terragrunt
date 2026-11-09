package scaffold_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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

func TestScaffoldChecksRootFileUnderOutputFolder(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		file     string
		rootFile string
		vars     []string
		want     scaffoldRootOutcome
		strict   bool
	}{
		{
			name:     "differently cased terragrunt.hcl under the strict control",
			file:     config.DefaultTerragruntConfigPath,
			rootFile: otherCaseConfigName,
			strict:   true,
			want:     scaffoldRootRejected,
		},
		{
			name:     "differently cased terragrunt.hcl with the control disabled",
			file:     config.DefaultTerragruntConfigPath,
			rootFile: otherCaseConfigName,
			want:     scaffoldRootWarned,
		},
		{
			name:     "terragrunt.hcl through --var under the strict control",
			file:     config.DefaultTerragruntConfigPath,
			rootFile: config.RecommendedParentConfigName,
			vars:     []string{"RootFileName=" + config.DefaultTerragruntConfigPath},
			strict:   true,
			want:     scaffoldRootRejected,
		},
		{
			name:     "terragrunt.hcl through --var without a root include under the strict control",
			file:     config.DefaultTerragruntConfigPath,
			rootFile: config.RecommendedParentConfigName,
			vars:     []string{"EnableRootInclude=false", "RootFileName=" + config.DefaultTerragruntConfigPath},
			strict:   true,
			want:     scaffoldRootGenerated,
		},
		{
			name:     "terragrunt.hcl through --var with the root include disabled as text under the strict control",
			file:     config.DefaultTerragruntConfigPath,
			rootFile: config.RecommendedParentConfigName,
			vars:     []string{"EnableRootInclude=f", "RootFileName=" + config.DefaultTerragruntConfigPath},
			strict:   true,
			want:     scaffoldRootGenerated,
		},
		{
			name:     "terragrunt.hcl through --var with an empty root include under the strict control",
			file:     config.DefaultTerragruntConfigPath,
			rootFile: config.RecommendedParentConfigName,
			vars:     []string{"EnableRootInclude=", "RootFileName=" + config.DefaultTerragruntConfigPath},
			strict:   true,
			want:     scaffoldRootGenerated,
		},
		{
			name:     "another root file under the strict control",
			file:     config.RecommendedParentConfigName,
			rootFile: config.RecommendedParentConfigName,
			strict:   true,
			want:     scaffoldRootGenerated,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source := writeComponent(t, "modules/vpc", map[string]string{"main.tf": vpcModule})
			fsys := vfs.NewOSFS()
			root := helpers.TmpDirWOSymlinks(t)
			file := filepath.Join(root, tc.file)
			otherCase := filepath.Join(root, otherCaseConfigName)

			writeFile(t, file, "")

			if tc.file == config.DefaultTerragruntConfigPath && !vfs.Exists(fsys, otherCase) {
				require.NoError(t, vfs.Link(fsys, file, otherCase))
			}

			opts := newScaffoldOptions(t, helpers.TmpDirWOSymlinks(t))
			opts.ScaffoldOutputFolder = filepath.Join(root, "unit")
			opts.ScaffoldRootFileName = tc.rootFile
			opts.ScaffoldVars = tc.vars

			if tc.strict {
				require.NoError(t, opts.StrictControls.EnableControl(controls.RootTerragruntHCL))
			}

			ctrl, ok := opts.StrictControls.Find(controls.RootTerragruntHCL).(*controls.Control)
			require.True(t, ok, "control %s should be a *controls.Control", controls.RootTerragruntHCL)

			err := scaffold.Run(
				config.WithCaches(t.Context()),
				logger.CreateLogger(),
				venvtest.NewOSWithEmptyEnv(),
				opts,
				source,
				"",
			)

			if tc.want == scaffoldRootRejected {
				require.ErrorIs(t, err, ctrl.Error, "the enabled control should reject a terragrunt.hcl root")
				assert.NoFileExists(t, filepath.Join(opts.ScaffoldOutputFolder, config.DefaultTerragruntConfigPath))

				return
			}

			require.NoError(t, err)
			assert.FileExists(t, filepath.Join(opts.ScaffoldOutputFolder, config.DefaultTerragruntConfigPath))

			warned := true

			ctrl.OnceWarn.Do(func() { warned = false })

			assert.Equal(
				t,
				tc.want == scaffoldRootWarned,
				warned,
				"the disabled control should warn only for a terragrunt.hcl root",
			)
		})
	}
}

func TestScaffoldRejectsRootTerragruntHCLInMemory(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		rootFile string
		vars     []string
	}{
		{name: "terragrunt.hcl", rootFile: config.DefaultTerragruntConfigPath},
		{name: "differently cased terragrunt.hcl", rootFile: otherCaseConfigName},
		{
			name:     "terragrunt.hcl through --var",
			rootFile: config.RecommendedParentConfigName,
			vars:     []string{"RootFileName=" + config.DefaultTerragruntConfigPath},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/live")
			module := venvtest.Root("/catalog/modules/vpc")

			fsys := venvtest.NewFS(t, root, map[string]string{config.DefaultTerragruntConfigPath: ""})
			require.NoError(t, vfs.WriteFile(fsys, filepath.Join(module, "main.tf"), []byte(vpcModule), 0o644))
			require.NoError(t, vfs.Symlink(
				fsys,
				filepath.Join(root, config.DefaultTerragruntConfigPath),
				filepath.Join(root, otherCaseConfigName),
			))

			opts := newScaffoldOptions(t, venvtest.Root("/work"))
			opts.ScaffoldOutputFolder = filepath.Join(root, "unit")
			opts.ScaffoldRootFileName = tc.rootFile
			opts.ScaffoldVars = tc.vars

			require.NoError(t, opts.StrictControls.EnableControl(controls.RootTerragruntHCL))

			ctrl, ok := opts.StrictControls.Find(controls.RootTerragruntHCL).(*controls.Control)
			require.True(t, ok, "control %s should be a *controls.Control", controls.RootTerragruntHCL)

			err := scaffold.Run(
				config.WithCaches(t.Context()),
				logger.CreateLogger(),
				venvtest.New().WithFS(fsys),
				opts,
				getter.FileURL(module).String(),
				"",
			)
			require.ErrorIs(t, err, ctrl.Error)
			assert.False(t, vfs.Exists(fsys, filepath.Join(opts.ScaffoldOutputFolder, config.DefaultTerragruntConfigPath)))
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

// otherCaseConfigName is terragrunt.hcl in another letter case.
const otherCaseConfigName = "TERRAGRUNT.HCL"

// scaffoldRootOutcome is how scaffold handles the root file its include names.
type scaffoldRootOutcome int

const (
	scaffoldRootGenerated scaffoldRootOutcome = iota
	scaffoldRootWarned
	scaffoldRootRejected
)
