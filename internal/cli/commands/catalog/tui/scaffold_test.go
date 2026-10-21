package tui_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/component"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModelCtrlDScaffoldsModuleWithRacing runs ctrl+d through a headless
// tea.Program, so the tea.Exec scaffold command runs against a local module.
func TestModelCtrlDScaffoldsModuleWithRacing(t *testing.T) {
	t.Parallel()

	repoDir := helpers.TmpDirWOSymlinks(t)
	moduleDir := filepath.Join(repoDir, "modules", "vpc")

	require.NoError(t, os.MkdirAll(moduleDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(moduleDir, "main.tf"),
		[]byte("variable \"name\" {\n  type = string\n}\n"),
		0o644,
	))

	outputDir := helpers.TmpDirWOSymlinks(t)

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(outputDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = outputDir
	opts.NonInteractive = true

	c := tui.NewComponentForTest(component.KindModule, helpers.FileURL(repoDir), "modules/vpc", "")
	entry := tui.NewComponentEntry(c).WithSource(helpers.FileURL(repoDir))

	componentCh := make(chan *tui.ComponentEntry)
	close(componentCh)

	ctx := config.WithCaches(t.Context())

	m := tui.NewModelStreaming(
		ctx,
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv(),
		opts,
		entry,
		componentCh,
		nil,
	)

	p := tea.NewProgram(
		m,
		tea.WithContext(ctx),
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
		tea.WithoutRenderer(),
		tea.WithoutSignals(),
		tea.WithWindowSize(120, 40),
	)

	go func() {
		p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
		p.Send(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	}()

	finalModel, err := p.Run()
	require.NoError(t, err)

	require.NoError(t, finalModel.(tui.Model).Err())
	assert.FileExists(t, filepath.Join(outputDir, "terragrunt.hcl"))
}
