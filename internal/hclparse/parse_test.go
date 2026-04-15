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

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: "/project"})
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

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: "/project"})
	require.NoError(t, err)
	require.Len(t, result.Units, 2)

	assert.NotContains(t, result.AutoIncludes, "vpc")

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "db")]
	require.True(t, ok)
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, "vpc", resolved.Dependencies[0].Name)
	assert.Equal(t, filepath.Join("/project", ".terragrunt-stack", "vpc"), resolved.Dependencies[0].ConfigPath)
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

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: "/project"})
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

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: "/project"})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok)
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, "networking", resolved.Dependencies[0].Name)
	assert.Equal(t, filepath.Join("/project", ".terragrunt-stack", "networking"), resolved.Dependencies[0].ConfigPath)
}

func TestParseStackFile_NoAutoInclude(t *testing.T) {
	t.Parallel()

	src := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}
`

	result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{Src: []byte(src), Filename: "terragrunt.stack.hcl", StackDir: "/project"})
	require.NoError(t, err)
	require.Len(t, result.Units, 1)
	assert.Empty(t, result.AutoIncludes)
}

func TestGenerateAutoIncludeFile_NilResolved(t *testing.T) {
	t.Parallel()

	err := hclparse.GenerateAutoIncludeFile(vfs.NewMemMapFS(), nil, "/project/.terragrunt-stack/app", nil, nil)
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

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: srcBytes, Filename: "terragrunt.stack.hcl", StackDir: "/project"})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok)

	appDir := "/project/.terragrunt-stack/app"

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

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: srcBytes, Filename: "terragrunt.stack.hcl", StackDir: "/project"})
	require.NoError(t, err)

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.NotNil(t, resolved)

	appDir := "/project/.terragrunt-stack/app"

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

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: srcBytes, Filename: "terragrunt.stack.hcl", StackDir: "/project"})
	require.NoError(t, err)

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.NotNil(t, resolved)

	appDir := filepath.Join("/project", ".terragrunt-stack", "app")

	err = hclparse.GenerateAutoIncludeFile(fs, resolved, appDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	generated, err := vfs.ReadFile(fs, filepath.Join(appDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)

	content := string(generated)
	assert.Contains(t, content, `dependency "vpc"`)
	assert.Contains(t, content, "../vpc")
	assert.NotContains(t, content, "/project")
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

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: srcBytes, Filename: "terragrunt.stack.hcl", StackDir: "/project"})
	require.NoError(t, err)

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.NotNil(t, resolved)

	appDir := "/project/.terragrunt-stack/app"

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

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: srcBytes, Filename: "terragrunt.stack.hcl", StackDir: "/project"})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok)

	appDir := "/project/.terragrunt-stack/app"

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
		_, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: src, Filename: "terragrunt.stack.hcl", StackDir: "/project"})
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
		_, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: src, Filename: "terragrunt.stack.hcl", StackDir: "/project"})
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

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: src, Filename: "terragrunt.stack.hcl", StackDir: "/project"})
	if err != nil {
		require.NoError(b, err)
	}

	resolved := result.AutoIncludes[hclparse.AutoIncludeKey("unit", "app")]

	for b.Loop() {
		err := hclparse.GenerateAutoIncludeFile(fs, resolved, "/project/.terragrunt-stack/app", src, resolved.EvalCtx)
		if err != nil {
			require.NoError(b, err)
		}
	}
}
