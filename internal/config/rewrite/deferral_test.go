package config_test

import (
	"maps"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zclconf/go-cty/cty"

	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// fetches is when a parse case expects tofu to run for dependency outputs.
type fetches int

const (
	// fetchesInToV1 runs tofu in ToV1 and never in the parse.
	fetchesInToV1 fetches = iota
	// fetchesInParse runs tofu in the parse, where pkg/config parses an exposed include, and never in ToV1.
	fetchesInParse
	// fetchesNowhere runs no tofu.
	fetchesNowhere
)

// targets are the dependency targets every deferral case can point at. Each needs a module for the output run to
// download.
var targets = map[string]string{
	"vpc/terragrunt.hcl":    ``,
	"vpc/main.tf":           ``,
	"db/terragrunt.hcl":     ``,
	"db/main.tf":            ``,
	"broken/terragrunt.hcl": ``,
	"broken/main.tf":        ``,
}

// targetOutputs is the `tofu output -json` of each target, keyed by directory relative to the case root. broken
// reports no outputs, which pkg/config reads as a target that was never applied.
var targetOutputs = map[string]string{
	"vpc":    outputsJSON(map[string]string{"id": "vpc-123", "path": "../db"}),
	"db":     outputsJSON(map[string]string{"id": "db-456"}),
	"broken": `{}`,
}

// vpcOutputs is the `dependency` value of a run that resolved the vpc target.
var vpcOutputs = cty.ObjectVal(map[string]cty.Value{
	"vpc": cty.ObjectVal(map[string]cty.Value{
		"outputs": cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("vpc-123")}),
	}),
})

func TestParityDeferral(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files        map[string]string
		runDeps      *cty.Value
		name         string
		cfgPath      string
		fetches      fetches
		skipOutput   bool
		wantParseErr bool
		wantToV1Err  bool
	}{
		{
			name: "unit inputs read an output",
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.id
  name   = "app"
}
`,
			},
		},
		{
			name:  "root include reads an output with shallow merge",
			files: rootIncludeFiles("shallow"),
		},
		{
			name:  "root include reads an output with deep merge",
			files: rootIncludeFiles("deep"),
		},
		{
			name:  "root include reads an output with no merge",
			files: rootIncludeFiles("no_merge"),
		},
		{
			name: "unit overrides the config path of an envcommon dependency",
			files: map[string]string{
				"_envcommon/app.hcl": `
dependency "vpc" {
  config_path = "../vpc-default"

  mock_outputs = {
    id = "mock-vpc"
  }
}

inputs = {
  vpc_id = dependency.vpc.outputs.id
  env    = "common"
}
`,
				"app/terragrunt.hcl": `
include "envcommon" {
  path           = "../_envcommon/app.hcl"
  merge_strategy = "deep"
}

dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  env = "app"
}
`,
			},
		},
		{
			name: "multiple includes with overlapping inputs",
			files: map[string]string{
				"a.hcl": `
inputs = {
  from_a = dependency.vpc.outputs.id
  shared = "a"
}
`,
				"b.hcl": `
inputs = {
  from_b = dependency.db.outputs.id
  shared = "b"
}
`,
				"app/terragrunt.hcl": `
include "a" {
  path           = "../a.hcl"
  merge_strategy = "deep"
}

include "b" {
  path           = "../b.hcl"
  merge_strategy = "shallow"
}

dependency "vpc" {
  config_path = "../vpc"
}

dependency "db" {
  config_path = "../db"
}

inputs = {
  shared = "app"
}
`,
			},
		},
		{
			name: "autoinclude with its own dependency and inputs",
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.id
  name   = "unit"
}
`,
				"app/terragrunt.autoinclude.hcl": `
dependency "db" {
  config_path = "../db"
}

inputs = {
  db_id = dependency.db.outputs.id
  name  = "autoinclude"
}
`,
			},
		},
		{
			name:         "inputs read dependency inputs",
			fetches:      fetchesNowhere,
			wantParseErr: true,
			wantToV1Err:  true,
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_name = dependency.vpc.inputs.name
}
`,
			},
		},
		{
			name: "read_terragrunt_config inside deferred inputs",
			files: map[string]string{
				"common.hcl": `
inputs = {
  region = "us-east-1"
}
`,
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.id
  common = read_terragrunt_config("../common.hcl").inputs
}
`,
			},
		},
		{
			name:        "unknown output key",
			wantToV1Err: true,
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.missing
}
`,
			},
		},
		{
			name:        "run-supplied dependency value with an unknown output key",
			runDeps:     new(vpcOutputs),
			fetches:     fetchesNowhere,
			wantToV1Err: true,
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.missing
}
`,
			},
		},
		{
			name:        "missing local inside deferred inputs",
			wantToV1Err: true,
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.id
  bad    = local.missing
}
`,
			},
		},
		{
			name:        "unreferenced dependency that was never applied",
			wantToV1Err: true,
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "broken" {
  config_path = "../broken"
}

inputs = {
  name = "app"
}
`,
			},
		},
		{
			name:        "failed resolution leaves the include to resolve its own",
			wantToV1Err: true,
			files: map[string]string{
				"root.hcl": `
dependency "vpc" {
  config_path = "{{root}}/vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.id
}
`,
				"app/terragrunt.hcl": `
include "root" {
  path = "../root.hcl"
}

dependency "broken" {
  config_path = "../broken"
}
`,
			},
		},
		{
			name:         "config path from another dependency output",
			fetches:      fetchesNowhere,
			wantParseErr: true,
			wantToV1Err:  true,
			files:        chainedDependencyFiles,
		},
		{
			name:         "config path from another dependency output under validation",
			fetches:      fetchesNowhere,
			skipOutput:   true,
			wantParseErr: true,
			wantToV1Err:  true,
			files:        chainedDependencyFiles,
		},
		{
			name: "hook reads an output beside one that does not",
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

terraform {
  source = "."

  before_hook "plain" {
    commands = ["plan"]
    execute  = ["echo", "plain"]
  }

  before_hook "dep" {
    commands = ["plan"]
    execute  = ["echo", dependency.vpc.outputs.id]
  }
}
`,
			},
		},
		{
			name: "extra arguments read an output",
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

terraform {
  extra_arguments "vars" {
    commands  = ["plan"]
    arguments = ["-var=vpc_id=${dependency.vpc.outputs.id}"]
  }
}
`,
			},
		},
		{
			name: "remote state config reads an output",
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

remote_state {
  backend = "local"
  config = {
    path = "${dependency.vpc.outputs.id}.tfstate"
  }
}
`,
			},
		},
		{
			name: "remote state attribute reads an output",
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

remote_state = {
  backend = "local"
  config = {
    path = "${dependency.vpc.outputs.id}.tfstate"
  }
}
`,
			},
		},
		{
			name: "engine meta reads an output",
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

engine {
  source = "engine-source"
  meta = {
    vpc_id = dependency.vpc.outputs.id
  }
}
`,
			},
		},
		{
			name: "generate block reads an output",
			files: map[string]string{
				"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite"
  contents  = "provider \"aws\" { vpc = \"${dependency.vpc.outputs.id}\" }"
}
`,
			},
		},
		{
			name:    "child reads the inputs of an exposed include",
			fetches: fetchesInParse,
			files: map[string]string{
				"root.hcl": `
dependency "vpc" {
  config_path = "{{root}}/vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.id
}
`,
				"app/terragrunt.hcl": `
include "root" {
  path   = "../root.hcl"
  expose = true
}

inputs = {
  from_root = include.root.inputs.vpc_id
}
`,
			},
		},
		{
			name: "child reads the dependency config path of an exposed include",
			files: map[string]string{
				"vpc_dep.hcl": `
dependency "vpc" {
  config_path  = "{{root}}/vpc"
  skip_outputs = true

  mock_outputs = {
    id = "mock-vpc"
  }
}
`,
				"app/terragrunt.hcl": `
include "vpc_dep" {
  path   = "../vpc_dep.hcl"
  expose = true
}

dependency "db" {
  config_path = "../db"
}

inputs = {
  vpc_path = include.vpc_dep.dependency.vpc.config_path
  db_id    = dependency.db.outputs.id
}
`,
			},
		},
		{
			name:    "JSON config",
			cfgPath: "app/terragrunt.hcl.json",
			files: map[string]string{
				"app/terragrunt.hcl.json": `{
  "dependency": {
    "vpc": {
      "config_path": "../vpc"
    }
  },
  "inputs": {
    "vpc_id": "${dependency.vpc.outputs.id}"
  }
}`,
			},
		},
		{
			name:    "JSON hook reads an output",
			cfgPath: "app/terragrunt.hcl.json",
			files: map[string]string{
				"app/terragrunt.hcl.json": `{
  "dependency": {
    "vpc": {
      "config_path": "../vpc"
    }
  },
  "terraform": {
    "before_hook": {
      "dep": {
        "commands": ["plan"],
        "execute": ["echo", "${dependency.vpc.outputs.id}"]
      }
    }
  }
}`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/live")

			files := maps.Clone(targets)
			maps.Copy(files, tc.files)

			outputs := map[string]string{}
			for dir, out := range targetOutputs {
				outputs[filepath.Join(root, dir)] = out
			}

			cfgPath := tc.cfgPath
			if cfgPath == "" {
				cfgPath = "app/" + pkgconfig.DefaultTerragruntConfigPath
			}

			result := assertParity(t, parityCase{
				newFS:   memFS(root, files),
				cfgPath: filepath.Join(root, filepath.FromSlash(cfgPath)),
				configure: func(pctx *pkgconfig.ParsingContext) {
					pctx.SkipOutput = tc.skipOutput
					pctx.DecodedDependencies = tc.runDeps
				},
				outputs: outputs,
			})

			assert.Equal(
				t,
				tc.wantParseErr,
				result.parseErr != nil,
				"parse error: %v",
				result.parseErr,
			)

			if result.parsed != nil {
				assert.Equal(
					t,
					tc.wantToV1Err,
					result.toV1Err != nil,
					"ToV1 error: %v",
					result.toV1Err,
				)
			}

			switch tc.fetches {
			case fetchesInToV1:
				assert.Zero(t, result.parseExecs, "the parse ran tofu")
				assert.Positive(t, result.toV1Execs, "ToV1 ran no tofu")
			case fetchesInParse:
				assert.Positive(t, result.parseExecs, "the parse ran no tofu")
				assert.Zero(t, result.toV1Execs, "ToV1 ran tofu")
			case fetchesNowhere:
				assert.Zero(t, result.parseExecs, "the parse ran tofu")
				assert.Zero(t, result.toV1Execs, "ToV1 ran tofu")
			}
		})
	}
}

// chainedDependencyFiles is a unit whose second dependency takes its config path from an output of the first.
var chainedDependencyFiles = map[string]string{
	"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

dependency "db" {
  config_path = dependency.vpc.outputs.path
}

inputs = {
  vpc_id = dependency.vpc.outputs.id
}
`,
}

// rootIncludeFiles returns a unit that includes a root file with strategy. The root file reads an output of a
// dependency the unit declares.
func rootIncludeFiles(strategy string) map[string]string {
	return map[string]string{
		"root.hcl": `
inputs = {
  vpc_id = dependency.vpc.outputs.id
  region = "us-east-1"
}
`,
		"app/terragrunt.hcl": `
include "root" {
  path           = "../root.hcl"
  merge_strategy = "` + strategy + `"
}

dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  name = "app"
}
`,
	}
}
