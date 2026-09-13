package scaffold_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// unitConfig is a unit built the way catalog units are: its inputs come from
// `values.*`, so scaffolding it has to leave the configuration alone and write
// the values alongside it.
const unitConfig = `include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${values.base_url}//modules/vpc?ref=${values.ref}"
}

inputs = {
  name   = values.name
  region = try(values.region, "us-east-1")
}
`

const stackConfig = `unit "vpc" {
  source = "${values.base_url}//units/vpc"
  path   = "vpc"
}
`

func TestScaffoldCopiesUnit(t *testing.T) {
	t.Parallel()

	source := writeComponent(t, "units/app", map[string]string{
		"terragrunt.hcl": unitConfig,
		"extra.hcl":      "# carried along\n",
	})

	outputDir := runScaffold(t, source)

	got := readFile(t, filepath.Join(outputDir, "terragrunt.hcl"))
	assert.Equal(t, unitConfig, got, "the unit's own configuration is what the user wants")

	assert.FileExists(t, filepath.Join(outputDir, "extra.hcl"))

	values := readFile(t, filepath.Join(outputDir, "terragrunt.values.hcl"))
	assert.Contains(t, values, `base_url`)
	assert.Contains(t, values, `name`)
	assert.Contains(t, values, `ref`)
	assert.Contains(t, values, `region = "us-east-1"`, "try() fallbacks seed the optional section")
}

func TestScaffoldCopiesStackWithItsSupportingFiles(t *testing.T) {
	t.Parallel()

	source := writeComponent(t, "stacks/prod", map[string]string{
		"terragrunt.stack.hcl": stackConfig,
		"policy.json":          `{"Version": "2012-10-17"}`,
		"README.md":            "# Stack\n",
	})

	outputDir := runScaffold(t, source)

	assert.Equal(t, stackConfig, readFile(t, filepath.Join(outputDir, "terragrunt.stack.hcl")))
	assert.FileExists(t, filepath.Join(outputDir, "policy.json"))
	assert.FileExists(t, filepath.Join(outputDir, "README.md"))
	assert.FileExists(t, filepath.Join(outputDir, "terragrunt.values.hcl"))
}

// TestScaffoldGeneratesForModule pins the module path against the copy path
// taking over: a directory of .tf files still yields a generated
// terragrunt.hcl pointing at it.
func TestScaffoldGeneratesForModule(t *testing.T) {
	t.Parallel()

	source := writeComponent(t, "modules/vpc", map[string]string{
		"main.tf": `variable "name" {
  type = string
}
`,
	})

	outputDir := runScaffold(t, source)

	got := readFile(t, filepath.Join(outputDir, "terragrunt.hcl"))
	assert.Contains(t, got, "terraform {")
	assert.Contains(t, got, source, "the generated source points at the module")
	assert.Contains(t, got, "name")
	assert.NoFileExists(t, filepath.Join(outputDir, "terragrunt.values.hcl"))
}

// TestScaffoldGeneratesForModuleCarryingAUnit pins which marker wins when a
// module directory also holds a terragrunt.hcl, a shape that predates units:
// there are variables to generate from, so scaffolding is what the user meant.
// The catalog reads the same directory as a unit, since that is what it offers
// to browse.
func TestScaffoldGeneratesForModuleCarryingAUnit(t *testing.T) {
	t.Parallel()

	source := writeComponent(t, "modules/vpc", map[string]string{
		"main.tf": `variable "name" {
  type = string
}
`,
		"terragrunt.hcl": "inputs = {\n  name = \"example\"\n}\n",
	})

	outputDir := runScaffold(t, source)

	got := readFile(t, filepath.Join(outputDir, "terragrunt.hcl"))
	assert.Contains(t, got, "terraform {")
	assert.Contains(t, got, "TODO: fill in value", "the module's variables are generated")
	assert.NoFileExists(t, filepath.Join(outputDir, "terragrunt.values.hcl"))
}

// TestGeneratedFilesIncludesBoilerplateDependencyFiles exercises the
// template path with opts.Manifest true: a custom boilerplate template that
// depends on a nested template must list the dependency's generated files
// alongside the root template's outputs.
func TestGeneratedFilesIncludesBoilerplateDependencyFiles(t *testing.T) {
	t.Parallel()

	repoDir := helpers.TmpDirWOSymlinks(t)

	// A module source so Prepare takes the render path, not the copy path.
	writeFile(t, filepath.Join(repoDir, "modules", "vpc", "main.tf"), `variable "name" {
  type = string
}
`)

	writeFile(t, filepath.Join(repoDir, "templates", "root", "boilerplate.yml"), `
dependencies:
  - name: extra
    template-url: ../extra
    output-folder: ./config
`)
	writeFile(t, filepath.Join(repoDir, "templates", "root", "terragrunt.hcl"), `terraform {
  source = "{{ .sourceUrl }}"
}
`)
	writeFile(t, filepath.Join(repoDir, "templates", "extra", "boilerplate.yml"), "variables: []\n")
	writeFile(t, filepath.Join(repoDir, "templates", "extra", "extra.hcl"), `locals {
  from_dependency = true
}
`)

	outputDir := helpers.TmpDirWOSymlinks(t)

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(outputDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = outputDir
	opts.NoShell = true
	opts.NoHooks = true

	v := venvtest.NewOSWithEmptyEnv()
	l := logger.CreateLogger()

	plan, err := scaffold.Prepare(
		t.Context(),
		l,
		v,
		opts,
		repoDir+"//modules/vpc",
		repoDir+"//templates/root",
	)
	require.NoError(t, err)

	t.Cleanup(func() { plan.Cleanup(v.FS) })

	require.NoError(t, plan.Generate(t.Context(), l, v, opts, nil))

	got := plan.GeneratedFiles()
	assert.Contains(t, got, "terragrunt.hcl")
	assert.Contains(t, got, filepath.Join("config", "extra.hcl"),
		"dependency files must be listed relative to the generation root")
	assert.FileExists(t, filepath.Join(outputDir, "config", "extra.hcl"))
	assert.FileExists(t, filepath.Join(outputDir, "terragrunt.hcl"))
}

// TestGeneratedFilesEmptyAfterCopyingUnit pins the GeneratedFiles contract
// for copyable kinds: Generate copies the unit but leaves the list empty,
// because the catalog TUI reports those via CopyCmd.Result.
func TestGeneratedFilesEmptyAfterCopyingUnit(t *testing.T) {
	t.Parallel()

	source := writeComponent(t, "units/app", map[string]string{
		"terragrunt.hcl": unitConfig,
		"extra.hcl":      "# carried along\n",
	})

	outputDir := helpers.TmpDirWOSymlinks(t)

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(outputDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = outputDir

	v := venvtest.NewOSWithEmptyEnv()
	l := logger.CreateLogger()

	plan, err := scaffold.Prepare(t.Context(), l, v, opts, source, "")
	require.NoError(t, err)

	t.Cleanup(func() { plan.Cleanup(v.FS) })

	require.NoError(t, plan.Generate(t.Context(), l, v, opts, nil))

	assert.FileExists(t, filepath.Join(outputDir, "terragrunt.hcl"))
	assert.FileExists(t, filepath.Join(outputDir, "extra.hcl"))
	assert.Empty(t, plan.GeneratedFiles(),
		"copyable scaffolds report via CopyCmd.Result, not GeneratedFiles")
}

// TestScaffoldRefusesToOverwriteWhenCopying covers a collision in the output
// directory: the component is not scaffolded, and nothing is half-written.
func TestScaffoldRefusesToOverwriteWhenCopying(t *testing.T) {
	t.Parallel()

	source := writeComponent(t, "units/app", map[string]string{
		"terragrunt.hcl": "# unit\n",
		"extra.hcl":      "# extra\n",
	})

	outputDir := helpers.TmpDirWOSymlinks(t)
	writeFile(t, filepath.Join(outputDir, "extra.hcl"), "# mine\n")

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(outputDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = outputDir
	opts.NonInteractive = true

	err = scaffold.Run(t.Context(), logger.CreateLogger(), venvtest.NewOSWithEmptyEnv(), opts, source, "")
	require.Error(t, err)

	assert.NoFileExists(t, filepath.Join(outputDir, "terragrunt.hcl"))
	assert.Equal(t, "# mine\n", readFile(t, filepath.Join(outputDir, "extra.hcl")))
}

// writeComponent stages a component at dir inside a fresh repository and
// returns the go-getter source string addressing it, the same `repo//dir`
// shape the catalog hands to scaffold.
func writeComponent(t *testing.T, dir string, files map[string]string) string {
	t.Helper()

	repoDir := helpers.TmpDirWOSymlinks(t)

	for name, contents := range files {
		writeFile(t, filepath.Join(repoDir, filepath.FromSlash(dir), name), contents)
	}

	return repoDir + "//" + dir
}

// runScaffold scaffolds source into a fresh output directory and returns it.
func runScaffold(t *testing.T, source string) string {
	t.Helper()

	outputDir := helpers.TmpDirWOSymlinks(t)

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(outputDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = outputDir
	opts.NonInteractive = true

	require.NoError(
		t,
		scaffold.Run(t.Context(), logger.CreateLogger(), venvtest.NewOSWithEmptyEnv(), opts, source, ""),
	)

	return outputDir
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0644))
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(raw)
}
