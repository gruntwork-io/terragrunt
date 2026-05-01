package hclparse_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestParseStackFile_StackChildUnitPath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	nestedStackDir := filepath.Join(tmpDir, "catalog", "stacks", "networking")
	require.NoError(t, os.MkdirAll(nestedStackDir, 0755))

	nestedStackHCL := `
unit "vpc" {
  source = "../../units/vpc"
  path   = "vpc"
}

unit "subnets" {
  source = "../../units/subnets"
  path   = "subnets"
}
`
	require.NoError(t, os.WriteFile(
		filepath.Join(nestedStackDir, "terragrunt.stack.hcl"),
		[]byte(nestedStackHCL),
		0644,
	))

	parentStackDir := filepath.Join(tmpDir, "live")
	require.NoError(t, os.MkdirAll(parentStackDir, 0755))

	parentSrc := `
stack "networking" {
  source = "../catalog/stacks/networking"
  path   = "networking"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = stack.networking.vpc.path
    }
  }
}
`
	parentStackFile := filepath.Join(parentStackDir, "terragrunt.stack.hcl")
	require.NoError(t, os.WriteFile(parentStackFile, []byte(parentSrc), 0644))

	result, err := hclparse.ParseStackFile(vfs.NewOSFS(), &hclparse.ParseStackFileInput{Src: []byte(parentSrc), Filename: parentStackFile, StackDir: parentStackDir})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok)
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, "vpc", resolved.Dependencies[0].Name)

	expectedPath := filepath.Join(parentStackDir, ".terragrunt-stack", "networking", ".terragrunt-stack", "vpc")
	assert.Equal(t, expectedPath, resolved.Dependencies[0].ConfigPath)
}

// TestParseStackFile_NestedStackPath verifies that stack.<name>.<nested_stack>.path
// resolves correctly: a parent stack contains a child stack which itself contains
// a nested stack. The reference stack.parent.child.path resolves to the nested
// stack's generated directory.
func TestParseStackFile_NestedStackPath(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	require.NoError(t, fs.MkdirAll("/project/catalog/stacks/deep", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/project/catalog/stacks/deep/terragrunt.stack.hcl", []byte(`
unit "db" {
  source = "../../units/db"
  path   = "db"
}
`), 0644))

	require.NoError(t, fs.MkdirAll("/project/catalog/stacks/infra", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/project/catalog/stacks/infra/terragrunt.stack.hcl", []byte(`
unit "vpc" {
  source = "../../units/vpc"
  path   = "vpc"
}

stack "deep" {
  source = "../deep"
  path   = "deep"
}
`), 0644))

	parentStackDir := "/project/live"

	require.NoError(t, fs.MkdirAll(parentStackDir, 0755))

	parentSrc := `
stack "infra" {
  source = "../catalog/stacks/infra"
  path   = "infra"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "deep" {
      config_path = stack.infra.deep.path
    }
  }
}
`
	parentStackFile := filepath.Join(parentStackDir, "terragrunt.stack.hcl")
	require.NoError(t, vfs.WriteFile(fs, parentStackFile, []byte(parentSrc), 0644))

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
		Src:      []byte(parentSrc),
		Filename: parentStackFile,
		StackDir: parentStackDir,
	})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok)
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, "deep", resolved.Dependencies[0].Name)

	// stack.infra.deep.path should resolve to:
	// .terragrunt-stack/infra/.terragrunt-stack/deep
	expectedPath := filepath.Join(parentStackDir, ".terragrunt-stack", "infra", ".terragrunt-stack", "deep")
	assert.Equal(t, expectedPath, resolved.Dependencies[0].ConfigPath)
}

// TestParseStackFile_TopLevelStackNoDotTerragruntStack verifies that a top-level
// stack block with no_dot_terragrunt_stack = true resolves stack.<name>.path to
// <stackDir>/<s.Path> (not <stackDir>/.terragrunt-stack/<s.Path>), matching the
// behavior of resolveDestPath in pkg/config/stack.go. Also asserts the
// second-order behavior: when the recursion into the stack's children runs from
// the repositioned path, a child unit that also sets no_dot_terragrunt_stack
// materializes under the stack's own directory with no .terragrunt-stack/
// wrapping at any level.
func TestParseStackFile_TopLevelStackNoDotTerragruntStack(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	require.NoError(t, fs.MkdirAll("/project/catalog/stacks/networking", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/project/catalog/stacks/networking/terragrunt.stack.hcl", []byte(`
unit "vpc" {
  source                  = "../../units/vpc"
  path                    = "vpc"
  no_dot_terragrunt_stack = true
}
`), 0644))

	parentStackDir := "/project/live"

	require.NoError(t, fs.MkdirAll(parentStackDir, 0755))

	parentSrc := `
stack "networking" {
  source                  = "../catalog/stacks/networking"
  path                    = "networking"
  no_dot_terragrunt_stack = true
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "net" {
      config_path = stack.networking.path
    }

    dependency "vpc" {
      config_path = stack.networking.vpc.path
    }
  }
}
`
	parentStackFile := filepath.Join(parentStackDir, "terragrunt.stack.hcl")
	require.NoError(t, vfs.WriteFile(fs, parentStackFile, []byte(parentSrc), 0644))

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
		Src:      []byte(parentSrc),
		Filename: parentStackFile,
		StackDir: parentStackDir,
	})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok)
	require.Len(t, resolved.Dependencies, 2)

	depPathsByName := make(map[string]string, len(resolved.Dependencies))
	for _, dep := range resolved.Dependencies {
		depPathsByName[dep.Name] = dep.ConfigPath
	}

	// stack.networking.path resolves under parentStackDir, bypassing .terragrunt-stack/.
	assert.Equal(t, filepath.Join(parentStackDir, "networking"), depPathsByName["net"])

	// stack.networking.vpc.path: the child unit also sets no_dot_terragrunt_stack,
	// so recursion into the repositioned stack dir places vpc directly under it.
	assert.Equal(t, filepath.Join(parentStackDir, "networking", "vpc"), depPathsByName["vpc"])
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

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: testStackDir})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("stack", "networking")]
	require.True(t, ok, "stack networking should have autoinclude")
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, "vpc", resolved.Dependencies[0].Name)
	assert.Equal(t, hclparse.KindStack, resolved.Kind, "Kind must be KindStack so the generator picks terragrunt.autoinclude.stack.hcl")
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
    dependency "vpc" {
      config_path = unit.vpc.path
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
