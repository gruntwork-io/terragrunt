package tui_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnitPreviewShowsMetadataAndRelationships(t *testing.T) {
	t.Parallel()

	source := "example.com/mod/aws"
	unit := component.NewUnit("/repo/vpc").
		WithConfig(&config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: &source}}).
		WithReading("/repo/vpc/terragrunt.hcl", "/repo/common.hcl")
	unit.AddDependency(component.NewUnit("/repo/db"))
	unit.AddDependent(component.NewUnit("/repo/app"))

	m := newModel(t, vfs.NewMemMapFS(), tui.BuildTree("/repo", component.Components{unit}), tui.ColorDisabled)
	require.Equal(t, "vpc", m.Selected().Name())

	content := m.View().Content
	assert.Contains(t, content, "Kind:")
	assert.Contains(t, content, "unit")
	assert.Contains(t, content, "Source:")
	assert.Contains(t, content, source)
	assert.Contains(t, content, "Dependencies:")
	assert.Contains(t, content, "../db")
	assert.Contains(t, content, "Dependents:")
	assert.Contains(t, content, "../app")
	assert.Contains(t, content, "Reading:")
	// Read files are shown relative to the unit's own directory.
	assert.Contains(t, content, "terragrunt.hcl")
	assert.Contains(t, content, "../common.hcl")
}

func TestStackPreviewListsDefinedUnitsAndStacks(t *testing.T) {
	t.Parallel()

	stack := component.NewStack("/repo/live")
	stack.StoreConfig(&config.StackConfig{
		Units: []*config.Unit{
			{Name: "db", Source: "./mods/db", Path: "db"},
			{Name: "api", Source: "./mods/api", Path: "api"},
		},
		Stacks: []*config.Stack{
			{Name: "network", Source: "./stacks/net", Path: "net"},
		},
	})

	m := newModel(t, vfs.NewMemMapFS(), tui.BuildTree("/repo", component.Components{stack}), tui.ColorDisabled)
	require.Equal(t, "live", m.Selected().Name())
	require.Equal(t, tui.KindStack, m.Selected().Kind())

	content := m.View().Content
	assert.Contains(t, content, "stack")
	assert.Contains(t, content, "Units:")
	assert.Contains(t, content, "Stacks:")
	assert.Contains(t, content, "network")
	assert.Contains(t, content, "source:")
	assert.Contains(t, content, "./mods/api")
	assert.Contains(t, content, "path:")

	assert.Less(t, strings.Index(content, "api"), strings.Index(content, "db"),
		"stack entries should be listed sorted by name")
}

func TestComponentPreviewLoadsThenReportsNotDiscovered(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/terragrunt.hcl", nil, 0o644))

	m := newModel(t, fs, tui.NewRoot("/repo"), tui.ColorDisabled)
	require.Equal(t, tui.KindUnit, m.Selected().Kind())
	assert.NotContains(t, m.View().Content, "(not discovered)")

	// Discovery finishes without resolving vpc, so its bare classification is
	// flagged rather than presented as complete metadata.
	m = update(t, m, tui.DiscoveryResult{})
	assert.Contains(t, m.View().Content, "(not discovered)")
}

func TestFilePreviewRendersInDetailPane(t *testing.T) {
	t.Parallel()

	const content = "name = \"value\"\n"

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/terragrunt.hcl", []byte(content), 0o644))

	root := tui.BuildTree("/repo", component.Components{component.NewUnit("/repo/vpc")})

	m := newModel(t, fs, root, tui.ColorDisabled)
	m = press(t, m, 'l')

	require.Equal(t, "terragrunt.hcl", m.Selected().Name())
	assert.Contains(t, m.View().Content, "name = ")
}
