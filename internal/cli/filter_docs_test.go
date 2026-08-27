package cli_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The filter documentation pairs each query with the components it selects.
// Every case below is one of those pairs. A change in what a filter selects
// fails here, on the page that promised it.
//
// The pages are under docs/src/content/docs/03-features/08-filter.

// docsRoot is where a documentation example's tree is mounted. Each example
// discovers from the root directory inside it, leaving room beside that
// directory for the external dependencies some of the examples reach.
const docsRoot = "/docs"

// unitHCL is the smallest configuration that makes a directory discoverable as
// a unit. A stack needs no contents at all, only the file that names it one,
// so [tree.stack] writes an empty one.
const unitHCL = "terraform {\n  source = \".\"\n}"

// tree collects the files one documentation example runs against.
type tree map[string]string

// unit adds a discoverable unit at each path.
func (t tree) unit(paths ...string) tree {
	for _, path := range paths {
		t.config(path, unitHCL)
	}

	return t
}

// stack adds a discoverable stack at each path.
func (t tree) stack(paths ...string) tree {
	for _, path := range paths {
		t[filepath.Join(path, "terragrunt.stack.hcl")] = ""
	}

	return t
}

// config adds a unit at path whose configuration is contents.
func (t tree) config(path, contents string) tree {
	t[filepath.Join(path, "terragrunt.hcl")] = contents
	t[filepath.Join(path, "main.tf")] = ""

	return t
}

// file adds a file that is not itself a component, for the configurations
// units read and the modules they point at.
func (t tree) file(path, contents string) tree {
	t[path] = contents

	return t
}

// dependent adds a unit at path that depends on each of deps. The blocks are
// labeled by position, since no example selects a dependency by its label.
func (t tree) dependent(path string, deps ...string) tree {
	var contents strings.Builder

	contents.WriteString(unitHCL)

	for i, dep := range deps {
		fmt.Fprintf(&contents, "\n\ndependency \"dep%d\" {\n  config_path = %q\n}\n", i, dep)
	}

	return t.config(path, contents.String())
}

// findWithFilters runs find against files and returns the paths it printed.
func findWithFilters(t *testing.T, files tree, filters ...string) string {
	t.Helper()

	v := venvtest.New().WithFS(venvtest.NewFS(t, docsRoot, files))

	const fixedArgs = 4 // find, --no-color, --working-dir and its value

	args := make([]string, 0, fixedArgs+2*len(filters))
	args = append(args, "find", "--no-color")

	for _, filter := range filters {
		args = append(args, "--filter", filter)
	}

	out, err := runCLI(t, v, append(args, "--working-dir", filepath.Join(docsRoot, "root"))...)
	require.NoError(t, err)

	return out
}

func TestFilterDocumentationExamples(t *testing.T) {
	t.Parallel()

	nameBased := tree{}.unit("root/apps/app1", "root/apps/app2", "root/apps/other")

	attributeBased := tree{}.
		dependent("root/unit1", "../../dependencies/dependency-of-app1").
		stack("root/stack1").
		unit("dependencies/dependency-of-app1")

	pathBased := tree{}.unit(
		"root/envs/prod/apps/app1", "root/envs/prod/apps/app2",
		"root/envs/stage/apps/app1", "root/envs/stage/apps/app2",
		"root/envs/dev/apps/app1", "root/envs/dev/apps/app2",
	)

	negation := tree{}.
		unit(
			"root/envs/prod/apps/app1", "root/envs/prod/apps/app2",
			"root/envs/stage/apps/app1", "root/envs/stage/apps/app2",
		).
		stack("root/envs/prod/stacks/stack1", "root/envs/stage/stacks/stack1")

	intersection := tree{}.
		unit(
			"root/prod/units/unit1", "root/prod/units/unit2",
			"root/dev/units/unit1", "root/dev/units/unit2",
		).
		stack(
			"root/prod/stacks/stack1", "root/prod/stacks/stack2",
			"root/dev/stacks/stack1", "root/dev/stacks/stack2",
		)

	reading := tree{}.
		file("root/shared.hcl", "locals {\n  common_value = \"shared\"\n}\n").
		file("root/shared.tfvars", "test_var = \"value\"\n").
		file("root/common/vars.hcl", "locals {\n  vpc_cidr = \"10.0.0.0/16\"\n}\n").
		config("root/apps/app1", `locals {
  shared = read_terragrunt_config("../../shared.hcl")
}

terraform {
  source = "."
}
`).
		config("root/apps/app2", `locals {
  shared = read_terragrunt_config("../../shared.hcl")
  vars   = read_tfvars_file("../../shared.tfvars")
}

terraform {
  source = "."
}
`).
		config("root/apps/app3", `locals {
  common = read_terragrunt_config("../../common/vars.hcl")
}

terraform {
  source = "."
}
`).
		unit("root/libs/lib1")

	// db and cache depend on vpc. service depends on db and cache.
	graphBased := tree{}.
		unit("root/vpc").
		dependent("root/db", "../vpc").
		dependent("root/cache", "../vpc").
		dependent("root/service", "../db", "../cache")

	sourceBased := tree{}.
		config("root/github-acme-foo", "terraform {\n  source = \"github.com/acme/foo\"\n}\n").
		config("root/github-acme-bar", "terraform {\n  source = \"git::git@github.com:acme/bar\"\n}\n").
		config("root/gitlab-example-baz", "terraform {\n  source = \"gitlab.com/example/baz\"\n}\n").
		config("root/local-module", "terraform {\n  source = \"./module\"\n}\n").
		file("root/local-module/module/main.tf", "").
		config("root/other-unit", "terraform {\n  source = \"s3://bucket/module\"\n}\n")

	testCases := []struct {
		name    string
		want    string
		files   tree
		filters []string
	}{
		// Name-based filtering.
		{
			name:    "name-based-exact-match",
			files:   nameBased,
			filters: []string{"app1"},
			want:    "apps/app1\n",
		},
		{
			name:    "name-based-glob-pattern",
			files:   nameBased,
			filters: []string{"app*"},
			want:    "apps/app1\napps/app2\n",
		},

		// Path-based filtering.
		{
			name:    "path-based-relative-exact-match",
			files:   pathBased,
			filters: []string{"./envs/prod/apps/app1"},
			want:    "envs/prod/apps/app1\n",
		},
		{
			name:    "path-based-relative-glob-pattern",
			files:   pathBased,
			filters: []string{"./envs/stage/**"},
			want:    "envs/stage/apps/app1\nenvs/stage/apps/app2\n",
		},
		{
			name:    "path-based-absolute-exact-match",
			files:   pathBased,
			filters: []string{filepath.Join(docsRoot, "root", "envs", "dev", "apps", "*")},
			want:    "envs/dev/apps/app1\nenvs/dev/apps/app2\n",
		},
		{
			name:    "path-based-braced-exact-match",
			files:   pathBased,
			filters: []string{"{./envs/prod/apps/app2}"},
			want:    "envs/prod/apps/app2\n",
		},

		// Attribute-based filtering.
		{
			name:    "attribute-type-unit",
			files:   attributeBased,
			filters: []string{"type=unit"},
			want:    "unit1\n",
		},
		{
			name:    "attribute-type-stack",
			files:   attributeBased,
			filters: []string{"type=stack"},
			want:    "stack1\n",
		},
		{
			name:    "attribute-based-external-false",
			files:   attributeBased,
			filters: []string{"{./*}... | external=false"},
			want:    "stack1\nunit1\n",
		},
		{
			name:    "attribute-based-external-true",
			files:   attributeBased,
			filters: []string{"{./*}... | external=true"},
			want:    "../dependencies/dependency-of-app1\n",
		},
		{
			name:    "attribute-based-name-glob",
			files:   attributeBased,
			filters: []string{"name=stack*"},
			want:    "stack1\n",
		},

		// Negation.
		{
			name:    "negation-by-name",
			files:   negation,
			filters: []string{"!app1"},
			want: "envs/prod/apps/app2\nenvs/prod/stacks/stack1\n" +
				"envs/stage/apps/app2\nenvs/stage/stacks/stack1\n",
		},
		{
			name:    "negation-by-path",
			files:   negation,
			filters: []string{"!./envs/prod/**"},
			want:    "envs/stage/apps/app1\nenvs/stage/apps/app2\nenvs/stage/stacks/stack1\n",
		},
		{
			name:    "negation-by-attribute",
			files:   negation,
			filters: []string{"!type=stack"},
			want: "envs/prod/apps/app1\nenvs/prod/apps/app2\n" +
				"envs/stage/apps/app1\nenvs/stage/apps/app2\n",
		},

		// Intersection.
		{
			name:    "intersection-by-path-and-attribute",
			files:   intersection,
			filters: []string{"./prod/** | type=unit"},
			want:    "prod/units/unit1\nprod/units/unit2\n",
		},
		{
			name:    "intersection-by-path-and-negation",
			files:   intersection,
			filters: []string{"./prod/** | !type=unit"},
			want:    "prod/stacks/stack1\nprod/stacks/stack2\n",
		},
		{
			name:    "intersection-by-path-type-and-negation",
			files:   intersection,
			filters: []string{"./dev/** | type=unit | !name=unit1"},
			want:    "dev/units/unit2\n",
		},

		// Reading-attribute filtering.
		{
			name:    "reading-exact-file-match",
			files:   reading,
			filters: []string{"reading=shared.hcl"},
			want:    "apps/app1\napps/app2\n",
		},
		{
			name:    "reading-glob-pattern",
			files:   reading,
			filters: []string{"reading=shared*"},
			want:    "apps/app1\napps/app2\n",
		},
		{
			name:    "reading-nested-path",
			files:   reading,
			filters: []string{"reading=common/vars.hcl"},
			want:    "apps/app3\n",
		},
		{
			name:    "reading-negation",
			files:   reading,
			filters: []string{"!reading=shared.hcl"},
			want:    "apps/app3\nlibs/lib1\n",
		},
		{
			name:    "reading-intersection",
			files:   reading,
			filters: []string{"./apps/** | reading=shared.hcl"},
			want:    "apps/app1\napps/app2\n",
		},

		// Graph traversal.
		{
			name:    "graph-dependency-traversal",
			files:   graphBased,
			filters: []string{"service..."},
			want:    "cache\ndb\nservice\nvpc\n",
		},
		{
			name:    "graph-dependent-traversal",
			files:   graphBased,
			filters: []string{"...vpc"},
			want:    "cache\ndb\nservice\nvpc\n",
		},
		{
			name:    "graph-both-directions",
			files:   graphBased,
			filters: []string{"...db..."},
			want:    "db\nservice\nvpc\n",
		},
		{
			name:    "graph-exclude-target",
			files:   graphBased,
			filters: []string{"^service..."},
			want:    "cache\ndb\nvpc\n",
		},
		{
			name:    "graph-with-path-filter",
			files:   graphBased,
			filters: []string{"{./service}..."},
			want:    "cache\ndb\nservice\nvpc\n",
		},
		{
			name:    "graph-with-attribute-filter",
			files:   graphBased,
			filters: []string{"...name=vpc"},
			want:    "cache\ndb\nservice\nvpc\n",
		},
		{
			name:    "graph-with-intersection",
			files:   graphBased,
			filters: []string{"service... | !^db..."},
			want:    "cache\ndb\nservice\n",
		},

		// Depth-limited graph traversal.
		{
			name:    "graph-depth-limited-dependencies-1-level",
			files:   graphBased,
			filters: []string{"service...1"},
			want:    "cache\ndb\nservice\n",
		},
		{
			name:    "graph-depth-limited-dependents-1-level",
			files:   graphBased,
			filters: []string{"1...vpc"},
			want:    "cache\ndb\nvpc\n",
		},
		{
			name:    "graph-depth-limited-both-directions",
			files:   graphBased,
			filters: []string{"1...db...2"},
			want:    "db\nservice\nvpc\n",
		},

		// Source-based filtering.
		{
			name:    "source-exact-match-github",
			files:   sourceBased,
			filters: []string{"source=github.com/acme/foo"},
			want:    "github-acme-foo\n",
		},
		{
			name:    "source-exact-match-gitlab",
			files:   sourceBased,
			filters: []string{"source=gitlab.com/example/baz"},
			want:    "gitlab-example-baz\n",
		},
		{
			name:    "source-exact-match-local",
			files:   sourceBased,
			filters: []string{"source=./module"},
			want:    "local-module\n",
		},
		{
			name:    "source-glob-github-org",
			files:   sourceBased,
			filters: []string{"source=*github.com**acme/*"},
			want:    "github-acme-bar\ngithub-acme-foo\n",
		},
		{
			name:    "source-glob-github-ssh",
			files:   sourceBased,
			filters: []string{"source=git::git@github.com:acme/**"},
			want:    "github-acme-bar\n",
		},
		{
			name:    "source-glob-all-github",
			files:   sourceBased,
			filters: []string{"source=**github.com**"},
			want:    "github-acme-bar\ngithub-acme-foo\n",
		},
		{
			name:    "source-glob-gitlab",
			files:   sourceBased,
			filters: []string{"source=gitlab.com/**"},
			want:    "gitlab-example-baz\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, findWithFilters(t, tc.files, tc.filters...))
		})
	}
}

// TestFilterDocumentationExamplesWithUnion covers the examples that pass more
// than one --filter, where the results are the union of what each selects.
func TestFilterDocumentationExamplesWithUnion(t *testing.T) {
	t.Parallel()

	union := tree{}.
		unit(
			"root/envs/prod/unit1", "root/envs/prod/unit2",
			"root/envs/stage/unit1", "root/envs/stage/unit2",
			"root/dev/unit1", "root/dev/unit2",
		).
		stack(
			"root/envs/prod/stack1", "root/envs/prod/stack2",
			"root/envs/stage/stack1", "root/envs/stage/stack2",
			"root/dev/stack1", "root/dev/stack2",
		)

	testCases := []struct {
		name    string
		want    string
		filters []string
	}{
		{
			name:    "union-by-two-names",
			filters: []string{"unit1", "stack1"},
			want: "dev/stack1\ndev/unit1\nenvs/prod/stack1\nenvs/prod/unit1\n" +
				"envs/stage/stack1\nenvs/stage/unit1\n",
		},
		{
			name:    "union-by-two-paths",
			filters: []string{"./envs/prod/**", "./envs/stage/**"},
			want: "envs/prod/stack1\nenvs/prod/stack2\nenvs/prod/unit1\nenvs/prod/unit2\n" +
				"envs/stage/stack1\nenvs/stage/stack2\nenvs/stage/unit1\nenvs/stage/unit2\n",
		},
		{
			name:    "union-by-name-and-negation",
			filters: []string{"stack2", "!./envs/prod/**", "!./envs/stage/**"},
			want:    "dev/stack2\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, findWithFilters(t, union, tc.filters...))
		})
	}
}
