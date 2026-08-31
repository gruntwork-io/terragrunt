package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// basicUnits is one unit and one stack, the smallest tree that shows find
// distinguishing the two kinds.
var basicUnits = map[string]string{
	"unit/terragrunt.hcl":        "",
	"stack/terragrunt.stack.hcl": "",
}

// dagUnits wires four units so that dependency order and alphabetical order
// disagree: b comes before a, and d before c.
var dagUnits = map[string]string{
	"a-dependent/terragrunt.hcl": `
dependency "dep" {
  config_path = "../b-dependency"

  mock_outputs = {
    value = "mock value"
  }
}
`,
	"b-dependency/terragrunt.hcl": "",
	"c-mixed-deps/terragrunt.hcl": `
dependency "single_dep" {
  config_path = "../a-dependent"

  mock_outputs = {
    value = "mock value"
  }
}

dependencies {
  paths = ["../d-dependencies-only"]
}
`,
	"d-dependencies-only/terragrunt.hcl": `
dependencies {
  paths = ["../a-dependent"]
}
`,
}

func TestFindBasic(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "stack\nunit\n", runFind(t, basicUnits))
}

func TestFindBasicJSON(t *testing.T) {
	t.Parallel()

	assert.JSONEq(
		t,
		`[{"type": "stack", "path": "stack"}, {"type": "unit", "path": "unit"}]`,
		runFind(t, basicUnits, "--json"),
	)
}

func TestFindHidden(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"unit/terragrunt.hcl":        "",
		"stack/terragrunt.stack.hcl": "",
		".hide/unit/terragrunt.hcl":  "",
	}

	t.Run("hidden components are found by default", func(t *testing.T) {
		t.Parallel()

		want := filepath.Join(".hide", "unit") + "\nstack\nunit\n"
		assert.Equal(t, want, runFind(t, files))
	})

	t.Run("no-hidden leaves them out", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "stack\nunit\n", runFind(t, files, "--no-hidden"))
	})
}

func TestFindDAG(t *testing.T) {
	t.Parallel()

	t.Run("alphabetical by default", func(t *testing.T) {
		t.Parallel()

		want := "a-dependent\nb-dependency\nc-mixed-deps\nd-dependencies-only\n"
		assert.Equal(t, want, runFind(t, dagUnits))
	})

	t.Run("dag orders dependencies first", func(t *testing.T) {
		t.Parallel()

		want := "b-dependency\na-dependent\nd-dependencies-only\nc-mixed-deps\n"
		assert.Equal(t, want, runFind(t, dagUnits, "--dag"))
	})
}

// TestFindDAGWithMixedDependencies pins the order for a tree that uses both
// the dependency block and the dependencies block, which feed one graph.
func TestFindDAGWithMixedDependencies(t *testing.T) {
	t.Parallel()

	t.Run("text", func(t *testing.T) {
		t.Parallel()

		want := "b-dependency\na-dependent\nd-dependencies-only\nc-mixed-deps\n"
		assert.Equal(t, want, runFind(t, dagUnits, "--dag", "--dependencies"))
	})

	t.Run("json carries the edges", func(t *testing.T) {
		t.Parallel()

		want := `[
		  {"type":"unit","path":"b-dependency"},
		  {"type":"unit","path":"a-dependent","dependencies":["b-dependency"]},
		  {"type":"unit","path":"d-dependencies-only","dependencies":["a-dependent"]},
		  {"type":"unit","path":"c-mixed-deps","dependencies":["a-dependent","d-dependencies-only"]}
		]`

		assert.JSONEq(t, want, runFind(t, dagUnits, "--dag", "--dependencies", "--json"))
	})
}

// TestFindToleratesTerraformSourceReferencingDependency pins that a unit with
// an unparseable config still gets listed. Its terraform source reads the
// dependency namespace, which is rejected because the source has to resolve
// before dependencies are evaluated. Discovery drops the edge and keeps
// going.
func TestFindToleratesTerraformSourceReferencingDependency(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"app/terragrunt.hcl": `
terraform {
  source = true ? "./module" : dependency.upstream.outputs.source
}

dependency "upstream" {
  config_path  = "../upstream"
  mock_outputs = { source = "./module" }
}
`,
		"upstream/terragrunt.hcl": `
terraform {
  source = "./module"
}
`,
	}

	assert.JSONEq(
		t,
		`[{"type":"unit","path":"app"},{"type":"unit","path":"upstream"}]`,
		runFind(t, files, "--dependencies", "--json"),
	)
}

// internalVExternalUnits puts a dependency outside the directory find walks,
// so --external decides whether it appears.
var internalVExternalUnits = map[string]string{
	"internal/a-dependent/terragrunt.hcl": `
dependency "dep_b" {
  config_path = "../b-dependency"
}

dependency "dep_c" {
  config_path = "../../external/c-dependency"
}
`,
	"internal/b-dependency/terragrunt.hcl": "",
	"external/c-dependency/terragrunt.hcl": "",
}

func TestFindExternalDependencies(t *testing.T) {
	t.Parallel()

	assertExternalDependencies(t)
}

// TestFindExternalDependenciesWithFilterFlag repeats those two runs with a
// filter that selects everything, so a filter cannot quietly undo --external.
func TestFindExternalDependenciesWithFilterFlag(t *testing.T) {
	t.Parallel()

	assertExternalDependencies(t, "--filter", "{./**}...")
}

func assertExternalDependencies(t *testing.T, extra ...string) {
	t.Helper()

	v := venvtest.New().WithFS(venvtest.NewFS(t, discoveryRoot, internalVExternalUnits))
	internal := filepath.Join(discoveryRoot, "internal")

	base := []string{"find", "--no-color", "--working-dir", internal, "--dependencies"}

	out, err := runCLI(t, v, append(append(base, "--external"), extra...)...)
	require.NoError(t, err)

	want := filepath.Join("..", "external", "c-dependency") + "\na-dependent\nb-dependency\n"
	assert.Equal(t, want, out)

	out, err = runCLI(t, v, base...)
	require.NoError(t, err)
	assert.Equal(t, "a-dependent\nb-dependency\n", out)
}

func TestFindInclude(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"cloud.hcl":          "",
		"foo/terragrunt.hcl": "",
		"bar/terragrunt.hcl": `
include "cloud" {
  path = find_in_parent_folders("cloud.hcl")
}
`,
	}

	assert.JSONEq(
		t,
		`[{"type":"unit","path":"bar","include":{"cloud":"cloud.hcl"}},{"type":"unit","path":"foo"}]`,
		runFind(t, files, "--include", "--json"),
	)
}

// excludeUnits carries one unit excluded from plan, one from apply, and one
// excluded from neither.
var excludeUnits = map[string]string{
	"unit1/terragrunt.hcl": `
exclude {
  if      = true
  actions = ["plan"]
}
`,
	"unit2/terragrunt.hcl": `
exclude {
  if      = true
  actions = ["apply"]
}
`,
	"unit3/terragrunt.hcl": "",
}

func TestFindExclude(t *testing.T) {
	t.Parallel()

	t.Run("exclude reports every unit and its config", func(t *testing.T) {
		t.Parallel()

		want := `[
		  {"type":"unit","path":"unit1","exclude":
		    {"if":true,"actions":["plan"],"exclude_dependencies":null,"no_run":null}},
		  {"type":"unit","path":"unit2","exclude":
		    {"if":true,"actions":["apply"],"exclude_dependencies":null,"no_run":null}},
		  {"type":"unit","path":"unit3"}
		]`

		assert.JSONEq(t, want, runFind(t, excludeUnits, "--exclude", "--json"))
	})

	t.Run("queue-construct-as drops what the action excludes", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "unit2\nunit3\n", runFind(t, excludeUnits, "--queue-construct-as", "plan"))
		assert.Equal(t, "unit1\nunit3\n", runFind(t, excludeUnits, "--queue-construct-as", "apply"))
	})
}

// queueUnits nests units under two environments so that the order
// --queue-construct-as produces is visible in both directions.
var queueUnits = map[string]string{
	"stacks/live/dev/terragrunt.stack.hcl":  "",
	"stacks/live/prod/terragrunt.stack.hcl": "",
	"units/live/dev/vpc/terragrunt.hcl":     "",
	"units/live/prod/vpc/terragrunt.hcl":    "",
	"units/live/dev/db/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}
`,
	"units/live/prod/db/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}
`,
	"units/live/dev/ec2/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

dependency "db" {
  config_path = "../db"
}
`,
	"units/live/prod/ec2/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

dependency "db" {
  config_path = "../db"
}
`,
}

// TestFindQueueConstructAs pins that a destroy queue reverses the order an
// apply queue produces, since destroying a dependency before its dependents
// would fail.
func TestFindQueueConstructAs(t *testing.T) {
	t.Parallel()

	up := []string{
		"stacks/live/dev", "stacks/live/prod",
		"units/live/dev/vpc", "units/live/prod/vpc",
		"units/live/dev/db", "units/live/prod/db",
		"units/live/dev/ec2", "units/live/prod/ec2",
	}

	down := []string{
		"stacks/live/dev", "stacks/live/prod",
		"units/live/dev/ec2", "units/live/prod/ec2",
		"units/live/dev/db", "units/live/prod/db",
		"units/live/dev/vpc", "units/live/prod/vpc",
	}

	t.Run("plan", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, up, foundPaths(t, runFind(t, queueUnits, "--json", "--queue-construct-as", "plan")))
	})

	t.Run("destroy", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, down, foundPaths(t, runFind(t, queueUnits, "--json", "--queue-construct-as", "destroy")))
	})
}

// foundPaths pulls the paths out of find's JSON, in the order it printed them.
func foundPaths(t *testing.T, out string) []string {
	t.Helper()

	var components find.FoundComponents

	require.NoError(t, json.Unmarshal([]byte(out), &components))

	paths := make([]string, 0, len(components))
	for _, c := range components {
		paths = append(paths, filepath.ToSlash(c.Path))
	}

	return paths
}

// TestFindWithReadTerragruntConfig pins what find reports for a unit that
// reads a config file declaring a dependency block.
func TestFindWithReadTerragruntConfig(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"terragrunt.hcl": `
locals {
  common_deps = read_terragrunt_config("${get_terragrunt_dir()}/common_deps.hcl")
}

terraform {
  source = "."
}

inputs = {
  value        = local.common_deps.dependency.module.outputs.value
  module_value = local.common_deps.inputs.module_value
}
`,
		"common_deps.hcl": `
dependency "module" {
  config_path = "./module"
}

inputs = {
  module_value = dependency.module.outputs.value
}
`,
		"module/terragrunt.hcl": `
terraform {
  source = "."
}
`,
	}

	// A dependency block in a read file draws no edge, so --dag has nothing to
	// reorder and both runs report the same two units. These pin the read
	// parsing, and nothing about ordering.
	for _, args := range [][]string{{"--json"}, {"--dag", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()

			assert.ElementsMatch(t, []string{".", "module"}, foundPaths(t, runFind(t, files, args...)))
		})
	}
}

// TestFindWithDynamicDependencyConfigPath pins a config_path that needs a
// file that does not exist. The edge is unknowable, so discovery reports the
// unit with no dependency.
func TestFindWithDynamicDependencyConfigPath(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"unit/terragrunt.hcl": `
dependency "target" {
  config_path = read_terragrunt_config("does-not-exist.hcl").locals.aws_region == "x" ? "../a" : "../b"
}
`,
	}

	t.Run("text", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "unit\n", runFind(t, files, "--dag"))
	})

	t.Run("json", func(t *testing.T) {
		t.Parallel()

		assert.JSONEq(t, `[{"type":"unit","path":"unit"}]`, runFind(t, files, "--dag", "--json"))
	})
}
