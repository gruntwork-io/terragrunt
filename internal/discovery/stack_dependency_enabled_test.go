package discovery_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRelationshipPhase_StackDependencySkipsDisabledComponents pins that a dependency on a stack
// directory covers the units stack generation writes and no others. The stack below disables a unit,
// disables a nested stack, and disables one element of an expanded unit, so a dependent that waited
// on all of them would wait on directories that never exist.
func TestRelationshipPhase_StackDependencySkipsDisabledComponents(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	stackDir := filepath.Join(tmpDir, "networking")
	generatedDir := filepath.Join(stackDir, hclparse.StackDir)

	v := memRepoRootVenv(t, tmpDir)

	require.NoError(t, vfs.WriteFile(
		v.FS,
		filepath.Join(stackDir, "terragrunt.stack.hcl"),
		[]byte(`
unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}

unit "private" {
  enabled = false
  source  = "../units/subnet"
  path    = "private"
}

unit "subnet" {
  expansion {
    count = 3
  }

  enabled = count.index != 1
  source  = "../units/subnet"
  path    = "subnet-${count.index}"
}

stack "legacy" {
  enabled = false
  source  = "../stacks/legacy"
  path    = "legacy"
}
`),
		0o644,
	))

	// The nested stack a disabled stack block would have generated, left on disk so the walk has
	// something to find if it recurses into it anyway.
	require.NoError(t, vfs.WriteFile(
		v.FS,
		filepath.Join(generatedDir, "legacy", "terragrunt.stack.hcl"),
		[]byte(`
unit "stale" {
  source = "../units/subnet"
  path   = "stale"
}
`),
		0o644,
	))

	blockDepDir := filepath.Join(tmpDir, "app")
	pathsDepDir := filepath.Join(tmpDir, "reporter")

	writeUnits(t, v.FS, map[string]string{
		filepath.Join(generatedDir, "vpc"):      ``,
		filepath.Join(generatedDir, "subnet-0"): ``,
		filepath.Join(generatedDir, "subnet-2"): ``,
		blockDepDir: `
dependency "networking" {
  config_path = "../networking"
}
`,
		pathsDepDir: `
dependencies {
  paths = ["../networking"]
}
`,
	})

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithRelationships()

	components, err := d.Discover(t.Context(), logger.CreateLogger(), v, opts)
	require.NoError(t, err)

	want := []string{
		filepath.Join(generatedDir, "vpc"),
		filepath.Join(generatedDir, "subnet-0"),
		filepath.Join(generatedDir, "subnet-2"),
	}

	for _, dependent := range []string{blockDepDir, pathsDepDir} {
		found := components.FilterByPath(dependent)
		require.Len(t, found, 1, "dependent %s should be discovered", dependent)
		assert.ElementsMatch(t, want, found[0].Dependencies().Paths())
	}
}
