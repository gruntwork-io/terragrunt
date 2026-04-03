package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandStackDependency_NotAStack(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	result := discovery.ExpandStackDependency(tmpDir)
	assert.Nil(t, result)
}

func TestExpandStackDependency_NonexistentDir(t *testing.T) {
	t.Parallel()

	result := discovery.ExpandStackDependency("/nonexistent/path")
	assert.Nil(t, result)
}

func TestExpandStackDependency_StackWithUnits(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	stackHCL := `
unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}

unit "db" {
  source = "../units/db"
  path   = "db"
}

unit "app" {
  source = "../units/app"
  path   = "app"
}
`
	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, config.DefaultStackFile),
		[]byte(stackHCL),
		0644,
	))

	result := discovery.ExpandStackDependency(tmpDir)

	require.Len(t, result, 3)

	for _, p := range result {
		assert.Contains(t, p, config.StackDir)
	}

	expected := map[string]bool{
		filepath.Join(tmpDir, config.StackDir, "vpc"): true,
		filepath.Join(tmpDir, config.StackDir, "db"):  true,
		filepath.Join(tmpDir, config.StackDir, "app"): true,
	}

	for _, p := range result {
		assert.True(t, expected[p], "unexpected path: %s", p)
	}
}

func TestExpandStackDependency_EmptyStack(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, config.DefaultStackFile),
		[]byte("# Empty stack\n"),
		0644,
	))

	result := discovery.ExpandStackDependency(tmpDir)
	assert.Empty(t, result)
}

func TestExtractUnitPathsFromStackFile(t *testing.T) {
	t.Parallel()

	data := []byte(`
unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}

unit "db" {
  source = "../units/db"
  path   = "db"
}
`)
	stackDir := "/project/live/infra"

	paths := discovery.ExtractUnitPathsFromStackFile(data, stackDir)

	require.Len(t, paths, 2)
	assert.Equal(t, filepath.Join(stackDir, config.StackDir, "vpc"), paths[0])
	assert.Equal(t, filepath.Join(stackDir, config.StackDir, "db"), paths[1])
}

func TestExtractUnitPathsFromStackFile_InvalidHCL(t *testing.T) {
	t.Parallel()

	paths := discovery.ExtractUnitPathsFromStackFile([]byte(`invalid {{{`), "/project")
	assert.Nil(t, paths)
}
