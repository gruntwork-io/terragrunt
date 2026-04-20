package redesign_test

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

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
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
func runModel(t *testing.T, m redesign.Model, width, height int, interact func(p *tea.Program)) redesign.Model { //nolint:gocritic
	t.Helper()

	var out bytes.Buffer

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

	time.Sleep(50 * time.Millisecond)

	interact(p)

	select {
	case fm := <-done:
		return fm.(redesign.Model)
	case <-time.After(10 * time.Second):
		p.Kill()
		t.Fatal("program did not exit within timeout")

		return redesign.Model{}
	}
}

// createMockCatalogService creates a mock catalog service with test modules for testing
func createMockCatalogService(t *testing.T, opts *options.TerragruntOptions) catalog.CatalogService {
	t.Helper()

	mockNewRepo := func(ctx context.Context, logger log.Logger, repoOpts module.RepoOpts) (*module.Repo, error) {
		repoURL := repoOpts.CloneURL
		dummyRepoDir := filepath.Join(helpers.TmpDirWOSymlinks(t), strings.ReplaceAll(repoURL, "github.com/gruntwork-io/", ""))

		require.NoError(t, os.MkdirAll(dummyRepoDir, 0755), "MkdirAll %s", dummyRepoDir)

		gitDir := filepath.Join(dummyRepoDir, ".git")
		require.NoError(t, os.MkdirAll(gitDir, 0755), "MkdirAll %s", gitDir)
		require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), fmt.Appendf(nil, `[core]
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
`, repoURL), 0644), "WriteFile %s/config", gitDir)
		require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644), "WriteFile %s/HEAD", gitDir)

		refsDir := filepath.Join(gitDir, "refs")
		headsDir := filepath.Join(refsDir, "heads")
		remotesDir := filepath.Join(refsDir, "remotes", "origin")

		require.NoError(t, os.MkdirAll(headsDir, 0755), "MkdirAll %s", headsDir)
		require.NoError(t, os.MkdirAll(remotesDir, 0755), "MkdirAll %s", remotesDir)

		fakeCommitHash := "1234567890abcdef1234567890abcdef12345678"
		require.NoError(t, os.WriteFile(filepath.Join(headsDir, "main"), []byte(fakeCommitHash+"\n"), 0644), "WriteFile %s/main", headsDir)
		require.NoError(t, os.WriteFile(filepath.Join(remotesDir, "main"), []byte(fakeCommitHash+"\n"), 0644), "WriteFile %s/main", remotesDir)

		switch repoURL {
		case "github.com/gruntwork-io/test-repo-1":
			readme1Path := filepath.Join(dummyRepoDir, "README.md")
			require.NoError(t, os.WriteFile(readme1Path, []byte("# AWS VPC Module\nThis module creates a VPC in AWS with all the necessary components."), 0644), "WriteFile %s", readme1Path)

			mainTF1 := filepath.Join(dummyRepoDir, "main.tf")
			require.NoError(t, os.WriteFile(mainTF1, []byte("# VPC terraform configuration"), 0644), "WriteFile %s", mainTF1)
		case "github.com/gruntwork-io/test-repo-2":
			readme2Path := filepath.Join(dummyRepoDir, "README.md")
			require.NoError(t, os.WriteFile(readme2Path, []byte("# AWS EKS Module\nThis module creates an EKS cluster in AWS."), 0644), "WriteFile %s", readme2Path)

			mainTF2 := filepath.Join(dummyRepoDir, "main.tf")
			require.NoError(t, os.WriteFile(mainTF2, []byte("# EKS terraform configuration"), 0644), "WriteFile %s", mainTF2)
		default:
			return nil, fmt.Errorf("unexpected repoURL in mock: %s", repoURL)
		}

		repoOpts.CloneURL = dummyRepoDir

		return module.NewRepo(ctx, logger, repoOpts)
	}

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
	require.NoError(t, os.MkdirAll(unitDir, 0755), "MkdirAll %s", unitDir)
	opts.TerragruntConfigPath = filepath.Join(unitDir, "terragrunt.hcl")
	opts.ScaffoldRootFileName = config.RecommendedParentConfigName

	svc := catalog.NewCatalogService(opts).WithNewRepoFunc(mockNewRepo)

	ctx := t.Context()
	l := logger.CreateLogger()
	err = svc.Load(ctx, l)
	require.NoError(t, err)

	return svc
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
	moduleCh := make(chan *redesign.ModuleEntry, len(modules))
	m := redesign.NewModelStreaming(l, opts, redesign.NewModuleEntry(modules[len(modules)-1]), moduleCh)

	finalModel := runModel(t, m, 120, 40, func(p *tea.Program) {
		// Send the remaining modules in reverse order
		for i := len(modules) - 2; i >= 0; i-- {
			p.Send(redesign.ModuleMsg(redesign.NewModuleEntry(modules[i])))
			time.Sleep(50 * time.Millisecond)
		}

		time.Sleep(100 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	assert.Equal(t, redesign.ListState, finalModel.State)
	items := finalModel.List.Items()
	assert.Len(t, items, len(modules), "all modules should be in the list")

	// Verify sorted order (case-insensitive, matching the sort in model.go)
	for i := 1; i < len(items); i++ {
		prev := strings.ToLower(items[i-1].(*redesign.ModuleEntry).Title())
		curr := strings.ToLower(items[i].(*redesign.ModuleEntry).Title())
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

	moduleCh := make(chan *redesign.ModuleEntry, len(modules))
	m := redesign.NewModelStreaming(l, opts, redesign.NewModuleEntry(modules[0]), moduleCh)

	finalModel := runModel(t, m, 120, 40, func(p *tea.Program) {
		// Send the same module again — should be deduplicated
		p.Send(redesign.ModuleMsg(redesign.NewModuleEntry(modules[0])))
		time.Sleep(100 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	assert.Equal(t, redesign.ListState, finalModel.State)
	assert.Len(t, finalModel.List.Items(), 1, "duplicate module should not appear twice")
}
