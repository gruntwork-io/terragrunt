package test_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/format"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

const (
	testFixtureCatalogLocalTemplate = "fixtures/catalog/local-template"

	// catalogPipeWorkDirEnv carries the working directory the catalog pipe
	// child process renders, and marks the process as that child.
	catalogPipeWorkDirEnv = "CATALOG_PIPE_WORKING_DIR"

	// catalogEarlyExitModules is large enough that rendering every module
	// outgrows a pipe buffer, so the child is still writing when the reader
	// stops.
	catalogEarlyExitModules = 400
)

func TestCatalogGitRepoUpdate(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	tempDir := helpers.TmpDirWOSymlinks(t)

	_, err := module.NewRepo(
		ctx,
		logger.CreateLogger(),
		venv.OSVenv(),
		&module.RepoOpts{
			CloneURL: "github.com/gruntwork-io/terraform-fake-modules.git",
			Path:     tempDir,
		},
	)
	require.NoError(t, err)

	_, err = module.NewRepo(
		ctx,
		logger.CreateLogger(),
		venv.OSVenv(),
		&module.RepoOpts{
			CloneURL: "github.com/gruntwork-io/terraform-fake-modules.git",
			Path:     tempDir,
		},
	)
	require.NoError(t, err)
}

func TestScaffoldGitRepo(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	tempDir := helpers.TmpDirWOSymlinks(t)

	repo, err := module.NewRepo(
		ctx,
		logger.CreateLogger(),
		venv.OSVenv(),
		&module.RepoOpts{
			CloneURL: "github.com/gruntwork-io/terraform-fake-modules.git",
			Path:     tempDir,
		},
	)
	require.NoError(t, err)

	modules, err := repo.FindModules(ctx, logger.CreateLogger(), vfs.NewOSFS())
	require.NoError(t, err)
	assert.Len(t, modules, 4)
}

func TestScaffoldGitModule(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	tempDir := helpers.TmpDirWOSymlinks(t)

	repo, err := module.NewRepo(
		ctx,
		logger.CreateLogger(),
		venv.OSVenv(),
		&module.RepoOpts{
			CloneURL: "https://github.com/gruntwork-io/terraform-fake-modules.git",
			Path:     tempDir,
		},
	)
	require.NoError(t, err)

	modules, err := repo.FindModules(ctx, logger.CreateLogger(), vfs.NewOSFS())
	require.NoError(t, err)

	var auroraModule *module.Module

	for _, m := range modules {
		if m.Title() == "Terraform Fake AWS Aurora Module" {
			auroraModule = m
		}
	}

	assert.NotNil(t, auroraModule)

	testPath := helpers.TmpDirWOSymlinks(t)
	opts, err := options.NewTerragruntOptionsForTest(testPath)
	require.NoError(t, err)

	opts.ScaffoldVars = []string{"EnableRootInclude=false"}

	err = scaffold.Run(
		ctx,
		createLogger(),
		venv.OSVenv(),
		opts,
		auroraModule.TerraformSourcePath(),
		"",
	)
	require.NoError(t, err)

	cfg := readConfig(t, opts)
	assert.NotEmpty(t, cfg.Inputs)
	assert.Len(t, cfg.Inputs, 1)
	_, found := cfg.Inputs["vpc_id"]
	assert.True(t, found)
	assert.Contains(
		t,
		*cfg.Terraform.Source,
		"git::https://github.com/gruntwork-io/terraform-fake-modules.git//modules/aws/aurora",
	)
}

func TestScaffoldGitModuleHttps(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	tempDir := helpers.TmpDirWOSymlinks(t)

	repo, err := module.NewRepo(
		ctx,
		logger.CreateLogger(),
		venv.OSVenv(),
		&module.RepoOpts{
			CloneURL: "https://github.com/gruntwork-io/terraform-fake-modules",
			Path:     tempDir,
		},
	)
	require.NoError(t, err)

	modules, err := repo.FindModules(ctx, logger.CreateLogger(), vfs.NewOSFS())
	require.NoError(t, err)

	var auroraModule *module.Module

	for _, m := range modules {
		if m.Title() == "Terraform Fake AWS Aurora Module" {
			auroraModule = m
		}
	}

	assert.NotNil(t, auroraModule)

	testPath := helpers.TmpDirWOSymlinks(t)
	opts, err := options.NewTerragruntOptionsForTest(testPath)
	require.NoError(t, err)

	opts.ScaffoldVars = []string{"EnableRootInclude=false"}

	err = scaffold.Run(
		ctx,
		createLogger(),
		venv.OSVenv(),
		opts,
		auroraModule.TerraformSourcePath(),
		"",
	)
	require.NoError(t, err)

	cfg := readConfig(t, opts)
	assert.NotEmpty(t, cfg.Inputs)
	assert.Len(t, cfg.Inputs, 1)
	_, found := cfg.Inputs["vpc_id"]
	assert.True(t, found)
	assert.Contains(
		t,
		*cfg.Terraform.Source,
		"git::https://github.com/gruntwork-io/terraform-fake-modules.git//modules/aws/aurora?ref=v0.0.5",
	)

	helpers.RunTerragrunt(t, "terragrunt init --non-interactive --working-dir "+opts.WorkingDir)
}

func TestCatalogWithLocalDefaultTemplate(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCatalogLocalTemplate, ".boilerplate")
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureCatalogLocalTemplate)

	targetPath := filepath.Join(rootPath, "app")
	moduleURL := "github.com/gruntwork-io/terragrunt//test/fixtures/inputs"

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt scaffold --non-interactive --working-dir "+targetPath+" "+moduleURL,
	)

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(targetPath, "terragrunt.hcl"))
	assert.FileExists(t, filepath.Join(targetPath, "custom-template.txt"))

	content, err := util.ReadFileAsString(filepath.Join(targetPath, "terragrunt.hcl"))
	require.NoError(t, err)
	assert.Contains(t, content, "# Custom local template")
}

func readConfig(t *testing.T, opts *options.TerragruntOptions) *config.TerragruntConfig {
	t.Helper()

	assert.FileExists(t, opts.WorkingDir+"/terragrunt.hcl")

	opts, err := options.NewTerragruntOptionsForTest(
		filepath.Join(opts.WorkingDir, "terragrunt.hcl"),
	)
	require.NoError(t, err)

	l := logger.CreateLogger()
	_, pctx := configbridge.NewParsingContext(t.Context(), l, opts)
	cfg, err := config.ReadTerragruntConfig(
		t.Context(),
		l,
		pctx,
		config.DefaultParserOptions(l, opts.StrictControls),
	)
	require.NoError(t, err)

	return cfg
}

// TestCatalogIgnoreFileFlagAction drives the --ignore-file flag's
// Action the same way the CLI parser would: it resolves relative paths
// against opts.WorkingDir, rejects missing paths and directories, and
// writes the resolved absolute path back to opts.CatalogIgnoreFile.
func TestCatalogIgnoreFileFlagAction(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()

	ignoreFile := filepath.Join(workDir, "ignore-rules")
	require.NoError(t, os.WriteFile(ignoreFile, []byte("examples\n"), 0644))

	externalDir := t.TempDir()
	externalFile := filepath.Join(externalDir, "external-rules")
	require.NoError(t, os.WriteFile(externalFile, []byte("test/**\n"), 0644))

	cases := []struct {
		name       string
		input      string
		wantResult string
		wantErr    bool
	}{
		{
			name:       "empty input is a no-op",
			input:      "",
			wantResult: "",
		},
		{
			name:       "absolute path passes through",
			input:      externalFile,
			wantResult: externalFile,
		},
		{
			name:       "relative path resolves against WorkingDir",
			input:      "ignore-rules",
			wantResult: ignoreFile,
		},
		{
			name:    "missing path is rejected",
			input:   filepath.Join(workDir, "does-not-exist"),
			wantErr: true,
		},
		{
			name:    "directory is rejected",
			input:   workDir,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest(
				filepath.Join(workDir, "terragrunt.hcl"),
			)
			require.NoError(t, err)

			opts.WorkingDir = workDir

			action := ignoreFileAction(t, opts)

			err = action(t.Context(), nil, tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, opts.CatalogIgnoreFile)
		})
	}
}

// TestCatalogDiscoveryWithIgnoreFiles exercises the full discovery
// pipeline against a local fixture: whole-repo walk, module/template
// classification, repo-root .terragrunt-catalog-ignore, and a layered
// --ignore-file (with negation that re-includes a path the repo file
// would otherwise exclude).
func TestCatalogDiscoveryWithIgnoreFiles(t *testing.T) {
	t.Parallel()

	repoDir := helpers.TmpDirWOSymlinks(t)

	writeFixtureFile(t, filepath.Join(repoDir, "modules", "vpc", "main.tf"), "# vpc module")
	writeFixtureFile(
		t,
		filepath.Join(repoDir, "templates", "service", ".boilerplate", "boilerplate.yml"),
		"variables: []\n",
	)
	writeFixtureFile(
		t,
		filepath.Join(repoDir, "examples", "vpc", "main.tf"),
		"# ignored by repo file",
	)
	writeFixtureFile(
		t,
		filepath.Join(repoDir, "integration", "vpc", "main.tf"),
		"# ignored by extra file",
	)
	writeFixtureFile(
		t,
		filepath.Join(repoDir, "stash", "keep", "main.tf"),
		"# re-included by extra negation",
	)
	writeFixtureFile(t, filepath.Join(repoDir, "stash", "drop", "main.tf"), "# still excluded")

	writeFixtureFile(t, filepath.Join(repoDir, ".terragrunt-catalog-ignore"),
		"examples\nexamples/**\nstash/**\n")

	extraDir := t.TempDir()
	extraIgnore := filepath.Join(extraDir, "extra-ignore")
	require.NoError(t, os.WriteFile(extraIgnore, []byte("integration/**\n!stash/keep\n"), 0644))

	seedFakeGit(t, repoDir)

	repo, err := module.NewRepo(
		t.Context(),
		logger.CreateLogger(),
		venv.OSVenv(),
		&module.RepoOpts{
			CloneURL:       repoDir,
			Path:           repoDir,
			RootWorkingDir: repoDir,
		},
	)
	require.NoError(t, err)

	components, err := tui.NewComponentDiscovery().
		WithExtraIgnoreFile(extraIgnore).
		Discover(vfs.NewOSFS(), repo)
	require.NoError(t, err)

	got := map[string]tui.ComponentKind{}
	for _, c := range components {
		got[c.Dir] = c.Kind
	}

	want := map[string]tui.ComponentKind{
		"modules/vpc":       tui.ComponentKindModule,
		"templates/service": tui.ComponentKindTemplate,
		"stash/keep":        tui.ComponentKindModule,
	}

	assert.Equal(t, want, got)
}

// TestCatalogNonTTYFailsFast verifies that running the catalog command
// without an interactive terminal exits with the friendly typed error
// instead of bubbletea's raw TTY failure.
//
// The guard mirrors the command's own TTY probe: when the test environment
// has a controlling terminal (a developer's shell), the command would
// launch the real TUI and block, so the test only runs where a TTY is
// genuinely unavailable (e.g. CI runners).
func TestCatalogNonTTYFailsFast(t *testing.T) {
	t.Parallel()

	if term.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("stdin is a terminal; the catalog TUI would launch for real")
	}

	if in, out, err := tea.OpenTTY(); err == nil {
		closeErr := in.Close()
		if out != in {
			closeErr = errors.Join(closeErr, out.Close())
		}

		require.NoError(t, closeErr)
		t.Skip("a controlling terminal is available; the catalog TUI would launch for real")
	}

	workDir := t.TempDir()

	_, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt catalog --working-dir "+workDir)

	require.Error(t, err)
	require.ErrorIs(t, err, tui.ErrNoTerminal)
}

// TestCatalogJSONLFormat renders a catalog non-interactively, one JSON object
// per line, and checks the components it discovered.
func TestCatalogJSONLFormat(t *testing.T) {
	t.Parallel()

	workDir := catalogJSONLFixture(t)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt catalog --experiment catalog-format --format jsonl --working-dir "+workDir)
	require.NoError(t, err)

	byDir := parseCatalogJSONL(t, stdout)

	kinds := map[string]string{}
	for dir, entry := range byDir {
		kinds[dir] = entry.Kind
	}

	assert.Equal(t, map[string]string{
		"modules/vpc":       "module",
		"templates/service": "template",
		"units/app":         "unit",
		"stacks/prod":       "stack",
	}, kinds)

	vpc := byDir["modules/vpc"]
	assert.Equal(t, "VPC", vpc.Title)
	assert.Equal(t, "Creates a VPC.", vpc.Description)
	assert.Equal(t, []string{"networking"}, vpc.Tags)
	assert.Contains(t, vpc.Doc, "Everything a VPC needs.")
	assert.False(t, vpc.Copyable)

	assert.True(t, byDir["units/app"].Copyable)
	assert.True(t, byDir["stacks/prod"].Copyable)
}

// TestCatalogJSONLFormatWithoutTTY guards the non-interactive path against the
// terminal check that the TUI needs, which would make the command unusable in
// CI. It mirrors the skip of [TestCatalogNonTTYFailsFast]: where a terminal is
// available, a regression would launch the TUI and block instead of failing.
func TestCatalogJSONLFormatWithoutTTY(t *testing.T) {
	t.Parallel()

	if term.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("stdin is a terminal; a regression would launch the catalog TUI for real")
	}

	if in, out, err := tea.OpenTTY(); err == nil {
		closeErr := in.Close()
		if out != in {
			closeErr = errors.Join(closeErr, out.Close())
		}

		require.NoError(t, closeErr)
		t.Skip("a controlling terminal is available; a regression would launch the catalog TUI for real")
	}

	workDir := catalogJSONLFixture(t)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt catalog --experiment catalog-format --format jsonl --working-dir "+workDir)
	require.NoError(t, err)
	assert.Len(t, parseCatalogJSONL(t, stdout), 4)
}

func TestCatalogJSONLFormatRequiresExperiment(t *testing.T) {
	t.Parallel()

	if helpers.IsExperimentMode(t) {
		t.Skip(
			"Skipping: TG_EXPERIMENT_MODE forces all experiments on, opening the gate this test pins shut",
		)
	}

	workDir := catalogJSONLFixture(t)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt catalog --format jsonl --working-dir "+workDir)

	require.ErrorIs(t, err, catalog.ErrFormatRequiresExperiment)
	assert.Empty(t, stdout)
}

// TestCatalogUnknownFormat uses a format nothing plans to implement, so that
// adding a renderer never turns this into a failure that has to be chased.
func TestCatalogUnknownFormat(t *testing.T) {
	t.Parallel()

	workDir := catalogJSONLFixture(t)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt catalog --experiment catalog-format --format pdf --working-dir "+workDir)

	require.Error(t, err)
	assert.Empty(t, stdout)
}

// TestCatalogJSONLFormatCleansUpOnEarlyExit covers `terragrunt catalog
// --format=jsonl | head -1`: the reader stops, and the command must still
// remove the repositories it cloned into the temporary directory.
//
// It runs in a child process because standard output has to be a real pipe.
// A write to a closed pipe on file descriptor 1 is the only thing that
// reaches the path this guards, and the test process cannot produce one
// without taking over its own standard output.
func TestCatalogJSONLFormatCleansUpOnEarlyExit(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("SIGPIPE, and the early exit it used to cause, do not exist on Windows")
	}

	workDir := catalogManyModulesFixture(t, catalogEarlyExitModules)
	childTempDir := t.TempDir()

	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCatalogPipeHelper$")

	cmd.Env = append(
		os.Environ(),
		catalogPipeWorkDirEnv+"="+workDir,
		"TMPDIR="+childTempDir,
	)

	var childStderr bytes.Buffer

	cmd.Stderr = &childStderr

	childStdout, err := cmd.StdoutPipe()
	require.NoError(t, err)

	require.NoError(t, cmd.Start())

	// Reading one record and closing is what `head -1` does. The fixture is
	// large enough that the child is still writing at that point.
	line, err := bufio.NewReader(childStdout).ReadString('\n')
	require.NoError(t, err)
	require.NoError(t, childStdout.Close())

	waitErr := cmd.Wait()

	var entry format.Entry

	require.NoError(t, json.Unmarshal([]byte(line), &entry))
	assert.Equal(t, "module", entry.Kind)

	leftovers, err := filepath.Glob(filepath.Join(childTempDir, "catalog-*"))
	require.NoError(t, err)
	assert.Empty(
		t,
		leftovers,
		"clone directories survived the early exit (child exit: %v, child stderr: %s)",
		waitErr,
		childStderr.String(),
	)
}

// TestCatalogPipeHelper renders a catalog to the process's own standard
// output. It is the child of [TestCatalogJSONLFormatCleansUpOnEarlyExit] and
// skips itself in every other run.
func TestCatalogPipeHelper(t *testing.T) {
	t.Parallel()

	workDir := os.Getenv(catalogPipeWorkDirEnv)
	if workDir == "" {
		t.Skip("not the catalog pipe child process")
	}

	require.NoError(t, helpers.RunTerragruntCommand(t,
		"terragrunt catalog --experiment catalog-format --format jsonl --working-dir "+workDir,
		os.Stdout, os.Stderr))
}

// catalogManyModulesFixture builds a repository of count modules, each with a
// README long enough that rendering them all outgrows a pipe buffer, and
// returns a working directory whose catalog configuration points at it.
func catalogManyModulesFixture(t *testing.T, count int) string {
	t.Helper()

	repoDir := helpers.TmpDirWOSymlinks(t)
	body := strings.Repeat("Everything this module needs. ", 20)

	for i := range count {
		name := fmt.Sprintf("m%04d", i)

		writeFixtureFile(t, filepath.Join(repoDir, "modules", name, "main.tf"), "# "+name)
		writeFixtureFile(
			t,
			filepath.Join(repoDir, "modules", name, "README.md"),
			"# "+name+"\n\n"+body+"\n",
		)
	}

	seedFakeGit(t, repoDir)

	workDir := helpers.TmpDirWOSymlinks(t)

	writeFixtureFile(t, filepath.Join(workDir, "terragrunt.hcl"), `catalog {
  urls = ["`+filepath.ToSlash(repoDir)+`"]
}
`)

	return workDir
}

// catalogJSONLFixture builds a repository holding one component of each kind
// and a working directory whose catalog configuration points at it, then
// returns the working directory.
func catalogJSONLFixture(t *testing.T) string {
	t.Helper()

	repoDir := helpers.TmpDirWOSymlinks(t)

	writeFixtureFile(t, filepath.Join(repoDir, "modules", "vpc", "main.tf"), "# vpc module")
	writeFixtureFile(t, filepath.Join(repoDir, "modules", "vpc", "README.md"), `---
name: VPC
description: Creates a VPC.
tags:
  - networking
---

Everything a VPC needs.
`)
	writeFixtureFile(
		t,
		filepath.Join(repoDir, "templates", "service", ".boilerplate", "boilerplate.yml"),
		"variables: []\n",
	)
	writeFixtureFile(t, filepath.Join(repoDir, "units", "app", "terragrunt.hcl"), "# app unit")
	writeFixtureFile(
		t,
		filepath.Join(repoDir, "stacks", "prod", "terragrunt.stack.hcl"),
		"# prod stack",
	)

	seedFakeGit(t, repoDir)

	workDir := helpers.TmpDirWOSymlinks(t)

	writeFixtureFile(t, filepath.Join(workDir, "terragrunt.hcl"), `catalog {
  urls = ["`+filepath.ToSlash(repoDir)+`"]
}
`)

	return workDir
}

// parseCatalogJSONL parses each line of rendered output and keys the entries
// by directory. Entries arrive in discovery order, so callers may not depend
// on the order they were written in.
func parseCatalogJSONL(t *testing.T, stdout string) map[string]*format.Entry {
	t.Helper()

	entries := map[string]*format.Entry{}

	for line := range strings.SplitSeq(strings.TrimSuffix(stdout, "\n"), "\n") {
		if line == "" {
			continue
		}

		entry := &format.Entry{}
		require.NoError(t, json.Unmarshal([]byte(line), entry))

		entries[entry.Dir] = entry
	}

	return entries
}

func ignoreFileAction(
	t *testing.T,
	opts *options.TerragruntOptions,
) clihelper.FlagActionFunc[string] {
	t.Helper()

	flagList := catalog.NewFlags(catalog.NewOptions(opts), nil)

	flag := flagList.Get(catalog.IgnoreFileFlagName)
	require.NotNil(t, flag, "--%s flag not registered", catalog.IgnoreFileFlagName)

	wrapper, ok := flag.(*flags.Flag)
	require.True(t, ok, "expected *flags.Flag wrapper, got %T", flag)

	inner, ok := wrapper.Flag.(*clihelper.GenericFlag[string])
	require.True(t, ok, "expected *clihelper.GenericFlag[string], got %T", wrapper.Flag)

	require.NotNil(t, inner.Action, "--%s flag is missing its Action", catalog.IgnoreFileFlagName)

	return inner.Action
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func seedFakeGit(t *testing.T, repoDir string) {
	t.Helper()

	gitDir := filepath.Join(repoDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte(`[core]
	repositoryformatversion = 0
[remote "origin"]
	url = github.com/gruntwork-io/fake-repo
`), 0644))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644),
	)
}
