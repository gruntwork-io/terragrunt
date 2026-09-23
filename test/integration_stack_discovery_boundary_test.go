package test_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// boundaryStackCatalogStack only parses once generated beside the roles.yml it reads.
const boundaryStackCatalogStack = `locals {
  roles = yamldecode(file(find_in_parent_folders("roles.yml")))
}

unit "account" {
  source = "${get_repo_root()}/catalog/units/account"
  path   = "account"
}

unit "roles" {
  source = "${get_repo_root()}/catalog/units/roles"
  path   = "roles"

  values = {
    roles = local.roles
  }
}
`

// boundaryStackCatalogRolesUnit only parses once generated beside the roles.yml it reads.
const boundaryStackCatalogRolesUnit = `locals {
  roles = yamldecode(file(find_in_parent_folders("roles.yml")))
}

dependency "account" {
  config_path = "../account"

  mock_outputs = {
    account_id = "000000000000"
  }
}

inputs = {
  account_id = dependency.account.outputs.account_id
  roles      = values.roles
}
`

const boundaryStackAccountModule = `output "account_id" {
  value = "000000000000"
}
`

const boundaryStackRolesModule = `variable "account_id" {}

variable "roles" {}
`

const boundaryStackLiveStack = `stack "account" {
  source = "${get_repo_root()}/catalog/stacks/account"
  path   = "account"
}
`

// boundaryStackStandaloneUnit tolerates its read file being deleted, so a deletion diff still parses.
const boundaryStackStandaloneUnit = `locals {
  extra = try(file(mark_as_read("${get_terragrunt_dir()}/extra.yml")), "")
}
`

const boundaryStackStandaloneModule = `output "ok" {
  value = "ok"
}
`

// boundaryStackAppUnit depends on the sibling db unit so dependency traversal has something to follow.
const boundaryStackAppUnit = `dependency "db" {
  config_path = "../db"

  mock_outputs = {
    id = "mock"
  }
}

inputs = {
  db_id = dependency.db.outputs.id
}
`

const boundaryStackDBModule = `output "id" {
  value = "db"
}
`

const boundaryStackAppModule = `variable "db_id" {}
`

const (
	boundaryStackAppDir         = "live/app"
	boundaryStackDBDir          = "live/db"
	boundaryStackStandaloneDir  = "live/standalone"
	boundaryStackStandaloneRead = boundaryStackStandaloneDir + "/extra.yml"
	boundaryStackRolesFile      = "live/accounts/my-account/roles.yml"
	boundaryStackLiveStackFile  = "live/accounts/my-account/terragrunt.stack.hcl"
	boundaryStackCatalogFile    = "catalog/stacks/account/terragrunt.stack.hcl"
	boundaryStackCatalogUnit    = "catalog/units/account/terragrunt.hcl"
	boundaryStackUnusedUnit     = "catalog/units/unused/terragrunt.hcl"
	boundaryStackOtherUnit      = "other/unit/terragrunt.hcl"
	boundaryStackGeneratedDir   = "live/accounts/my-account/.terragrunt-stack/account"
	boundaryStackRolesUnitDir   = boundaryStackGeneratedDir + "/.terragrunt-stack/roles"
	boundaryStackBoundedFilter  = "--filter '(./live/)...[main...HEAD]'"
)

// TestStackDiscoveryBoundaryGitFilterSkipsCatalog pins that worktree stack generation stays inside the boundary.
func TestStackDiscoveryBoundaryGitFilterSkipsCatalog(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		changed  string
		command  string
		expected []string
	}{
		{
			name:     "find with read file change",
			changed:  boundaryStackRolesFile,
			command:  "find",
			expected: []string{boundaryStackGeneratedDir, boundaryStackRolesUnitDir},
		},
		{
			name:     "find with live stack file change",
			changed:  boundaryStackLiveStackFile,
			command:  "find",
			expected: []string{"live/accounts/my-account"},
		},
		{
			name:    "find with catalog stack file change",
			changed: boundaryStackCatalogFile,
			command: "find",
		},
		{name: "stack generate with read file change", changed: boundaryStackRolesFile, command: "stack generate"},
		{name: "stack generate with live stack file change", changed: boundaryStackLiveStackFile, command: "stack generate"},
		{name: "stack generate with catalog stack file change", changed: boundaryStackCatalogFile, command: "stack generate"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir, runner := setupBoundaryStackRepo(t)

			appendBoundaryStackChange(t, runner, filepath.Join(tmpDir, tc.changed))

			stdout, stderr, err := runBoundaryStackCommand(t, tmpDir, tc.command, boundaryStackBoundedFilter)
			require.NoError(t, err, "stderr: %s", stderr)
			assert.NotContains(t, stderr, boundaryStackCatalogFile)

			if tc.command == "find" {
				assert.ElementsMatch(t, tc.expected, outputLines(stdout))
				return
			}

			assert.DirExists(t, filepath.Join(tmpDir, filepath.FromSlash(boundaryStackRolesUnitDir)))
			assert.NoDirExists(t, filepath.Join(tmpDir, "catalog", "stacks", "account", ".terragrunt-stack"))
		})
	}
}

// TestStackDiscoveryBoundaryGitFilterBoundsTargets pins that the boundary confines Git worktree unit discovery.
func TestStackDiscoveryBoundaryGitFilterBoundsTargets(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		changed  string
		removed  string
		workDir  string
		args     string
		expected []string
	}{
		{
			name:     "unbounded Git filter from a subdirectory spans the repository",
			changed:  boundaryStackStandaloneDir + "/terragrunt.hcl",
			workDir:  "live/accounts",
			args:     "--filter '[main...HEAD]'",
			expected: []string{boundaryStackStandaloneDir},
		},
		{
			name:     "Git boundary from a subdirectory resolves against the repository root",
			changed:  boundaryStackStandaloneDir + "/terragrunt.hcl",
			workDir:  "live/accounts",
			args:     "--filter '(./live/standalone)...[main...HEAD]'",
			expected: []string{boundaryStackStandaloneDir},
		},
		{
			name:     "changed sibling unit under a wider Git boundary",
			changed:  boundaryStackStandaloneDir + "/terragrunt.hcl",
			workDir:  "live/accounts",
			args:     "--filter '(./live)...[main...HEAD]'",
			expected: []string{boundaryStackStandaloneDir},
		},
		{
			name:     "dependency-side boundary does not hide changed units",
			changed:  boundaryStackStandaloneDir + "/terragrunt.hcl",
			args:     "--filter '[main...HEAD]...(./catalog)'",
			expected: []string{boundaryStackStandaloneDir},
		},
		{
			name:    "changed catalog unit outside the boundary",
			changed: boundaryStackCatalogUnit,
			args:    boundaryStackBoundedFilter,
		},
		{
			name:    "removed catalog unit outside the boundary",
			removed: boundaryStackUnusedUnit,
			args:    boundaryStackBoundedFilter,
		},
		{
			name:     "changed read file inside the boundary",
			changed:  boundaryStackRolesFile,
			args:     boundaryStackBoundedFilter,
			expected: []string{boundaryStackGeneratedDir, boundaryStackRolesUnitDir},
		},
		{
			name:     "deleted read file inside the boundary",
			removed:  boundaryStackStandaloneRead,
			args:     boundaryStackBoundedFilter,
			expected: []string{boundaryStackStandaloneDir},
		},
		{
			name:     "flag boundary inside the working directory",
			changed:  boundaryStackRolesFile,
			args:     "--discovery-boundary ./live --filter '...[main...HEAD]'",
			expected: []string{boundaryStackGeneratedDir, boundaryStackRolesUnitDir},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir, runner := setupBoundaryStackRepo(t)

			if tc.changed != "" {
				appendBoundaryStackChange(t, runner, filepath.Join(tmpDir, tc.changed))
			}

			if tc.removed != "" {
				removeBoundaryStackPath(t, runner, filepath.Join(tmpDir, tc.removed))
			}

			stdout, stderr, err := runBoundaryStackCommand(
				t,
				filepath.Join(tmpDir, filepath.FromSlash(tc.workDir)),
				"find",
				tc.args,
			)
			require.NoError(t, err, "stderr: %s", stderr)
			assert.ElementsMatch(t, tc.expected, outputLines(stdout))
			assert.NotContains(t, stderr, "catalog/")
		})
	}
}

// TestStackDiscoveryBoundaryGitTargetsResolvePerWorktree pins that Git targets resolve boundaries in their own worktree.
func TestStackDiscoveryBoundaryGitTargetsResolvePerWorktree(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		changed      string
		deleteInTree string
		workDir      string
		args         string
		errText      string
		expected     []string
	}{
		{
			name:     "dependency-side boundary keeps the dependencies of a changed unit",
			changed:  boundaryStackAppDir + "/terragrunt.hcl",
			args:     "--filter '[HEAD^...HEAD]...(./live)'",
			expected: []string{boundaryStackAppDir, boundaryStackDBDir},
		},
		{
			name:     "dependency-side boundary from a subdirectory",
			changed:  boundaryStackAppDir + "/terragrunt.hcl",
			workDir:  "other",
			args:     "--filter '[HEAD^...HEAD]...(./live)'",
			expected: []string{boundaryStackAppDir, boundaryStackDBDir},
		},
		{
			name:         "dependent-side boundary finds a dependent deleted in the working tree",
			changed:      boundaryStackDBDir + "/terragrunt.hcl",
			deleteInTree: boundaryStackAppDir,
			args:         "--filter '(./live)...[HEAD^...HEAD]'",
			expected:     []string{boundaryStackDBDir, boundaryStackAppDir},
		},
		{
			name:         "dependent-side boundary from a subdirectory",
			changed:      boundaryStackDBDir + "/terragrunt.hcl",
			deleteInTree: boundaryStackAppDir,
			workDir:      "other",
			args:         "--filter '(./live)...[HEAD^...HEAD]'",
			expected:     []string{boundaryStackDBDir, boundaryStackAppDir},
		},
		{
			name:    "boundary at neither reference is an error",
			changed: boundaryStackDBDir + "/terragrunt.hcl",
			args:    "--filter '(./nope)...[HEAD^...HEAD]'",
			errText: "not a directory at either compared reference",
		},
		{
			name:    "dependency-side boundary at neither reference is an error",
			changed: boundaryStackAppDir + "/terragrunt.hcl",
			args:    "--filter '[HEAD^...HEAD]...(./missing)'",
			errText: "not a directory at either compared reference",
		},
		{
			name:     "flag bounds the dependencies of a Git target in its worktree",
			changed:  boundaryStackAppDir + "/terragrunt.hcl",
			args:     "--filter '[HEAD^...HEAD]...' --discovery-boundary ./live",
			expected: []string{boundaryStackAppDir, boundaryStackDBDir},
		},
		{
			name:     "flag above the repository root covers it",
			changed:  boundaryStackDBDir + "/terragrunt.hcl",
			workDir:  "live",
			args:     "--filter '...[HEAD^...HEAD]' --discovery-boundary ..",
			expected: []string{boundaryStackDBDir, boundaryStackAppDir},
		},
		{
			name:     "flag bounds the dependents of a Git target in its worktree",
			changed:  boundaryStackDBDir + "/terragrunt.hcl",
			args:     "--filter '...[HEAD^...HEAD]' --discovery-boundary ./live",
			expected: []string{boundaryStackDBDir, boundaryStackAppDir},
		},
		{
			name:     "non-Git boundary next to a Git boundary from a subdirectory",
			changed:  boundaryStackAppDir + "/terragrunt.hcl",
			workDir:  "live",
			args:     "--filter '(./db)...{./db}' --filter '(./live)...[HEAD^...HEAD]'",
			expected: []string{"db", boundaryStackAppDir},
		},
		{
			name:     "flag does not hide changes for a filter without dependents",
			changed:  boundaryStackOtherUnit,
			args:     "--filter '[HEAD^...HEAD]' --discovery-boundary ./live",
			expected: []string{"other/unit"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir, runner := setupBoundaryStackRepo(t)

			appendBoundaryStackChange(t, runner, filepath.Join(tmpDir, tc.changed))

			if tc.deleteInTree != "" {
				require.NoError(t, os.RemoveAll(filepath.Join(tmpDir, filepath.FromSlash(tc.deleteInTree))))
			}

			stdout, stderr, err := runBoundaryStackCommand(
				t,
				filepath.Join(tmpDir, filepath.FromSlash(tc.workDir)),
				"find",
				tc.args,
			)

			if tc.errText != "" {
				require.ErrorContains(t, err, tc.errText)
				return
			}

			require.NoError(t, err, "stderr: %s", stderr)
			assert.ElementsMatch(t, tc.expected, outputLines(stdout))
		})
	}
}

// TestStackGenerateFlagNarrowsNonGraphFilters pins that the flag still narrows the working-tree stack walk for path filters.
func TestStackGenerateFlagNarrowsNonGraphFilters(t *testing.T) {
	t.Parallel()

	tmpDir, _ := setupBoundaryStackRepo(t)

	_, stderr, err := runBoundaryStackCommand(t, tmpDir, "stack generate", "--discovery-boundary ./live --filter './live/**'")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.DirExists(t, filepath.Join(tmpDir, filepath.FromSlash(boundaryStackRolesUnitDir)))
	assert.NoDirExists(t, filepath.Join(tmpDir, "catalog", "stacks", "account", ".terragrunt-stack"))
}

// setupBoundaryStackRepo creates a catalog beside the live tree on a branch cut from main.
func setupBoundaryStackRepo(t *testing.T) (string, *git.GitRunner) {
	t.Helper()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	runner := helpers.InitTestGitRunner(t, tmpDir)

	for path, contents := range map[string]string{
		".gitignore":                                   ".terragrunt-stack\n",
		boundaryStackCatalogFile:                       boundaryStackCatalogStack,
		boundaryStackCatalogUnit:                       "",
		"catalog/units/account/main.tf":                boundaryStackAccountModule,
		"catalog/units/roles/terragrunt.hcl":           boundaryStackCatalogRolesUnit,
		boundaryStackUnusedUnit:                        boundaryStackCatalogRolesUnit,
		boundaryStackOtherUnit:                         "",
		boundaryStackAppDir + "/terragrunt.hcl":        boundaryStackAppUnit,
		boundaryStackAppDir + "/main.tf":               boundaryStackAppModule,
		boundaryStackDBDir + "/terragrunt.hcl":         "",
		boundaryStackDBDir + "/main.tf":                boundaryStackDBModule,
		"other/unit/main.tf":                           boundaryStackStandaloneModule,
		"catalog/units/roles/main.tf":                  boundaryStackRolesModule,
		boundaryStackLiveStackFile:                     boundaryStackLiveStack,
		boundaryStackRolesFile:                         "[]\n",
		boundaryStackStandaloneDir + "/terragrunt.hcl": boundaryStackStandaloneUnit,
		boundaryStackStandaloneDir + "/main.tf":        boundaryStackStandaloneModule,
		boundaryStackStandaloneRead:                    "extra: true\n",
	} {
		writeExpansionFilterFile(t, filepath.Join(tmpDir, filepath.FromSlash(path)), contents)
	}

	commitExpansionFilterChanges(t, runner, "Add catalog and live stack")

	if branch, err := runner.Config(t.Context(), "init.defaultBranch"); err != nil ||
		branch != "main" {
		require.NoError(t, runner.Checkout(t.Context(), "main", true))
	}

	require.NoError(t, runner.Checkout(t.Context(), "change", true))

	return tmpDir, runner
}

// appendBoundaryStackChange appends a comment to path and commits it on the current branch.
func appendBoundaryStackChange(t *testing.T, runner *git.GitRunner, path string) {
	t.Helper()

	f, err := os.OpenFile(filepath.FromSlash(path), os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)

	_, err = f.WriteString("# changed\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	commitExpansionFilterChanges(t, runner, "Change "+filepath.Base(path))
}

// removeBoundaryStackPath deletes path and commits the removal on the current branch.
func removeBoundaryStackPath(t *testing.T, runner *git.GitRunner, path string) {
	t.Helper()

	require.NoError(t, os.Remove(filepath.FromSlash(path)))

	commitExpansionFilterChanges(t, runner, "Remove "+filepath.Base(path))
}

// runBoundaryStackCommand runs a terragrunt command against dir.
func runBoundaryStackCommand(t *testing.T, dir, command, args string) (string, string, error) {
	t.Helper()

	return helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt "+command+" --no-color --non-interactive --working-dir "+dir+" "+args,
	)
}

// outputLines returns the non-empty lines of stdout with forward slashes.
func outputLines(stdout string) []string {
	var lines []string

	for line := range strings.SplitSeq(stdout, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, filepath.ToSlash(line))
		}
	}

	return lines
}
