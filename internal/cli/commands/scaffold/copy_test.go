package scaffold_test

import (
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/getter"
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

// TestScaffoldCopiesUnitAddressedWithoutSubdirectory pins that a local unit
// named by its own directory, with no "//" selector, arrives without the
// manifest the local copy writes beside it.
func TestScaffoldCopiesUnitAddressedWithoutSubdirectory(t *testing.T) {
	t.Parallel()

	source := helpers.TmpDirWOSymlinks(t)
	writeFile(t, filepath.Join(source, "terragrunt.hcl"), "# unit\n")

	outputDir := runScaffold(t, source)

	assert.FileExists(t, filepath.Join(outputDir, "terragrunt.hcl"))
	assert.NoFileExists(t, filepath.Join(outputDir, getter.SourceManifestName))
}

// TestScaffoldLeavesNoSourceManifestBehind pins that scaffold output holds no
// source manifest at any depth, whichever getter fetched the component.
func TestScaffoldLeavesNoSourceManifestBehind(t *testing.T) {
	t.Parallel()

	nestedFiles := map[string]string{
		"nested/sub/notes.txt":          "# notes\n",
		".terraform/providers/fake.zip": "zip\n",
		".git/objects/ab/deadbeef":      "object\n",
		".gitignore":                    ".terraform\n",
		"README.md":                     "# Component\n",
	}

	moduleFiles := map[string]string{
		"main.tf": "variable \"name\" {\n  type = string\n}\n",
	}

	templateFiles := map[string]string{
		"boilerplate.yml":   "variables: []\n",
		"terragrunt.hcl":    "# generated\n",
		"nested/sub/doc.md": "# doc\n",
	}

	testCases := []struct {
		name  string
		setup func(t *testing.T) (source, templateURL string)
		want  []string
	}{
		{
			name: "unit addressed by its own directory",
			setup: func(t *testing.T) (string, string) {
				t.Helper()

				dir := helpers.TmpDirWOSymlinks(t)
				writeFiles(t, dir, nestedFiles)
				writeFile(t, filepath.Join(dir, "terragrunt.hcl"), "# unit\n")

				return helpers.FileURL(dir), ""
			},
			want: []string{"terragrunt.hcl", "nested/sub/notes.txt", ".terraform/providers/fake.zip"},
		},
		{
			name: "unit addressed through a subdirectory",
			setup: func(t *testing.T) (string, string) {
				t.Helper()

				files := map[string]string{"terragrunt.hcl": "# unit\n"}
				maps.Copy(files, nestedFiles)

				return writeComponent(t, "units/app", files), ""
			},
			want: []string{"terragrunt.hcl", "nested/sub/notes.txt", ".terraform/providers/fake.zip"},
		},
		{
			name: "unit cloned from a repository that committed manifests",
			setup: func(t *testing.T) (string, string) {
				t.Helper()

				committed := "units/app/nested/" + getter.SourceManifestName

				repoDir := helpers.TmpDirWOSymlinks(t)
				writeFiles(t, repoDir, map[string]string{
					"units/app/terragrunt.hcl":               "# unit\n",
					"units/app/nested/sub/notes.txt":         "# notes\n",
					"units/app/" + getter.SourceManifestName: "committed\n",
					committed:                                "committed\n",
				})
				helpers.CreateGitRepo(t, repoDir)

				// A global gitignore could keep the manifests out of the
				// commit, and the case would then pass without testing anything.
				out, err := exec.CommandContext(t.Context(), "git", "-C", repoDir, "ls-files", "--error-unmatch", committed).
					CombinedOutput()
				require.NoError(t, err, string(out))

				return "git::" + helpers.FileURL(repoDir) + "//units/app", ""
			},
			want: []string{"terragrunt.hcl", "nested/sub/notes.txt"},
		},
		{
			name: "module rendered from its own template",
			setup: func(t *testing.T) (string, string) {
				t.Helper()

				files := maps.Clone(moduleFiles)
				for name, contents := range templateFiles {
					files[".boilerplate/"+name] = contents
				}

				return writeComponent(t, "modules/vpc", files), ""
			},
			want: []string{"terragrunt.hcl", "nested/sub/doc.md"},
		},
		{
			name: "module rendered from a separate template",
			setup: func(t *testing.T) (string, string) {
				t.Helper()

				templateDir := helpers.TmpDirWOSymlinks(t)
				writeFiles(t, templateDir, templateFiles)

				return writeComponent(t, "modules/vpc", moduleFiles), helpers.FileURL(templateDir)
			},
			want: []string{"terragrunt.hcl", "nested/sub/doc.md"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source, templateURL := tc.setup(t)
			outputDir := runScaffoldWithTemplate(t, source, templateURL)

			for _, name := range tc.want {
				require.FileExists(t, filepath.Join(outputDir, filepath.FromSlash(name)))
			}

			assert.Empty(t, findSourceManifests(t, outputDir))
		})
	}
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

	return helpers.FileURL(repoDir) + "//" + dir
}

// runScaffold scaffolds source into a fresh output directory and returns it.
func runScaffold(t *testing.T, source string) string {
	t.Helper()

	return runScaffoldWithTemplate(t, source, "")
}

// runScaffoldWithTemplate is runScaffold rendering through the template at
// templateURL, when it is set.
func runScaffoldWithTemplate(t *testing.T, source, templateURL string) string {
	t.Helper()

	outputDir := helpers.TmpDirWOSymlinks(t)

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(outputDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = outputDir
	opts.NonInteractive = true

	require.NoError(
		t,
		scaffold.Run(t.Context(), logger.CreateLogger(), venvtest.NewOSWithEmptyEnv(), opts, source, templateURL),
	)

	return outputDir
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0644))
}

// writeFiles writes each file, keyed by its slash-separated path, under dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, contents := range files {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), contents)
	}
}

// findSourceManifests returns every source manifest under dir, relative to it.
func findSourceManifests(t *testing.T, dir string) []string {
	t.Helper()

	var found []string

	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.Name() != getter.SourceManifestName {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		found = append(found, filepath.ToSlash(rel))

		return nil
	}))

	return found
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(raw)
}
