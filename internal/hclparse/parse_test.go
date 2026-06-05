package hclparse_test

import (
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

const (
	testStackDir = "/test/stack"
	testGenDir   = "/test/stack/.terragrunt-stack/app"
)

func TestParseStackFile_SimpleUnits(t *testing.T) {
	t.Parallel()

	src := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

unit "db" {
  source = "../catalog/units/db"
  path   = "db"
}
`

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)
	require.Len(t, result.Units, 2)
	assert.Equal(t, "vpc", result.Units[0].Name)

	assert.Equal(t, "vpc", result.Units[0].Path)
	assert.Equal(t, "db", result.Units[1].Name)
	assert.Equal(t, "db", result.Units[1].Path)
	assert.Empty(t, result.AutoIncludes)
}

func TestParseStackFile_UnitWithAutoInclude(t *testing.T) {
	t.Parallel()

	src := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

unit "db" {
  source = "../catalog/units/db"
  path   = "db"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }
  }
}
`

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)
	require.Len(t, result.Units, 2)

	assert.NotContains(t, result.AutoIncludes, "vpc")

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "db")]
	require.True(t, ok)
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, "vpc", resolved.Dependencies[0].Name)
	assert.Equal(t, filepath.Join(testStackDir, ".terragrunt-stack", "vpc"), resolved.Dependencies[0].ConfigPath)
	assert.NotNil(t, resolved.RawBody)
}

func TestParseStackFile_MultipleDependencies(t *testing.T) {
	t.Parallel()

	src := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

unit "rds" {
  source = "../catalog/units/rds"
  path   = "rds"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }

    dependency "rds" {
      config_path = unit.rds.path
    }
  }
}
`

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok)
	require.Len(t, resolved.Dependencies, 2)
	assert.Equal(t, "vpc", resolved.Dependencies[0].Name)
	assert.Equal(t, "rds", resolved.Dependencies[1].Name)
}

func TestParseStackFile_StackRefInAutoInclude(t *testing.T) {
	t.Parallel()

	src := `
stack "networking" {
  source = "../stacks/networking"
  path   = "networking"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "networking" {
      config_path = stack.networking.path
    }
  }
}
`

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok)
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, "networking", resolved.Dependencies[0].Name)
	assert.Equal(t, filepath.Join(testStackDir, ".terragrunt-stack", "networking"), resolved.Dependencies[0].ConfigPath)
}

func TestParseStackFile_NoAutoInclude(t *testing.T) {
	t.Parallel()

	src := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}
`

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)
	require.Len(t, result.Units, 1)
	assert.Empty(t, result.AutoIncludes)
}

func TestParseStackFile_InvalidEvaluatedLocalReferenceIsEvalError(t *testing.T) {
	t.Parallel()

	src := `
locals {
  base   = {}
  broken = local.base.missing
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"
}
`

	_, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.Error(t, err)

	var evalErr hclparse.LocalEvalError
	require.ErrorAs(t, err, &evalErr)
	assert.Equal(t, "broken", evalErr.Name)
	assert.Contains(t, err.Error(), "Unsupported attribute")

	var cycleErr hclparse.LocalsCycleError
	assert.NotErrorAs(t, err, &cycleErr)
}

func TestGenerateAutoIncludeFile_NilResolved(t *testing.T) {
	t.Parallel()

	err := hclparse.GenerateAutoIncludeFile(vfs.NewMemMapFS(), nil, testGenDir, nil, nil)
	assert.NoError(t, err)
}

func TestGenerateAutoIncludeFile_FullFlow(t *testing.T) {
	t.Parallel()

	src := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path

      mock_outputs_allowed_terraform_commands = ["plan"]
      mock_outputs = {
        val = "fake-val"
      }
    }

    inputs = {
      val = dependency.vpc.outputs.val
    }
  }
}
`
	srcBytes := []byte(src)
	fs := vfs.NewMemMapFS()

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: srcBytes, Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok)

	appDir := testGenDir

	err = hclparse.GenerateAutoIncludeFile(fs, resolved, appDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	generated, err := vfs.ReadFile(fs, filepath.Join(appDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)

	content := string(generated)
	assert.Contains(t, content, "Generated by Terragrunt")
	assert.Contains(t, content, `dependency "vpc"`)
	assert.Contains(t, content, "../vpc")
	assert.Contains(t, content, "mock_outputs_allowed_terraform_commands")
	assert.Contains(t, content, `"fake-val"`)
	assert.Contains(t, content, "dependency.vpc.outputs.val")
}

// TestGenerateAutoIncludeFile_MockOutputsResolvesLocal pins that a dependency's mock_outputs resolves
// stack-level references (local.*) to literals at generate time.
func TestGenerateAutoIncludeFile_MockOutputsResolvesLocal(t *testing.T) {
	t.Parallel()

	src := `
locals {
  account = {
    name   = "my-account"
    region = "eu-west-1"
  }
}

unit "account" {
  source = "../catalog/units/account"
  path   = "account"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "account" {
      config_path = unit.account.path

      mock_outputs = {
        name   = local.account.name
        region = local.account.region
      }
    }
  }
}
`
	srcBytes := []byte(src)
	fs := vfs.NewMemMapFS()

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: srcBytes, Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok)

	err = hclparse.GenerateAutoIncludeFile(fs, resolved, testGenDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	generated, err := vfs.ReadFile(fs, filepath.Join(testGenDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)

	content := string(generated)
	// Dependency path: config_path = unit.account.path resolves to the sibling unit at generate time.
	assert.Contains(t, content, `"../account"`, "the dependency config_path (unit.<name>.path) must resolve at generate time")
	// Dependency mock outputs: stack-level locals resolve to literals at generate time.
	assert.Contains(t, content, `"my-account"`, "a local in mock_outputs must resolve at generate time")
	assert.Contains(t, content, `"eu-west-1"`, "a local in mock_outputs must resolve at generate time")
	assert.NotContains(t, content, "local.account.name", "the local reference must not be left literal")
	assert.NotContains(t, content, "local.account.region", "the local reference must not be left literal")
}

func TestGenerateAutoIncludeFile_MultipleDeps(t *testing.T) {
	t.Parallel()

	src := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

unit "rds" {
  source = "../catalog/units/rds"
  path   = "rds"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }

    dependency "rds" {
      config_path = unit.rds.path
    }

    inputs = {
      vpc_id = dependency.vpc.outputs.vpc_id
      db_url = dependency.rds.outputs.endpoint
    }
  }
}
`
	srcBytes := []byte(src)
	fs := vfs.NewMemMapFS()

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: srcBytes, Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.NotNil(t, resolved)

	appDir := testGenDir

	err = hclparse.GenerateAutoIncludeFile(fs, resolved, appDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	generated, err := vfs.ReadFile(fs, filepath.Join(appDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)

	content := string(generated)
	assert.Contains(t, content, `dependency "vpc"`)
	assert.Contains(t, content, `dependency "rds"`)
	assert.Contains(t, content, "dependency.vpc.outputs.vpc_id")
	assert.Contains(t, content, "dependency.rds.outputs.endpoint")
}

func TestGenerateAutoIncludeFile_RelativePath(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	src := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }

    inputs = {
      val = dependency.vpc.outputs.val
    }
  }
}
`
	srcBytes := []byte(src)

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: srcBytes, Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.NotNil(t, resolved)

	appDir := filepath.Join(testStackDir, ".terragrunt-stack", "app")

	err = hclparse.GenerateAutoIncludeFile(fs, resolved, appDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	generated, err := vfs.ReadFile(fs, filepath.Join(appDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)

	content := string(generated)
	assert.Contains(t, content, `dependency "vpc"`)
	assert.Contains(t, content, "../vpc")
	assert.NotContains(t, content, testStackDir)
}

func TestGenerateAutoIncludeFile_LocalsEvaluated(t *testing.T) {
	t.Parallel()

	src := `
locals {
  env = "production"
}

unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }

    env_name = local.env

    inputs = {
      vpc_id = dependency.vpc.outputs.vpc_id
    }
  }
}
`
	srcBytes := []byte(src)
	fs := vfs.NewMemMapFS()

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: srcBytes, Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.NotNil(t, resolved)

	appDir := testGenDir

	err = hclparse.GenerateAutoIncludeFile(fs, resolved, appDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	generated, err := vfs.ReadFile(fs, filepath.Join(appDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)

	content := string(generated)
	assert.Contains(t, content, "production")
	assert.NotContains(t, content, "local.env")
	assert.Contains(t, content, "dependency.vpc.outputs.vpc_id")
}

func TestGenerateAutoIncludeFile_PartialEval(t *testing.T) {
	t.Parallel()

	src := `
locals {
  env    = "production"
  region = "us-east-1"
}

unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path

      mock_outputs_allowed_terraform_commands = ["plan"]
      mock_outputs = {
        vpc_id = "mock-vpc-id"
      }
    }

    inputs = {
      env    = local.env
      region = local.region
      vpc_id = dependency.vpc.outputs.vpc_id
    }

    env_label = local.env

    name_tag = "${local.env}-${dependency.vpc.outputs.vpc_id}-app"
  }
}
`
	srcBytes := []byte(src)
	fs := vfs.NewMemMapFS()

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: srcBytes, Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok)

	appDir := testGenDir

	err = hclparse.GenerateAutoIncludeFile(fs, resolved, appDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	generated, err := vfs.ReadFile(fs, filepath.Join(appDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)

	content := string(generated)

	assert.Contains(t, content, `"production"`)
	assert.NotContains(t, content, "local.env")
	assert.Contains(t, content, `"us-east-1"`)
	assert.NotContains(t, content, "local.region")
	assert.Contains(t, content, "dependency.vpc.outputs.vpc_id")
	assert.Contains(t, content, "production-${dependency.vpc.outputs.vpc_id}-app")
	assert.Contains(t, content, `dependency "vpc"`)
	assert.Contains(t, content, "mock_outputs")
}

func TestParseStackFile_LocalsCycle(t *testing.T) {
	t.Parallel()

	src := `
locals {
  a = local.b
  b = local.a
}

unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}
`

	_, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.Error(t, err)

	var cycleErr hclparse.LocalsCycleError
	require.ErrorAs(t, err, &cycleErr)
	assert.Len(t, cycleErr.Names, 2)
}

func TestParseStackFile_LocalsCannotReferenceUnit(t *testing.T) {
	t.Parallel()

	src := `
locals {
  vpc_path = unit.vpc.path
}

unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}
`

	_, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{
		Src:      []byte(src),
		Filename: "terragrunt.stack.hcl",
		StackDir: testStackDir,
	})

	var localErr hclparse.LocalEvalError

	require.ErrorAs(t, err, &localErr)
	assert.Equal(t, "vpc_path", localErr.Name)
	assert.Contains(t, localErr.Error(), `There is no variable named "unit"`)
}

func TestParseStackFile_MultipleLocals(t *testing.T) {
	t.Parallel()

	src := `
locals {
  env    = "staging"
  region = "eu-west-1"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.app.path
    }

    env_tag    = local.env
    region_tag = local.region
  }
}
`
	srcBytes := []byte(src)
	fs := vfs.NewMemMapFS()

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: srcBytes, Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.NotNil(t, resolved)

	appDir := testGenDir

	err = hclparse.GenerateAutoIncludeFile(fs, resolved, appDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	generated, err := vfs.ReadFile(fs, filepath.Join(appDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)

	content := string(generated)
	assert.Contains(t, content, "staging")
	assert.Contains(t, content, "eu-west-1")
	assert.NotContains(t, content, "local.env")
	assert.NotContains(t, content, "local.region")
}

func TestParseStackFile_StackAutoInclude(t *testing.T) {
	t.Parallel()

	// A stack autoinclude injects valid terragrunt.stack.hcl content (unit/stack blocks), since stacks do not have dependencies.
	src := `
stack "networking" {
  source = "../stacks/networking"
  path   = "networking"

  autoinclude {
    unit "extra" {
      source = "../catalog/units/extra"
      path   = "extra"
    }
  }
}
`

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("stack", "networking")]
	require.True(t, ok, "stack networking should have autoinclude")
	assert.Equal(t, hclparse.KindStack, resolved.Kind, "Kind must be KindStack so the generator picks terragrunt.autoinclude.stack.hcl")
}

// A stack autoinclude must not carry a dependency block: stacks do not have dependencies, and the
// generated terragrunt.autoinclude.stack.hcl is strictly decoded as unit/stack blocks only, so the
// rejection is raised at parse/generate time with a clear message instead of a later discovery decode error.
func TestParseStackFile_StackAutoIncludeRejectsDependency(t *testing.T) {
	t.Parallel()

	src := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

stack "networking" {
  source = "../stacks/networking"
  path   = "networking"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }
  }
}
`

	_, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.Error(t, err, "a stack autoinclude carrying a dependency block must be rejected")
	assert.Contains(t, err.Error(), "dependency block is not allowed in a stack autoinclude")
}

// Pins the kind-to-filename mapping: unit autoincludes write terragrunt.autoinclude.hcl, stack autoincludes write terragrunt.autoinclude.stack.hcl.
func TestGenerateAutoIncludeFile_FilenameByKind(t *testing.T) {
	t.Parallel()

	src := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

stack "networking" {
  source = "../stacks/networking"
  path   = "networking"

  autoinclude {
    unit "extra" {
      source = "../catalog/units/extra"
      path   = "extra"
    }
  }
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }
  }
}
`
	srcBytes := []byte(src)
	fs := vfs.NewMemMapFS()

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: srcBytes, Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)

	unitResolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.NotNil(t, unitResolved)

	stackResolved := result.AutoIncludes[hclparse.AutoIncludeKey("stack", "networking")]
	require.NotNil(t, stackResolved)

	unitDir := filepath.Join(testStackDir, ".terragrunt-stack", "app")
	require.NoError(t, hclparse.GenerateAutoIncludeFile(fs, unitResolved, unitDir, srcBytes, unitResolved.EvalCtx))

	unitExists, err := vfs.FileExists(fs, filepath.Join(unitDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)
	assert.True(t, unitExists, "unit autoinclude must be written as terragrunt.autoinclude.hcl")

	stackExists, err := vfs.FileExists(fs, filepath.Join(unitDir, hclparse.AutoIncludeStackFile))
	require.NoError(t, err)
	assert.False(t, stackExists, "unit autoinclude must NOT be written as terragrunt.autoinclude.stack.hcl")

	stackDir := filepath.Join(testStackDir, ".terragrunt-stack", "networking")
	require.NoError(t, hclparse.GenerateAutoIncludeFile(fs, stackResolved, stackDir, srcBytes, stackResolved.EvalCtx))

	stackFileExists, err := vfs.FileExists(fs, filepath.Join(stackDir, hclparse.AutoIncludeStackFile))
	require.NoError(t, err)
	assert.True(t, stackFileExists, "stack autoinclude must be written as terragrunt.autoinclude.stack.hcl")

	stackUnitFileExists, err := vfs.FileExists(fs, filepath.Join(stackDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)
	assert.False(t, stackUnitFileExists, "stack autoinclude must NOT be written as terragrunt.autoinclude.hcl")
}

func TestAutoIncludeFileNameForKind(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "terragrunt.autoinclude.hcl", hclparse.AutoIncludeFileNameForKind(hclparse.KindUnit))
	assert.Equal(t, "terragrunt.autoinclude.stack.hcl", hclparse.AutoIncludeFileNameForKind(hclparse.KindStack))
}

func TestAutoIncludeFileNameForKind_PanicsOnUnknownKind(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, `hclparse.AutoIncludeFileNameForKind: unknown kind "" (expected "unit" or "stack")`, func() {
		hclparse.AutoIncludeFileNameForKind("")
	})
	assert.PanicsWithValue(t, `hclparse.AutoIncludeFileNameForKind: unknown kind "unknown" (expected "unit" or "stack")`, func() {
		hclparse.AutoIncludeFileNameForKind("unknown")
	})
}

func TestParseStackFile_DuplicateUnits(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	mainSrc := `
include "shared" {
  path = "shared.hcl"
}

unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}
`
	includeSrc := `
unit "vpc" {
  source = "../catalog/units/vpc2"
  path   = "vpc2"
}
`

	require.NoError(t, fs.MkdirAll(testStackDir, 0755))
	require.NoError(t, vfs.WriteFile(fs, filepath.Join(testStackDir, "shared.hcl"), []byte(includeSrc), 0644))

	_, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: []byte(mainSrc), Filename: filepath.Join(testStackDir, "terragrunt.stack.hcl"), StackDir: testStackDir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate unit name")
}

func TestParseStackFile_DuplicateStacks(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	mainSrc := `
include "shared" {
  path = "shared.hcl"
}

stack "infra" {
  source = "../stacks/infra"
  path   = "infra"
}
`
	includeSrc := `
stack "infra" {
  source = "../stacks/infra2"
  path   = "infra2"
}
`

	require.NoError(t, fs.MkdirAll(testStackDir, 0755))
	require.NoError(t, vfs.WriteFile(fs, filepath.Join(testStackDir, "shared.hcl"), []byte(includeSrc), 0644))

	_, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: []byte(mainSrc), Filename: filepath.Join(testStackDir, "terragrunt.stack.hcl"), StackDir: testStackDir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate stack name")
}

// A top-level unit named "path" must parse cleanly and `unit.path.path` must resolve via autoinclude.
func TestParseStackFile_UnitNamedPathResolvesViaAutoInclude(t *testing.T) {
	t.Parallel()

	src := `
unit "path" {
  source = "../catalog/units/foo"
  path   = "p"
}

unit "consumer" {
  source = "../catalog/units/c"
  path   = "c"

  autoinclude {
    dependency "p" {
      config_path = unit.path.path
    }
  }
}
`

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{
		Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir,
	})
	require.NoError(t, err)
	require.Len(t, result.Units, 2)

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey(hclparse.KindUnit, "consumer")]
	require.NotNil(t, resolved)
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, filepath.Join(testStackDir, hclparse.StackDir, "p"), resolved.Dependencies[0].ConfigPath)
}

// A top-level stack named "path" must parse cleanly.
func TestParseStackFile_StackNamedPathIsAccepted(t *testing.T) {
	t.Parallel()

	src := `
stack "path" {
  source = "../catalog/stacks/path"
  path   = "p"
}
`

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{
		Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir,
	})
	require.NoError(t, err)
	require.Len(t, result.Stacks, 1)
	assert.Equal(t, "path", result.Stacks[0].Name)
}

func TestParseStackFile_IncludeWithLocals(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	mainSrc := `
include "bad" {
  path = "bad.hcl"
}
`
	includeSrc := `
locals {
  env = "prod"
}

unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}
`

	require.NoError(t, fs.MkdirAll(testStackDir, 0755))
	require.NoError(t, vfs.WriteFile(fs, filepath.Join(testStackDir, "bad.hcl"), []byte(includeSrc), 0644))

	_, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: []byte(mainSrc), Filename: filepath.Join(testStackDir, "terragrunt.stack.hcl"), StackDir: testStackDir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not define locals")
}

func TestParseStackFile_IncludeWithNestedIncludes(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	mainSrc := `
include "nested" {
  path = "nested.hcl"
}
`
	includeSrc := `
include "deeper" {
  path = "deeper.hcl"
}

unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}
`

	require.NoError(t, fs.MkdirAll(testStackDir, 0755))
	require.NoError(t, vfs.WriteFile(fs, filepath.Join(testStackDir, "nested.hcl"), []byte(includeSrc), 0644))

	_, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: []byte(mainSrc), Filename: filepath.Join(testStackDir, "terragrunt.stack.hcl"), StackDir: testStackDir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not define nested includes")
}

// Locks the per-file SourceBytes invariant: when a unit's autoinclude block lives in an included stack file, GenerateAutoIncludeFile must use the included file's bytes (not the root's) to slice expression text.
func TestParseStackFile_AutoIncludeInsideIncludedFile(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	rootSrc := `
include "shared" {
  path = "shared.hcl"
}
`
	includedSrc := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"

  autoinclude {
    dependency "db" {
      config_path = "../db"
    }
  }
}
`

	require.NoError(t, fs.MkdirAll(testStackDir, 0755))
	require.NoError(t, vfs.WriteFile(fs, filepath.Join(testStackDir, "shared.hcl"), []byte(includedSrc), 0644))

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: []byte(rootSrc), Filename: filepath.Join(testStackDir, "terragrunt.stack.hcl"), StackDir: testStackDir})
	require.NoError(t, err)
	require.Len(t, result.AutoIncludes, 1)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "vpc")]
	require.True(t, ok, "expected resolved autoinclude for unit 'vpc'")
	require.NotNil(t, resolved)

	// SourceBytes must point at the included file's bytes, not the root's, so the generator can slice expression byte ranges correctly.
	assert.Equal(t, []byte(includedSrc), resolved.SourceBytes, "SourceBytes must equal the included file's bytes")
	assert.NotEqual(t, []byte(rootSrc), resolved.SourceBytes, "SourceBytes must not be the root file's bytes")
}

// Benchmarks

func BenchmarkParseStackFile_Simple(b *testing.B) {
	src := []byte(`
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

unit "db" {
  source = "../catalog/units/db"
  path   = "db"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"
}
`)
	fs := vfs.NewMemMapFS()

	for b.Loop() {
		_, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: src, Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
		if err != nil {
			require.NoError(b, err)
		}
	}
}

func BenchmarkParseStackFile_WithAutoInclude(b *testing.B) {
	src := []byte(`
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

unit "db" {
  source = "../catalog/units/db"
  path   = "db"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
      mock_outputs = { id = "mock" }
    }

    inputs = {
      vpc_id = dependency.vpc.outputs.id
    }
  }
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }

    dependency "db" {
      config_path = unit.db.path
    }

    inputs = {
      vpc_id = dependency.vpc.outputs.id
      db_url = dependency.db.outputs.endpoint
    }
  }
}
`)
	fs := vfs.NewMemMapFS()

	for b.Loop() {
		_, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: src, Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
		if err != nil {
			require.NoError(b, err)
		}
	}
}

func BenchmarkGenerateAutoIncludeFile(b *testing.B) {
	src := []byte(`
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
      mock_outputs_allowed_terraform_commands = ["plan"]
      mock_outputs = { val = "fake-val" }
    }

    inputs = {
      val = dependency.vpc.outputs.val
    }
  }
}
`)
	fs := vfs.NewMemMapFS()

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: src, Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	if err != nil {
		require.NoError(b, err)
	}

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]

	for b.Loop() {
		err := hclparse.GenerateAutoIncludeFile(fs, resolved, testGenDir, src, resolved.EvalCtx)
		if err != nil {
			require.NoError(b, err)
		}
	}
}

// TestAutoIncludeResolve_RejectsValuesAttribute verifies a `values = {...}` inside autoinclude is rejected for both unit and stack kinds.
func TestAutoIncludeResolve_RejectsValuesAttribute(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "unit-kind autoinclude with values",
			src: `
unit "u" {
  source = "../catalog/units/u"
  path   = "u"

  autoinclude {
    values = { v = "literal" }
  }
}
`,
		},
		{
			name: "stack-kind autoinclude with values",
			src: `
stack "s" {
  source = "../catalog/stacks/s"
  path   = "s"

  autoinclude {
    values = { v = "literal" }
  }
}
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{
				Src: []byte(tc.src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir,
			})

			var diags hcl.Diagnostics

			require.ErrorAs(t, err, &diags)
			require.True(t, diags.HasErrors())
			assert.Equal(t, "values is not allowed inside autoinclude", diags[0].Summary)
			assert.Contains(t, diags[0].Detail, "parent unit/stack block")
		})
	}
}

// TestAutoIncludeResolve_RejectsValuesReference verifies a values.* reference inside autoinclude is rejected for both kinds.
func TestAutoIncludeResolve_RejectsValuesReference(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		attr string
		src  string
	}{
		{
			// attr is the dependency label (blockOwnerLabel names a dependency block by its label).
			name: "unit dependency mock_outputs",
			attr: "dep",
			src: `
unit "u" {
  source = "../catalog/units/u"
  path   = "u"

  autoinclude {
    dependency "dep" {
      config_path = unit.u.path

      mock_outputs = {
        region = values.region
      }
    }
  }
}
`,
		},
		{
			name: "unit inputs",
			attr: "inputs",
			src: `
unit "u" {
  source = "../catalog/units/u"
  path   = "u"

  autoinclude {
    inputs = {
      region = values.region
    }
  }
}
`,
		},
		{
			name: "stack inputs",
			attr: "inputs",
			src: `
stack "s" {
  source = "../catalog/stacks/s"
  path   = "s"

  autoinclude {
    inputs = {
      region = values.region
    }
  }
}
`,
		},
		{
			// A stack autoinclude injects unit/stack blocks; a values.* reference inside an injected block must be rejected too.
			name: "stack injected unit values",
			attr: "extra",
			src: `
stack "s" {
  source = "../catalog/stacks/s"
  path   = "s"

  autoinclude {
    unit "extra" {
      source = "../catalog/units/extra"
      path   = "extra"

      values = {
        v = values.region
      }
    }
  }
}
`,
		},
		{
			// A values.* reference nested in a block (an injected unit's own autoinclude inputs) must be rejected.
			name: "stack injected unit nested autoinclude inputs",
			attr: "extra",
			src: `
stack "s" {
  source = "../catalog/stacks/s"
  path   = "s"

  autoinclude {
    unit "extra" {
      source = "../catalog/units/extra"
      path   = "extra"

      autoinclude {
        inputs = {
          v = values.region
        }
      }
    }
  }
}
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{
				Src: []byte(tc.src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir,
			})

			var valuesErr hclparse.AutoIncludeValuesReferenceError

			require.ErrorAs(t, err, &valuesErr)
			assert.Equal(t, tc.attr, valuesErr.Attr)
			assert.Contains(t, err.Error(), "unit-scoped namespace")
			assert.Contains(t, err.Error(), "stack-level local")
		})
	}
}

// TestAutoIncludeResolve_RejectsLocalsBlock verifies a locals block inside autoinclude is rejected for both kinds.
func TestAutoIncludeResolve_RejectsLocalsBlock(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "unit",
			src: `
unit "u" {
  source = "../catalog/units/u"
  path   = "u"

  autoinclude {
    locals {
      x = 1
    }
  }
}
`,
		},
		{
			name: "stack",
			src: `
stack "s" {
  source = "../catalog/stacks/s"
  path   = "s"

  autoinclude {
    locals {
      x = 1
    }
  }
}
`,
		},
		{
			// A locals block nested inside a dependency block must also be rejected.
			name: "nested inside dependency",
			src: `
unit "u" {
  source = "../catalog/units/u"
  path   = "u"

  autoinclude {
    dependency "dep" {
      config_path = unit.u.path

      locals {
        x = 1
      }
    }
  }
}
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{
				Src: []byte(tc.src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir,
			})

			var localsErr hclparse.AutoIncludeLocalsBlockError

			require.ErrorAs(t, err, &localsErr)
			assert.Contains(t, err.Error(), "declare locals at the stack level")
		})
	}
}

// TestParseStackFile_AutoIncludeReferencesUnitMergedFromInclude verifies a unit declared in an included file is reachable as unit.<name>.path during autoinclude resolution.
func TestParseStackFile_AutoIncludeReferencesUnitMergedFromInclude(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	mainSrc := `
include "shared" {
  path = "shared.hcl"
}
`

	includeSrc := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }
  }
}
`

	require.NoError(t, fs.MkdirAll(testStackDir, 0755))
	require.NoError(t, vfs.WriteFile(fs, filepath.Join(testStackDir, "shared.hcl"), []byte(includeSrc), 0644))

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
		Src:      []byte(mainSrc),
		Filename: filepath.Join(testStackDir, "terragrunt.stack.hcl"),
		StackDir: testStackDir,
	})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok, "autoinclude for unit 'app' (from included file) must be resolved")
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, "vpc", resolved.Dependencies[0].Name)
	assert.Equal(t, filepath.Join(testStackDir, ".terragrunt-stack", "vpc"), resolved.Dependencies[0].ConfigPath,
		"unit.vpc.path must resolve to vpc's generated path after include merge, not be undefined")
}

func TestParseStackFile_IncludePathReferencesRootLocal(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	mainSrc := `
locals {
  shared_file = "shared.hcl"
}

include "shared" {
  path = local.shared_file
}
`

	includeSrc := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}
`

	require.NoError(t, fs.MkdirAll(testStackDir, 0755))
	require.NoError(t, vfs.WriteFile(fs, filepath.Join(testStackDir, "shared.hcl"), []byte(includeSrc), 0644))

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
		Src:      []byte(mainSrc),
		Filename: filepath.Join(testStackDir, "terragrunt.stack.hcl"),
		StackDir: testStackDir,
	})
	require.NoError(t, err)
	require.Len(t, result.Units, 1)
	assert.Equal(t, "vpc", result.Units[0].Name)
}

// TestParseStackFile_BootstrapUnitPathEvalErrorSurfaces verifies the bootstrap path returns an error when a unit's path expression cannot be evaluated.
func TestParseStackFile_BootstrapUnitPathEvalErrorSurfaces(t *testing.T) {
	t.Parallel()

	src := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = get_repo_root()
}
`

	_, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{
		Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir,
	})
	require.Error(t, err, "bootstrap parse must surface unsupported-function eval errors instead of silently skipping the unit")
	assert.Contains(t, err.Error(), "get_repo_root", "error must name the offending function so users can locate it")
}

// TestParseStackFile_BootstrapStackSourceEvalErrorSurfaces pins the same hardening for stack-block source expressions.
func TestParseStackFile_BootstrapStackSourceEvalErrorSurfaces(t *testing.T) {
	t.Parallel()

	src := `
stack "networking" {
  source = get_repo_root()
  path   = "networking"
}
`

	_, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{
		Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir,
	})
	require.Error(t, err, "bootstrap parse must surface unsupported-function eval errors in stack source instead of silently skipping the stack")
	assert.Contains(t, err.Error(), "get_repo_root", "error must name the offending function so users can locate it")
}
func TestParseStackFile_LocalEvaluatedOnce(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	onceFn := function.New(&function.Spec{
		Type: function.StaticReturnType(cty.String),
		Impl: func([]cty.Value, cty.Type) (cty.Value, error) {
			calls.Add(1)
			return cty.StringVal("/includes/extra.stack.hcl"), nil
		},
	})

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/includes/extra.stack.hcl", []byte(`
unit "extra" {
  source = "../catalog/units/extra"
  path   = "extra"
}
`), 0644))

	src := []byte(`
locals {
  include_file = once()
}

include "extra" {
  path = local.include_file
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"
}
`)

	_, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
		Src:      src,
		Filename: "terragrunt.stack.hcl",
		StackDir: "/test",
		Functions: map[string]function.Function{
			"once": onceFn,
		},
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func TestParseStackFile_MissingRequiredSourceOrPathReturnsError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "unit missing source",
			src: `
unit "vpc" {
  path = "vpc"
}
`,
		},
		{
			name: "unit missing path",
			src: `
unit "vpc" {
  source = "../catalog/units/vpc"
}
`,
		},
		{
			name: "stack missing source",
			src: `
stack "networking" {
  path = "networking"
}
`,
		},
		{
			name: "stack missing path",
			src: `
stack "networking" {
  source = "../catalog/stacks/networking"
}
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{
				Src:      []byte(tc.src),
				Filename: "terragrunt.stack.hcl",
				StackDir: testStackDir,
			})

			require.Error(t, err)
		})
	}
}

func TestParseStackFile_LocalsCycleIsDeterministic(t *testing.T) {
	t.Parallel()

	src := []byte(`
locals {
  a = local.b
  b = local.c
  c = local.a
}

unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}
`)

	var first []string

	for i := range 5 {
		_, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: src, Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
		require.Error(t, err)

		var cycleErr hclparse.LocalsCycleError
		require.ErrorAs(t, err, &cycleErr)

		if i == 0 {
			first = cycleErr.Names
			continue
		}

		assert.Equal(t, first, cycleErr.Names, "cycle path must be deterministic across runs (run %d)", i)
	}
}

func TestParseStackFile_IncludePathNullCarriesSourcePosition(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/test/dummy.stack.hcl", []byte(`
unit "extra" {
  source = "../catalog/units/extra"
  path   = "extra"
}
`), 0644))

	src := []byte(`
locals {
  bad = null
}

include "extra" {
  path = local.bad
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"
}
`)

	_, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: src, Filename: "terragrunt.stack.hcl", StackDir: "/test"})
	require.Error(t, err)

	var validationErr hclparse.IncludeValidationError
	require.ErrorAs(t, err, &validationErr)

	var diags hcl.Diagnostics
	require.ErrorAs(t, err, &diags)
	require.NotEmpty(t, diags)
	require.NotNil(t, diags[0].Subject, "include validation diag must carry a source position for editor underlining")
	assert.Equal(t, "terragrunt.stack.hcl", diags[0].Subject.Filename)
}

func TestParseStackFile_IncludeMissingRequiredSourceOrPathReturnsError(t *testing.T) {
	t.Parallel()

	// The include path is resolved relative to the parent stack file's StackDir
	// (here: `/test`), so the included file must live alongside the parent.
	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/test/extra.stack.hcl", []byte(`
unit "extra" {
  source = "../catalog/units/extra"
}
`), 0644))

	src := []byte(`
include "extra" {
  path = "extra.stack.hcl"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"
}
`)

	_, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
		Src:      src,
		Filename: "terragrunt.stack.hcl",
		StackDir: "/test",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `path`)
}
