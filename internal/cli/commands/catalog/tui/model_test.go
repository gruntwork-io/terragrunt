package tui_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

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

// runModel starts a tea.Program with the given model, sends messages via
// the interact callback, and returns the final model once the program exits.
// The program runs with a pipe for input, a buffer for output, and a fixed
// terminal size so tests are deterministic.
func runModel(t *testing.T, m tui.Model, width, height int, interact func(p *tea.Program)) tui.Model { //nolint:gocritic
	t.Helper()

	var out bytes.Buffer

	// Use a real pipe for input so that tea.Exec can release/restore the
	// terminal without hitting a nil reader.
	pr, pw, err := os.Pipe()
	require.NoError(t, err)

	defer pr.Close()
	defer pw.Close()

	p := tea.NewProgram(m,
		tea.WithInput(pr),
		tea.WithOutput(&out),
		tea.WithWindowSize(width, height),
		tea.WithColorProfile(colorprofile.TrueColor),
	)

	done := make(chan tea.Model, 1)

	go func() {
		finalModel, err := p.Run()
		assert.NoError(t, err)

		done <- finalModel
	}()

	// Give the program a moment to start and process the initial WindowSizeMsg.
	time.Sleep(50 * time.Millisecond)

	interact(p)

	select {
	case fm := <-done:
		return fm.(tui.Model)
	case <-time.After(10 * time.Second):
		p.Kill()
		t.Fatal("program did not exit within timeout")

		return tui.Model{}
	}
}

// createMockCatalogService creates a mock catalog service with test modules for testing
func createMockCatalogService(t *testing.T, opts *options.TerragruntOptions) catalog.CatalogService {
	t.Helper()

	mockNewRepo := func(ctx context.Context, logger log.Logger, repoOpts module.RepoOpts) (*module.Repo, error) {
		repoURL := repoOpts.CloneURL
		// Create a temporary directory structure for testing
		dummyRepoDir := filepath.Join(helpers.TmpDirWOSymlinks(t), strings.ReplaceAll(repoURL, "github.com/gruntwork-io/", ""))

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
			os.WriteFile(readme1Path, []byte("# AWS VPC Module\nThis module creates a VPC in AWS with all the necessary components."), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, "main.tf"), []byte("# VPC terraform configuration"), 0644)
		case "github.com/gruntwork-io/test-repo-2":
			readme2Path := filepath.Join(dummyRepoDir, "README.md")
			os.WriteFile(readme2Path, []byte("# AWS EKS Module\nThis module creates an EKS cluster in AWS."), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, "main.tf"), []byte("# EKS terraform configuration"), 0644)
		default:
			return nil, fmt.Errorf("unexpected repoURL in mock: %s", repoURL)
		}

		repoOpts.CloneURL = dummyRepoDir

		return module.NewRepo(ctx, logger, repoOpts)
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

	finalModel := runModel(t, m, 120, 40, func(p *tea.Program) {
		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	assert.Equal(t, tui.ListState, finalModel.State)
	assert.NotNil(t, finalModel.SVC)
	assert.NotNil(t, finalModel.List)
	assert.Len(t, finalModel.SVC.Modules(), 2, "should have 2 test modules")
}

func TestTUINavigationToModuleDetails(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	svc := createMockCatalogService(t, opts)
	l := logger.CreateLogger()

	m := tui.NewModel(l, opts, svc)

	finalModel := runModel(t, m, 120, 40, func(p *tea.Program) {
		// Press Enter to select the first module
		p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
		time.Sleep(50 * time.Millisecond)

		// Press 'q' to go back to list
		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
		time.Sleep(50 * time.Millisecond)

		// Quit
		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	assert.Equal(t, tui.ListState, finalModel.State)
}

func TestTUIModuleFiltering(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	svc := createMockCatalogService(t, opts)
	l := logger.CreateLogger()

	m := tui.NewModel(l, opts, svc)

	finalModel := runModel(t, m, 120, 40, func(p *tea.Program) {
		// Activate filtering with '/'
		p.Send(tea.KeyPressMsg{Code: '/', Text: "/"})
		time.Sleep(50 * time.Millisecond)

		// Type filter text
		for _, r := range "VPC" {
			p.Send(tea.KeyPressMsg{Code: r, Text: string(r)})
		}

		time.Sleep(50 * time.Millisecond)

		// Press Escape to exit filtering
		p.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
		time.Sleep(50 * time.Millisecond)

		// Quit
		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	assert.Equal(t, tui.ListState, finalModel.State)
}

func TestTUIWindowResize(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	svc := createMockCatalogService(t, opts)
	l := logger.CreateLogger()

	m := tui.NewModel(l, opts, svc)

	finalModel := runModel(t, m, 80, 30, func(p *tea.Program) {
		// Send window resize
		p.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
		time.Sleep(50 * time.Millisecond)

		// Quit
		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	assert.Equal(t, tui.ListState, finalModel.State)
}

// TestModelStreamingInsertsSorted verifies that modules sent via moduleMsg
// are inserted in alphabetical order in the list.
func TestModelStreamingInsertsSorted(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	svc := createMockCatalogService(t, opts)
	modules := svc.Modules()
	require.GreaterOrEqual(t, len(modules), 2, "need at least 2 modules")

	// Start with the last module alphabetically
	moduleCh := make(chan *module.Module, len(modules))
	m := tui.NewModelStreaming(l, opts, modules[len(modules)-1], moduleCh)

	finalModel := runModel(t, m, 120, 40, func(p *tea.Program) {
		// Send the remaining modules in reverse order
		for i := len(modules) - 2; i >= 0; i-- {
			p.Send(tui.ModuleMsg(modules[i]))
			time.Sleep(50 * time.Millisecond)
		}

		time.Sleep(100 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	assert.Equal(t, tui.ListState, finalModel.State)
	items := finalModel.List.Items()
	assert.Len(t, items, len(modules), "all modules should be in the list")

	// Verify sorted order
	for i := 1; i < len(items); i++ {
		prev := items[i-1].(*module.Module).Title()
		curr := items[i].(*module.Module).Title()
		assert.LessOrEqual(t, prev, curr, "modules should be in alphabetical order: %q should come before %q", prev, curr)
	}
}

// TestModelStreamingDeduplicates verifies that sending the same module
// twice does not result in a duplicate entry in the list.
func TestModelStreamingDeduplicates(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	svc := createMockCatalogService(t, opts)
	modules := svc.Modules()
	require.NotEmpty(t, modules)

	moduleCh := make(chan *module.Module, len(modules))
	m := tui.NewModelStreaming(l, opts, modules[0], moduleCh)

	finalModel := runModel(t, m, 120, 40, func(p *tea.Program) {
		// Send the same module again — should be deduplicated
		p.Send(tui.ModuleMsg(modules[0]))
		time.Sleep(100 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	assert.Equal(t, tui.ListState, finalModel.State)
	assert.Len(t, finalModel.List.Items(), 1, "duplicate module should not appear twice")
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
	svc := catalog.NewCatalogService(opts).WithRepoURL("https://github.com/gruntwork-io/terraform-fake-modules.git")

	// Load modules from the real repository
	ctx := t.Context()
	l := logger.CreateLogger()
	err = svc.Load(ctx, l)
	require.NoError(t, err)

	modules := svc.Modules()
	require.NotEmpty(t, modules, "should have modules from real repository")

	m := tui.NewModel(l, opts, svc)

	finalModel := runModel(t, m, 120, 40, func(p *tea.Program) {
		// Press 'S' to scaffold the first module
		p.Send(tea.KeyPressMsg{Code: 'S', Text: "S"})
	})

	// Verify the model transitioned to ScaffoldState
	assert.Equal(t, tui.ScaffoldState, finalModel.State)
	assert.NotNil(t, finalModel.SVC)
	assert.NotEmpty(t, finalModel.SVC.Modules())

	// Verify that a terragrunt.hcl file was actually created
	terragruntFile := filepath.Join(tempDir, "terragrunt.hcl")
	assert.FileExists(t, terragruntFile, "scaffold should create terragrunt.hcl file")
}
