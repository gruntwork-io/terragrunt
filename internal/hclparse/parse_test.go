package hclparse_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
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
	assert.Equal(t, []string{"a", "b"}, cycleErr.Names)
	assert.Equal(t, map[string][]string{
		"a": {"b"},
		"b": {"a"},
	}, cycleErr.Edges)
}

// TestParseStackFile_LocalsThreeNodeCycle verifies that cycles longer than two
// participants are surfaced with every member and edge intact.
func TestParseStackFile_LocalsThreeNodeCycle(t *testing.T) {
	t.Parallel()

	src := `
locals {
  a = local.b
  b = local.c
  c = local.a
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
	require.Error(t, err)

	var cycleErr hclparse.LocalsCycleError
	require.ErrorAs(t, err, &cycleErr)
	assert.Equal(t, []string{"a", "b", "c"}, cycleErr.Names)
	assert.Equal(t, map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"},
	}, cycleErr.Edges)
}

// TestParseStackFile_LocalsSelfReference pins the self-reference behavior: an
// attribute that depends on itself is a cycle of length 1.
func TestParseStackFile_LocalsSelfReference(t *testing.T) {
	t.Parallel()

	src := `
locals {
  a = local.a
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
	require.Error(t, err)

	var cycleErr hclparse.LocalsCycleError
	require.ErrorAs(t, err, &cycleErr)
	assert.Equal(t, []string{"a"}, cycleErr.Names)
	assert.Equal(t, map[string][]string{"a": {"a"}}, cycleErr.Edges)
}

// TestParseStackFile_LocalsCycleWithUnrelatedSiblings verifies that locals
// outside the cycle still resolve and only the cycle members are reported.
func TestParseStackFile_LocalsCycleWithUnrelatedSiblings(t *testing.T) {
	t.Parallel()

	src := `
locals {
  ok_one  = "value-1"
  ok_two  = local.ok_one
  bad_one = local.bad_two
  bad_two = local.bad_one
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
	require.Error(t, err)

	var cycleErr hclparse.LocalsCycleError
	require.ErrorAs(t, err, &cycleErr)
	assert.Equal(t, []string{"bad_one", "bad_two"}, cycleErr.Names)
	assert.Equal(t, map[string][]string{
		"bad_one": {"bad_two"},
		"bad_two": {"bad_one"},
	}, cycleErr.Edges)
}

// TestParseStackFile_LocalsUnknownReference checks that referencing an
// undefined sibling surfaces as a precise HCL evaluation error (via
// LocalEvalError), not as a cycle.
func TestParseStackFile_LocalsUnknownReference(t *testing.T) {
	t.Parallel()

	src := `
locals {
  a = local.does_not_exist
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
	require.Error(t, err)

	var evalErr hclparse.LocalEvalError
	require.ErrorAs(t, err, &evalErr)
	assert.Equal(t, "a", evalErr.Name)

	var cycleErr hclparse.LocalsCycleError
	assert.NotErrorAs(t, err, &cycleErr, "expected eval error, not cycle error")
}

// TestParseStackFile_LocalsLinearChain exercises deep dependency chains to
// confirm the topological evaluator handles long paths without the iteration
// cap the previous implementation relied on. A chain of 200 locals would have
// stayed well under the cap, but encoding it as a single test makes the
// expected ordering visible.
func TestParseStackFile_LocalsLinearChain(t *testing.T) {
	t.Parallel()

	const depth = 200

	var b strings.Builder

	b.WriteString("locals {\n")
	b.WriteString("  l0 = 1\n")

	for i := 1; i < depth; i++ {
		fmt.Fprintf(&b, "  l%d = local.l%d + 1\n", i, i-1)
	}

	b.WriteString("}\n\n")
	b.WriteString("unit \"sink\" {\n")
	b.WriteString("  source = \"../catalog/units/sink\"\n")
	b.WriteString("  path   = \"sink\"\n\n")
	b.WriteString("  autoinclude {\n")
	b.WriteString("    dependency \"foo\" {\n")
	b.WriteString("      config_path = \"../foo\"\n")
	b.WriteString("    }\n\n")
	fmt.Fprintf(&b, "    last_value = local.l%d\n", depth-1)
	b.WriteString("  }\n}\n")

	srcBytes := []byte(b.String())
	fs := vfs.NewMemMapFS()

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
		Src:      srcBytes,
		Filename: "terragrunt.stack.hcl",
		StackDir: testStackDir,
	})
	require.NoError(t, err)

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "sink")]
	require.NotNil(t, resolved)

	sinkDir := filepath.Join(testStackDir, ".terragrunt-stack", "sink")
	err = hclparse.GenerateAutoIncludeFile(fs, resolved, sinkDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	generated, err := vfs.ReadFile(fs, filepath.Join(sinkDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)

	assert.Contains(t, string(generated), fmt.Sprintf("last_value = %d", depth))
}

// TestParseStackFile_LocalsDiamond verifies that diamond-shaped graphs (A -> B,
// A -> C, B -> D, C -> D) evaluate cleanly without revisiting nodes.
func TestParseStackFile_LocalsDiamond(t *testing.T) {
	t.Parallel()

	src := `
locals {
  base   = "base"
  left   = "${local.base}-left"
  right  = "${local.base}-right"
  merged = "${local.left}+${local.right}"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = "../vpc"
    }

    merged_tag = local.merged
  }
}
`
	srcBytes := []byte(src)
	fs := vfs.NewMemMapFS()

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
		Src:      srcBytes,
		Filename: "terragrunt.stack.hcl",
		StackDir: testStackDir,
	})
	require.NoError(t, err)

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.NotNil(t, resolved)

	err = hclparse.GenerateAutoIncludeFile(fs, resolved, testGenDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	generated, err := vfs.ReadFile(fs, filepath.Join(testGenDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)
	assert.Contains(t, string(generated), `"base-left+base-right"`)
}

// TestParseStackFile_LocalsDuplicateReference verifies that referencing the
// same sibling more than once in a single expression doesn't inflate the
// dependency count or affect evaluation.
func TestParseStackFile_LocalsDuplicateReference(t *testing.T) {
	t.Parallel()

	src := `
locals {
  base    = "base"
  doubled = "${local.base}-${local.base}-${local.base}"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = "../vpc"
    }

    tag = local.doubled
  }
}
`
	srcBytes := []byte(src)
	fs := vfs.NewMemMapFS()

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
		Src:      srcBytes,
		Filename: "terragrunt.stack.hcl",
		StackDir: testStackDir,
	})
	require.NoError(t, err)

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.NotNil(t, resolved)

	err = hclparse.GenerateAutoIncludeFile(fs, resolved, testGenDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	generated, err := vfs.ReadFile(fs, filepath.Join(testGenDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)
	assert.Contains(t, string(generated), `"base-base-base"`)
}

// TestParseStackFile_LocalsReferenceValues exercises the caller-injected
// `values` namespace inside a locals block. It is the closest analogue stack
// files have today to how `feature.<name>.value` references resolve in the
// broader Terragrunt locals pipeline: the namespace is supplied externally,
// the DAG builder ignores it (no graph edge), and HCL reads it from the
// evaluation context at eval time. The same shape will keep working when
// feature blocks are extended into terragrunt.stack.hcl — the only change
// needed is the caller populating evalCtx.Variables["feature"].
func TestParseStackFile_LocalsReferenceValues(t *testing.T) {
	t.Parallel()

	src := `
locals {
  env    = values.env
  region = values.region
  tag    = "${local.env}-${local.region}"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = "../vpc"
    }

    name_tag = local.tag
  }
}
`
	srcBytes := []byte(src)
	fs := vfs.NewMemMapFS()

	values := cty.ObjectVal(map[string]cty.Value{
		"env":    cty.StringVal("prod"),
		"region": cty.StringVal("us-west-2"),
	})

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
		Src:      srcBytes,
		Filename: "terragrunt.stack.hcl",
		StackDir: testStackDir,
		Values:   &values,
	})
	require.NoError(t, err)

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.NotNil(t, resolved)

	err = hclparse.GenerateAutoIncludeFile(fs, resolved, testGenDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	generated, err := vfs.ReadFile(fs, filepath.Join(testGenDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)
	assert.Contains(t, string(generated), `"prod-us-west-2"`)
}

// TestParseStackFile_LocalsMissingExternalNamespace verifies that referencing
// a namespace the caller never populated (the same failure mode users would
// see for an unwired feature block) surfaces as a LocalEvalError naming the
// offending local, not as a cycle.
func TestParseStackFile_LocalsMissingExternalNamespace(t *testing.T) {
	t.Parallel()

	src := `
locals {
  oops = values.absent
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
	require.Error(t, err)

	var evalErr hclparse.LocalEvalError
	require.ErrorAs(t, err, &evalErr)
	assert.Equal(t, "oops", evalErr.Name)

	var cycleErr hclparse.LocalsCycleError
	assert.NotErrorAs(t, err, &cycleErr, "missing external ref must not be reported as a cycle")
}

// TestParseStackFile_LocalsReferenceExternalNamespace confirms that locals
// referencing a different namespace (here, unit.<name>.path) evaluate against
// the eval context that the caller pre-populated, without being treated as
// edges in the locals DAG. This is the same path that feature.* references
// will take once feature blocks are wired into stack files.
func TestParseStackFile_LocalsReferenceExternalNamespace(t *testing.T) {
	t.Parallel()

	src := `
locals {
  vpc_path = unit.vpc.path
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
      config_path = local.vpc_path
    }
  }
}
`
	srcBytes := []byte(src)
	fs := vfs.NewMemMapFS()

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
		Src:      srcBytes,
		Filename: "terragrunt.stack.hcl",
		StackDir: testStackDir,
	})
	require.NoError(t, err)

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.NotNil(t, resolved)
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, filepath.Join(testStackDir, ".terragrunt-stack", "vpc"), resolved.Dependencies[0].ConfigPath)
}

// TestParseStackFile_LocalsBareLocalRoot pins the broad-dependency rule for
// bare `local` references: because the graph builder can't tell statically
// which sibling is being read, it forces the attribute to evaluate after every
// other sibling so the value observed is the fully-populated local map. This
// keeps the result independent of Go's randomized map iteration order.
//
// The test reads each sibling through the bare-`local` snapshot inside a
// string interpolation, which the autoinclude generator fully evaluates and
// emits as a literal. Seeing both values in the output proves both siblings
// were resolved before the snapshot was taken.
func TestParseStackFile_LocalsBareLocalRoot(t *testing.T) {
	t.Parallel()

	src := `
locals {
  a     = "alpha"
  z     = "zulu"
  whole = local
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = "../vpc"
    }

    snapshot = "a=${local.whole.a};z=${local.whole.z}"
  }
}
`
	srcBytes := []byte(src)
	fs := vfs.NewMemMapFS()

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
		Src:      srcBytes,
		Filename: "terragrunt.stack.hcl",
		StackDir: testStackDir,
	})
	require.NoError(t, err)

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.NotNil(t, resolved)

	err = hclparse.GenerateAutoIncludeFile(fs, resolved, testGenDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	generated, err := vfs.ReadFile(fs, filepath.Join(testGenDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)
	assert.Contains(t, string(generated), `"a=alpha;z=zulu"`)
}

// TestParseStackFile_LocalsBareLocalCycle confirms the broad-dependency rule
// surfaces as a cycle when two or more attributes both reference bare `local`
// (each demands the other be evaluated first).
func TestParseStackFile_LocalsBareLocalCycle(t *testing.T) {
	t.Parallel()

	src := `
locals {
  a = local
  b = local
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
	require.Error(t, err)

	var cycleErr hclparse.LocalsCycleError
	require.ErrorAs(t, err, &cycleErr)
	assert.Equal(t, []string{"a", "b"}, cycleErr.Names)
}

// TestParseStackFile_EmptyLocalsBlock makes sure an empty locals block — which
// has no attributes to evaluate — is a no-op and not an error.
func TestParseStackFile_EmptyLocalsBlock(t *testing.T) {
	t.Parallel()

	src := `
locals {
}

unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}
`

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{
		Src:      []byte(src),
		Filename: "terragrunt.stack.hcl",
		StackDir: testStackDir,
	})
	require.NoError(t, err)
	require.Len(t, result.Units, 1)
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
