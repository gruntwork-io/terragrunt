package tui_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test configuration - color profiles are handled by individual test cases if needed

// createMockCatalogService creates a mock catalog service with test modules for testing
func createMockCatalogService(t *testing.T, opts *options.TerragruntOptions) catalog.CatalogService {
	t.Helper()

	mockNewRepo := func(ctx context.Context, logger log.Logger, repoURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error) {
		// Create a temporary directory structure for testing
		dummyRepoDir := filepath.Join(t.TempDir(), strings.ReplaceAll(repoURL, "github.com/gruntwork-io/", ""))
		os.MkdirAll(filepath.Join(dummyRepoDir, ".git"), 0755)
		os.WriteFile(filepath.Join(dummyRepoDir, ".git", "config"), []byte("[remote \"origin\"]\nurl = "+repoURL), 0644)
		os.WriteFile(filepath.Join(dummyRepoDir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)

		// Create test modules based on repoURL
		switch repoURL {
		case "github.com/gruntwork-io/test-repo-1":
			readme1Path := filepath.Join(dummyRepoDir, "README.md")
			os.WriteFile(readme1Path, []byte("# AWS VPC Module\nThis module creates a VPC in AWS with all the necessary components."), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, "main.tf"), []byte("# VPC terraform configuration"), 0644)
		case "github.com/gruntwork-io/test-repo-2":
			readme2Path := filepath.Join(dummyRepoDir, "README.md")
			os.WriteFile(readme2Path, []byte("# AWS EKS Module\nThis module creates an EKS cluster in AWS."), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, "main.tf"), []byte("# EKS terraform configuration"), 0644)
		default:
			return nil, fmt.Errorf("unexpected repoURL in mock: %s", repoURL)
		}

		return module.NewRepo(ctx, logger, dummyRepoDir, path, walkWithSymlinks, allowCAS)
	}

	// Create a temporary root config file
	tmpDir := t.TempDir()
	rootFile := filepath.Join(tmpDir, "root.hcl")
	err := os.WriteFile(rootFile, []byte(`catalog {
	urls = [
		"github.com/gruntwork-io/test-repo-1",
		"github.com/gruntwork-io/test-repo-2",
	]
}`), 0600)
	require.NoError(t, err)

	unitDir := filepath.Join(tmpDir, "unit")
	os.MkdirAll(unitDir, 0755)
	opts.TerragruntConfigPath = filepath.Join(unitDir, "terragrunt.hcl")
	opts.ScaffoldRootFileName = config.RecommendedParentConfigName

	svc := catalog.NewCatalogService(opts).WithNewRepoFunc(mockNewRepo)

	// Load the modules
	ctx := t.Context()
	l := logger.CreateLogger()
	err = svc.Load(ctx, l)
	require.NoError(t, err)

	return svc
}

func TestTUIFinalModel(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	svc := createMockCatalogService(t, opts)
	l := logger.CreateLogger()

	m := tui.NewModel(l, opts, svc)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	// Send 'q' to quit the application immediately
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("q"),
	})

	fm := tm.FinalModel(t)
	finalModel, ok := fm.(tui.Model)
	require.True(t, ok, "final model should be of type model")

	// Verify the model has the expected state
	assert.Equal(t, tui.ListState, finalModel.State)
	assert.NotNil(t, finalModel.SVC)
	assert.NotNil(t, finalModel.List)
	assert.Len(t, finalModel.SVC.Modules(), 2, "should have 2 test modules")
}

func TestTUIInitialOutput(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	svc := createMockCatalogService(t, opts)
	l := logger.CreateLogger()

	m := tui.NewModel(l, opts, svc)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	// Send 'q' to quit immediately for consistent output
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("q"),
	})

	// Test that we get the expected output
	out, err := io.ReadAll(tm.FinalOutput(t))
	require.NoError(t, err)

	teatest.RequireEqualOutput(t, out)
}

func TestTUINavigationToModuleDetails(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	svc := createMockCatalogService(t, opts)
	l := logger.CreateLogger()

	m := tui.NewModel(l, opts, svc)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	// Wait for initial render
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte("List of Modules"))
	}, teatest.WithCheckInterval(time.Millisecond*100), teatest.WithDuration(time.Second*2))

	// Press Enter to select the first module (assuming it's pre-selected)
	tm.Send(tea.KeyMsg{
		Type: tea.KeyEnter,
	})

	// Wait for the pager view to appear
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		output := string(bts)
		// Check for pager view elements (scroll percentage, button bar)
		return strings.Contains(output, "%") && (strings.Contains(output, "Scaffold") || strings.Contains(output, "View Source"))
	}, teatest.WithCheckInterval(time.Millisecond*100), teatest.WithDuration(time.Second*3))

	// Send 'q' to go back to list
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("q"),
	})

	// Wait for return to list view
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte("List of Modules"))
	}, teatest.WithCheckInterval(time.Millisecond*100), teatest.WithDuration(time.Second*2))

	// Finally quit the application
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("q"),
	})

	tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second*2))
}

func TestTUIModuleFiltering(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	svc := createMockCatalogService(t, opts)
	l := logger.CreateLogger()

	m := tui.NewModel(l, opts, svc)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	// Wait for initial render
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte("List of Modules"))
	}, teatest.WithCheckInterval(time.Millisecond*100), teatest.WithDuration(time.Second*2))

	// Activate filtering with '/'
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("/"),
	})

	// Type filter text
	tm.Type("VPC")

	// Wait for filtering to take effect
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		output := string(bts)
		// Should show filtered results containing "VPC"
		return strings.Contains(strings.ToUpper(output), "VPC")
	}, teatest.WithCheckInterval(time.Millisecond*100), teatest.WithDuration(time.Second*3))

	// Press Escape to exit filtering
	tm.Send(tea.KeyMsg{
		Type: tea.KeyEsc,
	})

	// Wait for return to normal list view
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		output := string(bts)
		// Should show both modules again
		return strings.Contains(output, "VPC") && strings.Contains(output, "EKS")
	}, teatest.WithCheckInterval(time.Millisecond*100), teatest.WithDuration(time.Second*2))

	// Quit the application
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("q"),
	})

	tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second*2))
}

func TestTUIWindowResize(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	svc := createMockCatalogService(t, opts)
	l := logger.CreateLogger()

	m := tui.NewModel(l, opts, svc)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 30))

	// Wait for initial render
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte("List of Modules"))
	}, teatest.WithCheckInterval(time.Millisecond*100), teatest.WithDuration(time.Second*2))

	// Send window resize message
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Verify the interface handles resize gracefully
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte("List of Modules"))
	}, teatest.WithCheckInterval(time.Millisecond*100), teatest.WithDuration(time.Second*2))

	// Quit
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("q"),
	})

	tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second*2))
}
