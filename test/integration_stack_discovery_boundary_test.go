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

const (
	boundaryStackStandaloneDir  = "live/standalone"
	boundaryStackStandaloneRead = boundaryStackStandaloneDir + "/extra.yml"
	boundaryStackRolesFile      = "live/accounts/my-account/roles.yml"
	boundaryStackLiveStackFile  = "live/accounts/my-account/terragrunt.stack.hcl"
	boundaryStackCatalogFile    = "catalog/stacks/account/terragrunt.stack.hcl"
	boundaryStackCatalogUnit    = "catalog/units/account/terragrunt.hcl"
	boundaryStackUnusedUnit     = "catalog/units/unused/terragrunt.hcl"
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
			name:     "changed sibling unit under a parent boundary",
			changed:  boundaryStackStandaloneDir + "/terragrunt.hcl",
			workDir:  "live/accounts",
			args:     "--filter '(..)...[main...HEAD]'",
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
