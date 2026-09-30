package config_test

import (
	"path/filepath"
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// parityFixtureRoots are the fixture directories whose terragrunt.hcl files the parity test parses.
var parityFixtureRoots = []string{
	"include",
	"include-deep",
	"include-expose",
	"include-multiple",
	"multiinclude-dependency",
	"locals",
	"inputs",
	"feature-flags",
	"errors",
	"exclude",
	"hooks",
	"extra-args",
	"read-config",
	"parent-folders",
	"find-parent",
	"get-path",
	"null-values",
	"partial-parse",
	"render-json-mock-outputs",
	"dependency-expansion/mocks",
	"dependency-expansion/keyed",
}

// parityExcludedDirs are fixtures that call run_cmd, which the tofu stub cannot answer, or AWS.
var parityExcludedDirs = map[string]struct{}{
	"locals/run-once":              {},
	"locals/run-multiple":          {},
	"read-config/iam_role_in_file": {},
}

// withOutputExcludedDirs are fixtures whose output fetch calls AWS through the SDK, which the tofu stub cannot
// intercept.
var withOutputExcludedDirs = map[string]struct{}{
	"read-config/iam_roles_multiple_modules": {},
	"render-json-mock-outputs/app":           {},
}

func TestParityFixturesSkipOutput(t *testing.T) {
	t.Parallel()

	fixtures := loadFixtures(t, parityFixtureRoots, parityExcludedDirs)

	for _, fixture := range fixtures.configs {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			assertParity(t, parityCase{
				newFS:   fixtures.newFS,
				cfgPath: fixture.path,
				configure: func(pctx *pkgconfig.ParsingContext) {
					pctx.SkipOutput = true
					pctx.OriginalTerraformCommand = "plan"
				},
			})
		})
	}
}

func TestParityFixturesWithOutput(t *testing.T) {
	t.Parallel()

	fixtures := loadFixtures(t, parityFixtureRoots, parityExcludedDirs)

	for _, fixture := range fixtures.configs {
		if _, excluded := withOutputExcludedDirs[fixture.name]; excluded {
			continue
		}

		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			assertParity(t, parityCase{
				newFS:     fixtures.newFS,
				cfgPath:   fixture.path,
				configure: func(*pkgconfig.ParsingContext) {},
			})
		})
	}
}

func TestParityAutoInclude(t *testing.T) {
	t.Parallel()

	regionValues := cty.ObjectVal(map[string]cty.Value{"region": cty.StringVal("us-east-1")})

	testCases := []struct {
		files     map[string]string
		values    *cty.Value
		name      string
		cfgPath   string
		command   string
		stackDeps bool
	}{
		{
			name:      "malformed sibling",
			stackDeps: true,
			files: map[string]string{
				"shared/root.hcl": `
inputs = {
  shared = "from-root"
}
`,
				"terragrunt.hcl": `
include "root" {
  path           = "{{root}}/shared/root.hcl"
  merge_strategy = "shallow"
}

inputs = {
  name = "from-unit"
}
`,
				"terragrunt.autoinclude.hcl": `
inputs = {
  broken = local.does_not_exist
}
`,
			},
		},
		{
			name:      "autoinclude own locals",
			stackDeps: true,
			command:   "init",
			files: map[string]string{
				"foo/terragrunt.hcl": ``,
				"terragrunt.hcl": `
remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    path = dependency.foo.outputs.val
  }
}
`,
				"terragrunt.autoinclude.hcl": `
locals {
  target = "./foo"
}

dependency "foo" {
  config_path  = local.target
  skip_outputs = true
  mock_outputs = {
    val = "autoinclude-local-marker"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`,
			},
		},
		{
			name:      "include inherited deps",
			stackDeps: true,
			command:   "init",
			files: map[string]string{
				"foo/terragrunt.hcl": ``,
				"terragrunt.hcl": `
remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    path = dependency.foo.outputs.val
  }
}
`,
				"base/base.hcl": `
dependency "foo" {
  config_path  = "{{root}}/foo"
  skip_outputs = true
  mock_outputs = {
    val = "include-inherited-dep-marker"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`,
				"terragrunt.autoinclude.hcl": `
include "base" {
  path           = "{{root}}/base/base.hcl"
  merge_strategy = "deep"
}
`,
			},
		},
		{
			name:      "same dir include does not recurse",
			stackDeps: true,
			files: map[string]string{
				"terragrunt.hcl": `
inputs = {
  from_unit = "unit-value"
}
`,
				"common.hcl": `
inputs = {
  from_common = "common-value"
}
`,
				"terragrunt.autoinclude.hcl": `
include "common" {
  path           = "{{root}}/common.hcl"
  merge_strategy = "deep"
}
`,
			},
		},
		{
			name:      "pulled in file does not fold foreign autoinclude",
			stackDeps: true,
			command:   "init",
			files: map[string]string{
				"wanted-target/terragrunt.hcl": ``,
				"leak-target/terragrunt.hcl":   ``,
				"terragrunt.hcl": `
inputs = { from_unit = "a" }
`,
				"terragrunt.autoinclude.hcl": `
dependency "wanted" {
  config_path  = "{{root}}/wanted-target"
  skip_outputs = true
  mock_outputs = { val = "wanted" }
  mock_outputs_allowed_terraform_commands = ["init"]
}

include "base" {
  path           = "{{root}}/b/base.hcl"
  merge_strategy = "deep"
}
`,
				"b/base.hcl": `
inputs = { from_base = "b" }
`,
				"b/terragrunt.autoinclude.hcl": `
dependency "leak" {
  config_path  = "{{root}}/leak-target"
  skip_outputs = true
  mock_outputs = { val = "leaked" }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`,
			},
		},
		{
			name:      "exclude autoinclude wins",
			stackDeps: true,
			files: map[string]string{
				"terragrunt.hcl": `
exclude {
  if      = true
  actions = ["plan"]
}
`,
				"terragrunt.autoinclude.hcl": `
exclude {
  if      = true
  actions = ["apply"]
}
`,
			},
		},
		{
			name: "no autoinclude file",
			files: map[string]string{
				"terragrunt.hcl": `
terraform {
  source = "."
}

inputs = {
  name = "original"
}
`,
			},
		},
		{
			name: "autoinclude file",
			files: map[string]string{
				"terragrunt.hcl": `
terraform {
  source = "."
}

inputs = {
  name = "from-unit"
  keep = "unit-value"
}
`,
				"terragrunt.autoinclude.hcl": `
inputs = {
  name = "from-autoinclude"
  extra = "autoinclude-value"
}
`,
			},
		},
		{
			name: "stack level filename not merged into unit",
			files: map[string]string{
				"terragrunt.hcl": `
terraform {
  source = "."
}

inputs = {
  name = "from-unit"
}
`,
				"terragrunt.autoinclude.stack.hcl": `
inputs = {
  name = "from-stack-autoinclude-must-not-merge"
}
`,
			},
		},
		{
			name:      "remote state folds dependency mock output",
			stackDeps: true,
			command:   "init",
			files: map[string]string{
				"foo/terragrunt.hcl": ``,
				"terragrunt.hcl": `
remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    path = dependency.foo.outputs.val
  }
}
`,
				"terragrunt.autoinclude.hcl": `
dependency "foo" {
  config_path = "./foo"
  skip_outputs = true
  mock_outputs = {
    val = "autoinclude-mock-marker"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`,
			},
		},
		{
			name:      "autoinclude file parsed directly",
			stackDeps: true,
			command:   "init",
			cfgPath:   "terragrunt.autoinclude.hcl",
			files: map[string]string{
				"foo/terragrunt.hcl": ``,
				"terragrunt.autoinclude.hcl": `
dependency "foo" {
  config_path = "./foo"
  skip_outputs = true
  mock_outputs = {
    bar = "mocked"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`,
			},
		},
		{
			name:      "shallow input merge",
			stackDeps: true,
			files: map[string]string{
				"terragrunt.hcl": `
inputs = {
  tags      = { a = "1" }
  unit_only = "u"
}
`,
				"terragrunt.autoinclude.hcl": `
inputs = {
  tags    = { b = "2" }
  ai_only = "x"
}
`,
			},
		},
		{
			name:      "values placeholder for overridden inputs",
			stackDeps: true,
			files: map[string]string{
				"terragrunt.hcl": `
inputs = {
  vpc_id = values.vpc_id
  cidr   = values.cidr
}
`,
				"terragrunt.values.hcl": `
cidr = "10.0.0.0/16"
`,
				"terragrunt.autoinclude.hcl": `
inputs = {
  vpc_id = "from-autoinclude"
}
`,
			},
		},
		{
			name:      "overrides config path from values",
			stackDeps: true,
			command:   "init",
			values:    &regionValues,
			files: map[string]string{
				"vpc/terragrunt.hcl": ``,
				"terragrunt.hcl": `
dependency "vpc" {
  config_path  = values.vpc_path
  skip_outputs = true
  mock_outputs = {
    id = "from-unit"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}

inputs = {
  vpc_id = dependency.vpc.outputs.id
}
`,
				"terragrunt.autoinclude.hcl": `
dependency "vpc" {
  config_path  = "./vpc"
  skip_outputs = true
  mock_outputs = {
    id = "autoinclude-overrides-values-config-path"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`,
				"terragrunt.values.hcl": `
region = "us-east-1"
`,
			},
		},
		{
			name:      "overrides config path mixed deps",
			stackDeps: true,
			command:   "init",
			values:    &regionValues,
			files: map[string]string{
				"vpc/terragrunt.hcl": ``,
				"db/terragrunt.hcl":  ``,
				"terragrunt.hcl": `
dependency "vpc" {
  config_path  = values.vpc_path
  skip_outputs = true
  mock_outputs = {
    id = "unit-vpc"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}

dependency "db" {
  config_path  = "./db"
  skip_outputs = true
  mock_outputs = {
    id = "unit-db"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}

inputs = {
  vpc_id = dependency.vpc.outputs.id
  db_id  = dependency.db.outputs.id
}
`,
				"terragrunt.autoinclude.hcl": `
dependency "vpc" {
  config_path  = "./vpc"
  skip_outputs = true
  mock_outputs = {
    id = "autoinclude-vpc"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`,
			},
		},
		{
			name:      "non overridden failure still errors",
			stackDeps: true,
			command:   "init",
			values:    &regionValues,
			files: map[string]string{
				"other/terragrunt.hcl": ``,
				"terragrunt.hcl": `
dependency "vpc" {
  config_path  = values.vpc_path
  skip_outputs = true
  mock_outputs = {
    id = "unit-vpc"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`,
				"terragrunt.autoinclude.hcl": `
dependency "other" {
  config_path  = "./other"
  skip_outputs = true
  mock_outputs = {
    id = "autoinclude-other"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`,
			},
		},
		{
			name:      "missing values path without autoinclude still errors",
			stackDeps: true,
			command:   "init",
			values:    &regionValues,
			files: map[string]string{
				"terragrunt.hcl": `
dependency "vpc" {
  config_path  = values.vpc_path
  skip_outputs = true
  mock_outputs = {
    id = "unit-vpc"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}

inputs = {
  vpc_id = dependency.vpc.outputs.id
}
`,
			},
		},
		{
			name:      "overridden block is never evaluated",
			stackDeps: true,
			command:   "init",
			files: map[string]string{
				"vpc/terragrunt.hcl": ``,
				"terragrunt.hcl": `
dependency "vpc" {
  config_path  = "./vpc"
  skip_outputs = true
  mock_outputs = {
    id = "from-unit"
  }
  mock_outputs_allowed_terraform_commands = "init"
}

inputs = {
  vpc_id = dependency.vpc.outputs.id
}
`,
				"terragrunt.autoinclude.hcl": `
dependency "vpc" {
  config_path  = "./vpc"
  skip_outputs = true
  mock_outputs = {
    id = "from-autoinclude"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`,
			},
		},
		{
			name:      "expanded unit dep conflicts with unexpanded override",
			stackDeps: true,
			command:   "init",
			files: map[string]string{
				"vpc/terragrunt.hcl":     ``,
				"vpc-api/terragrunt.hcl": ``,
				"vpc-web/terragrunt.hcl": ``,
				"terragrunt.hcl": `
locals {
  services = toset(["api", "web"])
}

dependency "vpc" {
  expansion {
    for_each = local.services
  }

  config_path  = "./vpc-${each.value}"
  skip_outputs = true
  mock_outputs = {
    id = "unit-${each.value}"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`,
				"terragrunt.autoinclude.hcl": `
dependency "vpc" {
  config_path  = "./vpc"
  skip_outputs = true
  mock_outputs = {
    id = "from-autoinclude"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`,
			},
		},
		{
			name:      "expanded unit dep still errors",
			stackDeps: true,
			command:   "init",
			values:    &regionValues,
			files: map[string]string{
				"vpc/terragrunt.hcl": ``,
				"terragrunt.hcl": `
dependency "vpc" {
  expansion {
    for_each = values.regions
  }

  config_path  = "./vpc-${each.value}"
  skip_outputs = true
}
`,
				"terragrunt.autoinclude.hcl": `
dependency "vpc" {
  config_path  = "./vpc"
  skip_outputs = true
  mock_outputs = {
    id = "from-autoinclude"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")

			cfgPath := tc.cfgPath
			if cfgPath == "" {
				cfgPath = pkgconfig.DefaultTerragruntConfigPath
			}

			assertParity(t, parityCase{
				newFS:   memFS(root, tc.files),
				cfgPath: filepath.Join(root, cfgPath),
				configure: func(pctx *pkgconfig.ParsingContext) {
					if tc.stackDeps {
						pctx.Experiments.EnableExperiment(experiment.StackDependencies)
					}

					pctx.OriginalTerraformCommand = tc.command
					pctx.Values = tc.values
				},
			})
		})
	}
}

func TestParityDecodeForms(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		file    string
		content string
		drift   []drift
	}{
		{
			name:    "top level typo",
			content: `iam_rol = "arn:aws:iam::123456789012:role/unit"`,
		},
		{
			name:    "attribute named like a block",
			content: `terraform = {}`,
		},
		{
			name: "block named like an attribute",
			content: `
iam_role {}
`,
		},
		{
			name: "remote state typo",
			content: `
remote_state {
  backend     = "local"
  disable_int = true
  config      = {}
}
`,
			drift: []drift{driftSplitBodySuggestion},
		},
		{
			name: "engine typo",
			content: `
engine {
  source = "engine-source"
  vesion = "v1.0.0"
}
`,
			drift: []drift{driftSplitBodySuggestion},
		},
		{
			name:    "json typo",
			file:    pkgconfig.DefaultTerragruntJSONConfigPath,
			content: `{"iam_rol": "arn:aws:iam::123456789012:role/unit"}`,
		},
		{
			name:    "json engine shape",
			file:    pkgconfig.DefaultTerragruntJSONConfigPath,
			content: `{"engine": [5]}`,
			drift:   []drift{driftJSONShapeRepeated},
		},
		{
			name: "json remote state object",
			file: pkgconfig.DefaultTerragruntJSONConfigPath,
			content: `{
  "remote_state": {
    "backend": "local",
    "config": {"path": "terraform.tfstate"}
  }
}`,
		},
		{
			name: "json generate object",
			file: pkgconfig.DefaultTerragruntJSONConfigPath,
			content: `{
  "generate": {
    "provider": {
      "path": "provider.tf",
      "if_exists": "overwrite",
      "contents": "provider"
    }
  }
}`,
		},
		{
			name: "attribute forms",
			content: `
remote_state = {
  backend = "local"
  config  = { path = "terraform.tfstate" }
}

generate = {
  provider = {
    path      = "provider.tf"
    if_exists = "overwrite"
    contents  = "provider"
  }
}
`,
		},
		{
			name: "attribute and block forms",
			content: `
remote_state = {
  backend = "local"
  config  = { path = "attr.tfstate" }
}

remote_state {
  backend = "local"
  config  = { path = "block.tfstate" }
}

generate = {
  provider = {
    path      = "provider.tf"
    if_exists = "overwrite"
    contents  = "attr"
  }
}

generate "backend" {
  path      = "backend.tf"
  if_exists = "overwrite"
  contents  = "block"
}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")

			file := tc.file
			if file == "" {
				file = pkgconfig.DefaultTerragruntConfigPath
			}

			assertParity(t, parityCase{
				newFS:     memFS(root, map[string]string{file: tc.content}),
				cfgPath:   filepath.Join(root, file),
				configure: func(*pkgconfig.ParsingContext) {},
				drift:     tc.drift,
			})
		})
	}
}
