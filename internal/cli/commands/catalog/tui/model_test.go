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

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test configuration - color profiles are handled by individual test cases if needed

// createMockCatalogService creates a mock catalog service with test modules for testing
func createMockCatalogService(t *testing.T, opts *options.TerragruntOptions) catalog.CatalogService {
	t.Helper()

	mockNewRepo := func(
		ctx context.Context,
		logger log.Logger,
		repoURL, path string,
		walkWithSymlinks, allowCAS bool,
	) (*module.Repo, error) {
		// Create a temporary directory structure for testing
		repoName := strings.ReplaceAll(repoURL, "github.com/gruntwork-io/", "")
		dummyRepoDir := filepath.Join(helpers.TmpDirWOSymlinks(t), repoName)

		// Initialize as a proper git repository
		os.MkdirAll(dummyRepoDir, 0755)

		// Initialize git repository
		gitDir := filepath.Join(dummyRepoDir, ".git")
		os.MkdirAll(gitDir, 0755)
		os.WriteFile(filepath.Join(gitDir, "config"), fmt.Appendf(nil, `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
[remote "origin"]
	url = %s
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
	merge = refs/heads/main
`, repoURL), 0644)
		os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644)

		// Create refs directory structure
		refsDir := filepath.Join(gitDir, "refs")
		headsDir := filepath.Join(refsDir, "heads")
		remotesDir := filepath.Join(refsDir, "remotes", "origin")

		os.MkdirAll(headsDir, 0755)
		os.MkdirAll(remotesDir, 0755)

		// Create a fake commit hash for main branch
		fakeCommitHash := "1234567890abcdef1234567890abcdef12345678"
		os.WriteFile(filepath.Join(headsDir, "main"), []byte(fakeCommitHash+"\n"), 0644)
		os.WriteFile(filepath.Join(remotesDir, "main"), []byte(fakeCommitHash+"\n"), 0644)

		// Create test modules based on repoURL
		switch repoURL {
		case "github.com/gruntwork-io/test-repo-1":
			readme1Path := filepath.Join(dummyRepoDir, "README.md")
			vpcReadme := "# AWS VPC Module\nThis module creates a VPC in AWS with all the necessary components."
			os.WriteFile(readme1Path, []byte(vpcReadme), 0644)

			mainTfPath := filepath.Join(dummyRepoDir, "main.tf")
			os.WriteFile(mainTfPath, []byte("# VPC terraform configuration"), 0644)
		case "github.com/gruntwork-io/test-repo-2":
			readme2Path := filepath.Join(dummyRepoDir, "README.md")
			eksReadme := "# AWS EKS Module\nThis module creates an EKS cluster in AWS."
			os.WriteFile(readme2Path, []byte(eksReadme), 0644)

			mainTfPath := filepath.Join(dummyRepoDir, "main.tf")
			os.WriteFile(mainTfPath, []byte("# EKS terraform configuration"), 0644)
		default:
			return nil, fmt.Errorf("unexpected repoURL in mock: %s", repoURL)
		}

		return module.NewRepo(ctx, logger, dummyRepoDir, path, walkWithSymlinks, allowCAS)
	}

	// Create a temporary root config file
	tmpDir := helpers.TmpDirWOSymlinks(t)
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

	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("List of Modules"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*2),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("q"),
	})

	tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second*2))

	fm := tm.FinalModel(t)
	finalModel, ok := fm.(tui.Model)
	require.True(t, ok, "final model should be of type tui.Model, got %T", fm)

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
	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("List of Modules"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*2),
	)

	// Press Enter to select the first module (assuming it's pre-selected)
	tm.Send(tea.KeyMsg{
		Type: tea.KeyEnter,
	})

	// Wait for the pager view to appear
	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			output := string(bts)
			// Check for pager view elements (scroll percentage, button bar)
			hasPercent := strings.Contains(output, "%")
			hasButton := strings.Contains(output, "Scaffold") || strings.Contains(output, "View Source")

			return hasPercent && hasButton
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	// Send 'q' to go back to list
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("q"),
	})

	// Wait for return to list view
	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("List of Modules"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*2),
	)

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
	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("List of Modules"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*2),
	)

	// Activate filtering with '/'
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("/"),
	})

	// Type filter text
	tm.Type("VPC")

	// Wait for filtering to take effect
	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			output := string(bts)
			// Should show filtered results containing "VPC"
			return strings.Contains(strings.ToUpper(output), "VPC")
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	// Press Escape to exit filtering
	tm.Send(tea.KeyMsg{
		Type: tea.KeyEsc,
	})

	// Wait for return to normal list view
	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			output := string(bts)
			// Should show both modules again
			return strings.Contains(output, "VPC") && strings.Contains(output, "EKS")
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*2),
	)

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
	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("List of Modules"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*2),
	)

	// Send window resize message
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Verify the interface handles resize gracefully
	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("List of Modules"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*2),
	)

	// Quit
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("q"),
	})

	tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second*2))
}

// TestTUIScaffoldWithRealRepository tests scaffold functionality using a real git repository
// This test requires network access and may be slower, but provides more realistic testing
func TestTUIScaffoldWithRealRepository(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Create a temp directory for scaffold output
	tempDir := helpers.TmpDirWOSymlinks(t)
	opts.WorkingDir = tempDir
	opts.ScaffoldRootFileName = config.RecommendedParentConfigName
	opts.ScaffoldVars = []string{"EnableRootInclude=false"}

	// Use real terraform-fake-modules repository
	repoURL := "https://github.com/gruntwork-io/terraform-fake-modules.git"
	svc := catalog.NewCatalogService(opts).WithRepoURL(repoURL)

	// Load modules from the real repository
	ctx := t.Context()
	l := logger.CreateLogger()
	err = svc.Load(ctx, l)
	require.NoError(t, err)

	modules := svc.Modules()
	require.NotEmpty(t, modules, "should have modules from real repository")

	m := tui.NewModel(l, opts, svc)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	// Wait for initial render
	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("List of Modules"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	// Press 'S' to scaffold the first module
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("S"),
	})

	// Wait for scaffold to complete - the application should quit after scaffolding
	tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second*10))

	fm := tm.FinalModel(t)
	finalModel, ok := fm.(tui.Model)
	require.True(t, ok, "final model should be of type model")

	// Verify the model transitioned to ScaffoldState
	assert.Equal(t, tui.ScaffoldState, finalModel.State)
	assert.NotNil(t, finalModel.SVC)
	assert.NotEmpty(t, finalModel.SVC.Modules())

	// Verify that a terragrunt.hcl file was actually created
	terragruntFile := filepath.Join(tempDir, "terragrunt.hcl")
	assert.FileExists(t, terragruntFile, "scaffold should create terragrunt.hcl file")
}
