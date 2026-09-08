package test_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expansionFilterCatalogUnit is the source every expanded unit in these tests is generated
// from. Instances differ only by the terragrunt.values.hcl written alongside them.
const expansionFilterCatalogUnit = `inputs = {
  role = values.role
}
`

// setupExpansionFilterRepo creates a Git repository holding a unit catalog. The stacks
// documentation recommends gitignoring the generated tree, so these repositories do.
func setupExpansionFilterRepo(t *testing.T) (string, *git.GitRunner) {
	t.Helper()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	runner := helpers.InitTestGitRunner(t, tmpDir)

	writeExpansionFilterFile(t, filepath.Join(tmpDir, ".gitignore"), ".terragrunt-stack\n")
	writeExpansionFilterFile(
		t,
		filepath.Join(tmpDir, "catalog", "units", "app", "terragrunt.hcl"),
		expansionFilterCatalogUnit,
	)

	return tmpDir, runner
}

func writeExpansionFilterFile(t *testing.T, path, contents string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}

func commitExpansionFilterChanges(t *testing.T, runner *git.GitRunner, message string) {
	t.Helper()

	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), message))
}

// expansionFilterStack renders a stack file expanding one unit per element of the set.
func expansionFilterStack(elements string) string {
	return `unit "shard" {
  expansion {
    for_each = toset(` + elements + `)
  }

  source = "${get_repo_root()}/catalog/units/app"
  path   = "shard/${each.key}"

  values = {
    role = each.key
  }
}
`
}

// runExpansionFilterFind runs `terragrunt find` with the block-iteration experiment enabled
// and returns the selected paths.
func runExpansionFilterFind(t *testing.T, tmpDir, args string) []string {
	t.Helper()

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt find --no-color --experiment block-iteration --working-dir "+tmpDir+" "+args,
	)
	require.NoError(t, err, "stderr: %s", stderr)

	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}

// generateExpansionFilterStacks runs `terragrunt stack generate` over tmpDir and returns the
// generated unit directories, relative to tmpDir.
func generateExpansionFilterStacks(t *testing.T, tmpDir, args string) []string {
	t.Helper()

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --no-color --experiment block-iteration --working-dir "+tmpDir+" "+args,
	)
	require.NoError(t, err, "stderr: %s", stderr)

	var generated []string

	require.NoError(t, filepath.WalkDir(tmpDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}

			return nil
		}

		if entry.Name() != "terragrunt.hcl" {
			return nil
		}

		rel, relErr := filepath.Rel(tmpDir, filepath.Dir(path))
		if relErr != nil {
			return relErr
		}

		if strings.Contains(rel, ".terragrunt-stack") {
			generated = append(generated, filepath.ToSlash(rel))
		}

		return nil
	}))

	return generated
}

// TestStackExpansionGitFilterSelectsRemovedInstance pins the orphan cleanup workflow end to
// end. An instance dropped from a for_each is selected alongside its stack, whether the diff
// is spelled out as a Git expression or reached through --filter-affected.
func TestStackExpansionGitFilterSelectsRemovedInstance(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupExpansionFilterRepo(t)
	stackFile := filepath.Join(tmpDir, "live", "terragrunt.stack.hcl")

	writeExpansionFilterFile(t, stackFile, expansionFilterStack(`["web", "api"]`))
	commitExpansionFilterChanges(t, runner, "Expand shard over web and api")

	// --filter-affected compares against main, so the initial state has to live there before
	// the change lands on a branch of its own.
	if branch, err := runner.Config(t.Context(), "init.defaultBranch"); err != nil ||
		branch != "main" {
		require.NoError(t, runner.Checkout(t.Context(), "main", true))
	}

	require.NoError(t, runner.Checkout(t.Context(), "shrink-expansion", true))

	writeExpansionFilterFile(t, stackFile, expansionFilterStack(`["web"]`))
	commitExpansionFilterChanges(t, runner, "Drop the api shard")

	expected := []string{"live", "live/.terragrunt-stack/shard/api"}

	for _, tc := range []struct {
		name string
		args string
	}{
		{name: "git expression", args: "--filter '[main...HEAD]'"},
		{name: "filter-affected", args: "--filter-affected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.ElementsMatch(t, expected, runExpansionFilterFind(t, tmpDir, tc.args))
		})
	}
}

// TestStackExpansionGitFilterNeedsTheStackFileInTheDiff pins the limit of orphan discovery.
// An iteration set driven from outside the repository shrinks without touching the stack file
// or anything it reads. The diff then gives Terragrunt nothing to compare, and the instance
// that disappeared never shows up, even though its directory is still on disk.
func TestStackExpansionGitFilterNeedsTheStackFileInTheDiff(t *testing.T) {
	// t.Setenv rules out t.Parallel: the iteration set is driven from the environment.
	tmpDir, runner := setupExpansionFilterRepo(t)

	writeExpansionFilterFile(
		t,
		filepath.Join(tmpDir, "live", "terragrunt.stack.hcl"),
		expansionFilterStack(`compact(split(",", get_env("TEST_EXPANSION_SHARDS", "web,api")))`),
	)
	commitExpansionFilterChanges(t, runner, "Expand shard over the environment")

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --experiment block-iteration --working-dir "+tmpDir,
	)

	writeExpansionFilterFile(
		t,
		filepath.Join(tmpDir, "catalog", "units", "app", "terragrunt.hcl"),
		expansionFilterCatalogUnit+"\n# Touched\n",
	)
	commitExpansionFilterChanges(t, runner, "Touch the catalog unit")

	t.Setenv("TEST_EXPANSION_SHARDS", "web")

	require.DirExists(t, filepath.Join(tmpDir, "live", ".terragrunt-stack", "shard", "api"))

	assert.ElementsMatch(
		t,
		[]string{"catalog/units/app"},
		runExpansionFilterFind(t, tmpDir, "--filter '[HEAD~1...HEAD]'"),
	)
}

// TestStackExpansionFilterRestrictedToStacksNarrowsGeneration pins which filters narrow the
// set of stacks Terragrunt generates. Generation stays permissive until a filter names stacks.
// A filter aimed at a generated unit path leaves every stack generated, and only a
// stack-restricted filter cuts the set down.
func TestStackExpansionFilterRestrictedToStacksNarrowsGeneration(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		args      string
		generated []string
	}{
		{
			name: "no filter",
			generated: []string{
				"live/.terragrunt-stack/shard/api",
				"live/.terragrunt-stack/shard/web",
				"other/.terragrunt-stack/shard/solo",
			},
		},
		{
			name: "path filter naming a generated unit",
			args: "--filter './live/.terragrunt-stack/shard/web'",
			generated: []string{
				"live/.terragrunt-stack/shard/api",
				"live/.terragrunt-stack/shard/web",
				"other/.terragrunt-stack/shard/solo",
			},
		},
		{
			name: "path filter restricted to stacks",
			args: "--filter './live | type=stack'",
			generated: []string{
				"live/.terragrunt-stack/shard/api",
				"live/.terragrunt-stack/shard/web",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir, runner := setupExpansionFilterRepo(t)

			writeExpansionFilterFile(
				t,
				filepath.Join(tmpDir, "live", "terragrunt.stack.hcl"),
				expansionFilterStack(`["web", "api"]`),
			)
			writeExpansionFilterFile(
				t,
				filepath.Join(tmpDir, "other", "terragrunt.stack.hcl"),
				expansionFilterStack(`["solo"]`),
			)
			commitExpansionFilterChanges(t, runner, "Add two expanding stacks")

			assert.ElementsMatch(
				t,
				tc.generated,
				generateExpansionFilterStacks(t, tmpDir, tc.args),
			)
		})
	}
}

// TestStackExpansionFilterSelectsGeneratedInstancePath pins how a single expanded instance is
// selected. The keyed address Terragrunt logs is a display string, and the bracket that opens
// it is the Git expression delimiter in filter syntax, so it cannot be written as a filter
// term at all. Users select an instance by the directory its path attribute produced.
func TestStackExpansionFilterSelectsGeneratedInstancePath(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupExpansionFilterRepo(t)

	writeExpansionFilterFile(
		t,
		filepath.Join(tmpDir, "live", "terragrunt.stack.hcl"),
		expansionFilterStack(`["web", "api"]`),
	)
	commitExpansionFilterChanges(t, runner, "Expand shard over web and api")

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --experiment block-iteration --working-dir "+tmpDir,
	)

	assert.ElementsMatch(
		t,
		[]string{"live/.terragrunt-stack/shard/web"},
		runExpansionFilterFind(t, tmpDir, "--filter './live/.terragrunt-stack/shard/web'"),
	)

	assert.ElementsMatch(
		t,
		[]string{"live/.terragrunt-stack/shard/web"},
		runExpansionFilterFind(t, tmpDir, "--filter 'name=web'"),
	)

	// The CLI renders a filter parse failure as a diagnostic rather than passing the typed
	// error through, so the parser is asked directly for what the run rejects.
	_, err := filter.Parse(`shard["web"]`)

	var parseErr filter.ParseError
	require.ErrorAs(t, err, &parseErr)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt find --no-color --experiment block-iteration --working-dir "+tmpDir+
			` --filter 'shard["web"]'`,
	)
	require.Error(t, err)
}

// TestStackExpansionSelectionIgnoresStaleGeneratedDirectory pins how a Git-filtered run
// treats a generated directory the current stack file no longer produces. Regeneration leaves
// it in place, so the working directory keeps offering a unit no ref generates. The worktrees
// are clean checkouts and decide the selection, so once the shrink falls outside the diff the
// leftover directory contributes nothing.
func TestStackExpansionSelectionIgnoresStaleGeneratedDirectory(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupExpansionFilterRepo(t)
	stackFile := filepath.Join(tmpDir, "live", "terragrunt.stack.hcl")

	writeExpansionFilterFile(t, stackFile, expansionFilterStack(`["web", "api"]`))
	commitExpansionFilterChanges(t, runner, "Expand shard over web and api")

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --experiment block-iteration --working-dir "+tmpDir,
	)

	writeExpansionFilterFile(t, stackFile, expansionFilterStack(`["web"]`))
	commitExpansionFilterChanges(t, runner, "Drop the api shard")

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --experiment block-iteration --working-dir "+tmpDir,
	)

	assert.DirExists(
		t,
		filepath.Join(tmpDir, "live", ".terragrunt-stack", "shard", "api"),
		"regeneration leaves the dropped instance's directory behind",
	)

	assert.ElementsMatch(
		t,
		[]string{"live", "live/.terragrunt-stack/shard/api"},
		runExpansionFilterFind(t, tmpDir, "--filter '[HEAD~1...HEAD]'"),
	)

	writeExpansionFilterFile(
		t,
		filepath.Join(tmpDir, "catalog", "units", "app", "terragrunt.hcl"),
		expansionFilterCatalogUnit+"\n# Touched\n",
	)
	commitExpansionFilterChanges(t, runner, "Touch the catalog unit")

	assert.ElementsMatch(
		t,
		[]string{"catalog/units/app"},
		runExpansionFilterFind(t, tmpDir, "--filter '[HEAD~1...HEAD]'"),
	)
}

// TestStackExpansionGitFilterRejectsDestroy pins the command a user has to reach for when an
// expansion shrinks. Terragrunt schedules the destroy itself, off an apply against the older
// ref, and rejects a destroy the user asks for outright.
func TestStackExpansionGitFilterRejectsDestroy(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupExpansionFilterRepo(t)
	stackFile := filepath.Join(tmpDir, "live", "terragrunt.stack.hcl")

	writeExpansionFilterFile(t, stackFile, expansionFilterStack(`["web", "api"]`))
	commitExpansionFilterChanges(t, runner, "Expand shard over web and api")

	writeExpansionFilterFile(t, stackFile, expansionFilterStack(`["web"]`))
	commitExpansionFilterChanges(t, runner, "Drop the api shard")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all destroy --experiment block-iteration --non-interactive"+
			" --working-dir "+tmpDir+" --filter '[HEAD~1...HEAD]'",
	)

	var commandErr discovery.GitFilterCommandError
	require.ErrorAs(t, err, &commandErr)
	assert.Equal(t, "destroy", commandErr.Cmd)
}

// TestStackExpansionGitFilterNarrowedBySelectingFilter pins what happens when a Git filter is
// paired with a filter that selects. Filters union, and a generated unit the Git filter
// surfaces has to survive every selecting filter in that union. Narrowing a cleanup run to one
// stack therefore drops the orphans it was meant to cover, and naming a generated unit path
// drops everything, because that path lives in the worktrees rather than in the working
// directory the filter resolves against.
func TestStackExpansionGitFilterNarrowedBySelectingFilter(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupExpansionFilterRepo(t)
	liveStack := filepath.Join(tmpDir, "live", "terragrunt.stack.hcl")
	otherStack := filepath.Join(tmpDir, "other", "terragrunt.stack.hcl")

	writeExpansionFilterFile(t, liveStack, expansionFilterStack(`["web", "api"]`))
	writeExpansionFilterFile(t, otherStack, expansionFilterStack(`["red", "blue"]`))
	commitExpansionFilterChanges(t, runner, "Expand two stacks")

	writeExpansionFilterFile(t, liveStack, expansionFilterStack(`["web"]`))
	writeExpansionFilterFile(t, otherStack, expansionFilterStack(`["red"]`))
	commitExpansionFilterChanges(t, runner, "Shrink both expansions")

	assert.ElementsMatch(
		t,
		[]string{
			"live",
			"other",
			"live/.terragrunt-stack/shard/api",
			"other/.terragrunt-stack/shard/blue",
		},
		runExpansionFilterFind(t, tmpDir, "--filter '[HEAD~1...HEAD]'"),
	)

	restricted := runExpansionFilterFind(
		t,
		tmpDir,
		"--filter '[HEAD~1...HEAD]' --filter './live | type=stack'",
	)
	assert.Contains(t, restricted, "live")
	assert.NotContains(t, restricted, "other")
	assert.NotContains(t, restricted, "live/.terragrunt-stack/shard/api")

	assert.Empty(t, runExpansionFilterFind(
		t,
		tmpDir,
		"--filter '[HEAD~1...HEAD]' --filter './live/.terragrunt-stack/shard/api'",
	))
}
