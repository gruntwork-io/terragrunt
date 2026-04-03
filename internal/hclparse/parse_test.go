package hclparse_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
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

	result, err := hclparse.ParseStackFile([]byte(src), "terragrunt.stack.hcl", "/project")
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

	result, err := hclparse.ParseStackFile([]byte(src), "terragrunt.stack.hcl", "/project")
	require.NoError(t, err)
	require.Len(t, result.Units, 2)

	// vpc has no autoinclude
	assert.NotContains(t, result.AutoIncludes, "vpc")

	// db has autoinclude with resolved dependency
	resolved, ok := result.AutoIncludes["db"]
	require.True(t, ok)
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, "vpc", resolved.Dependencies[0].Name)
	// Path should be absolute: /project/.terragrunt-stack/vpc
	assert.Equal(t, filepath.Join("/project", ".terragrunt-stack", "vpc"), resolved.Dependencies[0].ConfigPath)
}

func TestParseStackFile_AutoIncludeWithMockOutputs(t *testing.T) {
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

	result, err := hclparse.ParseStackFile([]byte(src), "terragrunt.stack.hcl", "/project")
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes["app"]
	require.True(t, ok)
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, "vpc", resolved.Dependencies[0].Name)
	assert.Equal(t, filepath.Join("/project", ".terragrunt-stack", "vpc"), resolved.Dependencies[0].ConfigPath)
	// RawBody preserved for generation
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

	result, err := hclparse.ParseStackFile([]byte(src), "terragrunt.stack.hcl", "/project")
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes["app"]
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

	result, err := hclparse.ParseStackFile([]byte(src), "terragrunt.stack.hcl", "/project")
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes["app"]
	require.True(t, ok)
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, "networking", resolved.Dependencies[0].Name)
	assert.Equal(t, filepath.Join("/project", ".terragrunt-stack", "networking"), resolved.Dependencies[0].ConfigPath)
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

	// Parse with two-pass
	result, err := hclparse.ParseStackFile(srcBytes, "terragrunt.stack.hcl", "/project")
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes["app"]
	require.True(t, ok)

	// Generate to temp dir
	tmpDir := t.TempDir()

	err = hclparse.GenerateAutoIncludeFile(resolved, tmpDir, srcBytes)
	require.NoError(t, err)

	// Read generated file
	generatedPath := filepath.Join(tmpDir, hclparse.AutoIncludeFile)
	generated, err := os.ReadFile(generatedPath)
	require.NoError(t, err)

	content := string(generated)

	// Should contain the header comment
	assert.Contains(t, content, "Generated by Terragrunt")

	// Should contain dependency block with resolved config_path
	assert.Contains(t, content, `dependency "vpc"`)
	assert.Contains(t, content, filepath.Join("/project", ".terragrunt-stack", "vpc"))

	// Should contain mock_outputs preserved from AST
	assert.Contains(t, content, "mock_outputs_allowed_terraform_commands")
	assert.Contains(t, content, `"plan"`)
	assert.Contains(t, content, "mock_outputs")
	assert.Contains(t, content, `"fake-val"`)

	// Should contain inputs with dependency.vpc.outputs.val (NOT evaluated)
	assert.Contains(t, content, "inputs")
	assert.Contains(t, content, "dependency.vpc.outputs.val")
}

func TestGenerateAutoIncludeFile_NilResolved(t *testing.T) {
	t.Parallel()

	err := hclparse.GenerateAutoIncludeFile(nil, t.TempDir(), nil)
	assert.NoError(t, err)
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

	result, err := hclparse.ParseStackFile(srcBytes, "terragrunt.stack.hcl", "/project")
	require.NoError(t, err)

	resolved := result.AutoIncludes["app"]
	require.NotNil(t, resolved)

	tmpDir := t.TempDir()

	err = hclparse.GenerateAutoIncludeFile(resolved, tmpDir, srcBytes)
	require.NoError(t, err)

	generated, err := os.ReadFile(filepath.Join(tmpDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)

	content := string(generated)

	// Both dependencies present
	assert.Contains(t, content, `dependency "vpc"`)
	assert.Contains(t, content, `dependency "rds"`)

	// Inputs preserved with dependency refs
	assert.Contains(t, content, "dependency.vpc.outputs.vpc_id")
	assert.Contains(t, content, "dependency.rds.outputs.endpoint")
}

func TestParseStackFile_NoAutoInclude(t *testing.T) {
	t.Parallel()

	src := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}
`

	result, err := hclparse.ParseStackFile([]byte(src), "terragrunt.stack.hcl", "/project")
	require.NoError(t, err)
	require.Len(t, result.Units, 1)
	assert.Empty(t, result.AutoIncludes)
}

func TestGenerateAutoIncludeFile_PreservesInputsExpression(t *testing.T) {
	t.Parallel()

	// Test that complex expressions in inputs are preserved verbatim
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
      combined = dependency.vpc.outputs.val
    }
  }
}
`
	srcBytes := []byte(src)

	result, err := hclparse.ParseStackFile(srcBytes, "terragrunt.stack.hcl", "/project")
	require.NoError(t, err)

	resolved := result.AutoIncludes["app"]
	require.NotNil(t, resolved)

	tmpDir := t.TempDir()

	err = hclparse.GenerateAutoIncludeFile(resolved, tmpDir, srcBytes)
	require.NoError(t, err)

	generated, err := os.ReadFile(filepath.Join(tmpDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)

	content := string(generated)

	// The dependency.vpc.outputs.val expression must appear verbatim
	assert.Contains(t, content, "dependency.vpc.outputs.val")
}

func TestParseStackFile_StackChildUnitPath(t *testing.T) {
	t.Parallel()

	// Create a nested stack source on disk so DiscoverStackChildUnits can read it
	tmpDir := t.TempDir()

	// Create the nested stack source directory with a terragrunt.stack.hcl
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

	// Create the parent stack file that references the nested stack
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

	result, err := hclparse.ParseStackFile([]byte(parentSrc), parentStackFile, parentStackDir)
	require.NoError(t, err)

	// app's autoinclude should resolve vpc dependency to the nested unit path
	resolved, ok := result.AutoIncludes["app"]
	require.True(t, ok)
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, "vpc", resolved.Dependencies[0].Name)

	// The path should point to networking/.terragrunt-stack/vpc
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

	b.ResetTimer()

	for b.Loop() {
		_, err := hclparse.ParseStackFile(src, "terragrunt.stack.hcl", "/project")
		if err != nil {
			b.Fatal(err)
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

	b.ResetTimer()

	for b.Loop() {
		_, err := hclparse.ParseStackFile(src, "terragrunt.stack.hcl", "/project")
		if err != nil {
			b.Fatal(err)
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

	result, err := hclparse.ParseStackFile(src, "terragrunt.stack.hcl", "/project")
	if err != nil {
		b.Fatal(err)
	}

	resolved := result.AutoIncludes["app"]
	tmpDir := b.TempDir()

	b.ResetTimer()

	for b.Loop() {
		err := hclparse.GenerateAutoIncludeFile(resolved, tmpDir, src)
		if err != nil {
			b.Fatal(err)
		}
	}
}
