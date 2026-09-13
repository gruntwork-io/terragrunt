package cli_test

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/require"
)

// The block-iteration tests run the CLI against an in-memory tree built per test, and a
// lifecycle transition is an edit to that tree. The live configuration sits under live, and
// the modules and units it points at sit beside it.
var blockIterationRoot = venvtest.Root("/block-iteration")

const (
	generatedStackDir = ".terragrunt-stack"
	generatedValues   = "terragrunt.values.hcl"

	// sweepOrphans deletes the generated tree before regenerating it, the same sweep
	// `terragrunt stack clean` performs.
	sweepOrphans = "--source-update"

	configFileMode = 0o644
)

var liveDir = filepath.Join(blockIterationRoot, "live")

// experimentGate says whether a run opts into block-iteration, so a test that leaves the
// gate closed on purpose reads differently from one that forgot the flag.
type experimentGate int

const (
	gateClosed experimentGate = iota
	gateOpen
)

func (gate experimentGate) args() []string {
	if gate == gateOpen {
		return []string{"--experiment", "block-iteration"}
	}

	return nil
}

const (
	appModuleConfig = `inputs = {
  role = values.role
}
`

	appModuleMain = `variable "role" {
  type = string
}

output "role" {
  value = var.role
}
`

	teamStackConfig = `unit "member" {
  source = "./app"
  path   = "member"

  values = {
    role = values.role
  }
}
`
)

// Unit block variants, each a whole terragrunt.stack.hcl pointing at the app module.
const (
	unitStatic = `unit "aurora" {
  source = "../modules/app"
  path   = "aurora"

  values = {
    role = "aurora"
  }
}
`

	unitDisabled = `unit "aurora" {
  enabled = false

  source = "../modules/app"
  path   = "aurora"

  values = {
    role = "aurora"
  }
}
`

	unitForEachSet = `unit "aurora" {
  expansion {
    for_each = toset(["api", "web"])
  }

  source = "../modules/app"
  path   = "aurora/${each.key}"

  values = {
    role = each.key
  }
}
`

	unitForEachSetShrunk = `unit "aurora" {
  expansion {
    for_each = toset(["api"])
  }

  source = "../modules/app"
  path   = "aurora/${each.key}"

  values = {
    role = each.key
  }
}
`

	unitForEachMap = `unit "aurora" {
  expansion {
    for_each = {
      api = "backend"
      web = "frontend"
    }
  }

  source = "../modules/app"
  path   = "aurora/${each.key}"

  values = {
    role = each.value
  }
}
`

	unitCount = `locals {
  roles = ["alpha", "beta", "gamma"]
}

unit "aurora" {
  expansion {
    count = length(local.roles)
  }

  source = "../modules/app"
  path   = "aurora/${count.index}"

  values = {
    role = local.roles[count.index]
  }
}
`

	unitCountShrunk = `locals {
  roles = ["alpha", "gamma"]
}

unit "aurora" {
  expansion {
    count = length(local.roles)
  }

  source = "../modules/app"
  path   = "aurora/${count.index}"

  values = {
    role = local.roles[count.index]
  }
}
`
)

// Stack block variants, each a whole terragrunt.stack.hcl pointing at the team stack.
const (
	stackStatic = `stack "team" {
  source = "../modules/team"
  path   = "team"

  values = {
    role = "team"
  }
}
`

	stackDisabled = `stack "team" {
  enabled = false

  source = "../modules/team"
  path   = "team"

  values = {
    role = "team"
  }
}
`

	stackForEachSet = `stack "team" {
  expansion {
    for_each = toset(["east", "west"])
  }

  source = "../modules/team"
  path   = "team/${each.key}"

  values = {
    role = each.key
  }
}
`

	stackForEachSetShrunk = `stack "team" {
  expansion {
    for_each = toset(["east"])
  }

  source = "../modules/team"
  path   = "team/${each.key}"

  values = {
    role = each.key
  }
}
`

	stackForEachMap = `stack "team" {
  expansion {
    for_each = {
      east = "primary"
      west = "secondary"
    }
  }

  source = "../modules/team"
  path   = "team/${each.key}"

  values = {
    role = each.value
  }
}
`

	stackCount = `locals {
  roles = ["alpha", "beta", "gamma"]
}

stack "team" {
  expansion {
    count = length(local.roles)
  }

  source = "../modules/team"
  path   = "team/${count.index}"

  values = {
    role = local.roles[count.index]
  }
}
`

	stackCountShrunk = `locals {
  roles = ["alpha", "gamma"]
}

stack "team" {
  expansion {
    count = length(local.roles)
  }

  source = "../modules/team"
  path   = "team/${count.index}"

  values = {
    role = local.roles[count.index]
  }
}
`
)

// Dependency block variants, each a whole terragrunt.hcl pointing at the aurora units. Every
// block skips outputs and mocks them, so the address a reference resolves to shows up in the
// rendered inputs without a tofu binary.
const (
	dependencyStatic = `dependency "aurora" {
  config_path  = "../aurora-web"
  skip_outputs = true

  mock_outputs = {
    id = "aurora-web"
  }
}

inputs = {
  addresses = sort(keys(dependency))
  aurora_id = dependency.aurora.outputs.id
}
`

	dependencyDisabled = `dependency "aurora" {
  enabled     = false
  config_path = "../aurora-web"

  mock_outputs = {
    id = "aurora-web-id"
  }
}

inputs = {
  addresses = sort(keys(dependency))
  aurora_id = dependency.aurora.outputs.id
}
`

	dependencyForEachSet = `dependency "aurora" {
  expansion {
    for_each = toset(["api", "web"])
  }

  config_path  = "../aurora-${each.key}"
  skip_outputs = true

  mock_outputs = {
    id = "aurora-${each.key}"
  }
}

inputs = {
  addresses   = sort(keys(dependency))
  aurora_keys = sort(keys(dependency.aurora))
  ids         = { for key, instance in dependency.aurora : key => instance.outputs.id }
}
`

	dependencyForEachMap = `dependency "aurora" {
  expansion {
    for_each = {
      api = "backend"
      web = "frontend"
    }
  }

  config_path  = "../aurora-${each.key}"
  skip_outputs = true

  mock_outputs = {
    id = each.value
  }
}

inputs = {
  aurora_keys = sort(keys(dependency.aurora))
  ids         = { for key, instance in dependency.aurora : key => instance.outputs.id }
}
`

	dependencyCount = `locals {
  shards = ["web", "api", "edge"]
}

dependency "aurora" {
  expansion {
    count = length(local.shards)
  }

  config_path  = "../aurora-${local.shards[count.index]}"
  skip_outputs = true

  mock_outputs = {
    id = local.shards[count.index]
  }
}

inputs = {
  aurora_keys = sort(keys(dependency.aurora))
  ids         = { for key, instance in dependency.aurora : key => instance.outputs.id }
}
`

	dependencyCountShrunk = `locals {
  shards = ["web", "edge"]
}

dependency "aurora" {
  expansion {
    count = length(local.shards)
  }

  config_path  = "../aurora-${local.shards[count.index]}"
  skip_outputs = true

  mock_outputs = {
    id = local.shards[count.index]
  }
}

inputs = {
  aurora_keys = sort(keys(dependency.aurora))
  ids         = { for key, instance in dependency.aurora : key => instance.outputs.id }
}
`

	dependencyFanOut = `dependency "aurora" {
  expansion {
    count = 24
  }

  config_path  = "../aurora-web"
  skip_outputs = true

  mock_outputs = {
    id = "instance-${count.index}"
  }
}

inputs = {
  ids = { for key, instance in dependency.aurora : key => instance.outputs.id }
}
`

	// dependencyParity declares one disabled dependency beside one expanded dependency, so
	// the two spellings can be compared in a single set of inputs.
	dependencyParity = `dependency "vpc" {
  enabled     = false
  config_path = "../vpc"

  mock_outputs = {
    id = "vpc-id"
  }
}

dependency "aurora" {
  expansion {
    for_each = toset(["api", "web"])
  }

  config_path  = "../aurora-${each.key}"
  skip_outputs = true

  mock_outputs = {
    id = "aurora-${each.key}-id"
  }
}

inputs = {
  addresses   = sort(keys(dependency))
  aurora_keys = sort(keys(dependency.aurora))
  vpc_id      = dependency.vpc.outputs.id
  web_id      = dependency.aurora["web"].outputs.id
}
`
)

func unitTree(stackConfig string) tree {
	return tree{}.
		file(filepath.Join("live", config.DefaultStackFile), stackConfig).
		file(filepath.Join("modules", "app", config.DefaultTerragruntConfigPath), appModuleConfig).
		file(filepath.Join("modules", "app", "main.tf"), appModuleMain)
}

func stackTree(stackConfig string) tree {
	return tree{}.
		file(filepath.Join("live", config.DefaultStackFile), stackConfig).
		file(filepath.Join("modules", "team", config.DefaultStackFile), teamStackConfig).
		file(filepath.Join("modules", "team", "app", config.DefaultTerragruntConfigPath), appModuleConfig).
		file(filepath.Join("modules", "team", "app", "main.tf"), appModuleMain)
}

func dependencyTree(unitConfig string) tree {
	return tree{}.
		file(filepath.Join("live", config.DefaultTerragruntConfigPath), unitConfig).
		config("aurora-web", "").
		config("aurora-api", "").
		config("aurora-edge", "")
}

func blockIterationVenv(t *testing.T, files tree) (*venv.Venv, vfs.FS) {
	t.Helper()

	fsys := venvtest.NewFS(t, blockIterationRoot, files)

	return venvtest.New().WithFS(fsys), fsys
}

// switchConfig overwrites the live configuration file named name with contents, standing in
// for the edit a user makes when a block goes static to dynamic or back.
func switchConfig(t *testing.T, fsys vfs.FS, name, contents string) {
	t.Helper()

	require.NoError(
		t,
		vfs.WriteFile(fsys, filepath.Join(liveDir, name), []byte(contents), configFileMode),
	)
}

func generateStack(t *testing.T, v *venv.Venv, args ...string) {
	t.Helper()

	_, err := runCLI(t, v, slices.Concat(
		[]string{"stack", "generate", "--no-color", "--working-dir", liveDir},
		gateOpen.args(),
		args,
	)...)
	require.NoError(t, err)
}

// generatedUnits returns the slash-separated path of every generated unit, relative to the
// generated stack directory. Units left behind by an earlier generation are included, since
// an orphan is only visible against the whole set.
//
// A unit is identified by its generated values file. A generated stack root has one too when
// its stack block passes values down, so those are skipped by the stack file beside them.
func generatedUnits(t *testing.T, fsys vfs.FS) []string {
	t.Helper()

	stackDir := filepath.Join(liveDir, generatedStackDir)

	units := []string{}

	// A sweep with nothing left to regenerate removes the directory outright.
	if !vfs.Exists(fsys, stackDir) {
		return units
	}

	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || entry.Name() != generatedValues {
			return nil
		}

		dir := filepath.Dir(path)

		if vfs.Exists(fsys, filepath.Join(dir, config.DefaultStackFile)) {
			return nil
		}

		rel, err := filepath.Rel(stackDir, dir)
		if err != nil {
			return err
		}

		units = append(units, filepath.ToSlash(rel))

		return nil
	}

	require.NoError(t, vfs.WalkDir(fsys, stackDir, walk))

	slices.Sort(units)

	return units
}

// generatedRoles returns each generated unit's role value, keyed by the unit's path. A
// regeneration rewrites the role and an orphan keeps its old one, so the map tells the two
// apart across a transition.
func generatedRoles(t *testing.T, fsys vfs.FS) map[string]string {
	t.Helper()

	roles := map[string]string{}

	for _, unit := range generatedUnits(t, fsys) {
		values, err := vfs.ReadFile(
			fsys,
			filepath.Join(liveDir, generatedStackDir, filepath.FromSlash(unit), generatedValues),
		)
		require.NoError(t, err)

		_, role, found := strings.Cut(string(values), `role = "`)
		require.Truef(t, found, "unit %s generated no role value", unit)

		role, _, _ = strings.Cut(role, `"`)
		roles[unit] = role
	}

	return roles
}

// renderedInputs returns the inputs the live configuration resolves to. Inputs are where a
// dependency address becomes observable: a reference that resolves proves the address exists,
// and its value proves which instance it reached.
func renderedInputs(t *testing.T, v *venv.Venv, gate experimentGate) map[string]any {
	t.Helper()

	stdout, err := runCLI(t, v, slices.Concat(
		[]string{"render", "--format", "json", "--no-color", "--working-dir", liveDir},
		gate.args(),
	)...)
	require.NoError(t, err)

	rendered := struct {
		Inputs map[string]any `json:"inputs"`
	}{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &rendered))

	return rendered.Inputs
}
