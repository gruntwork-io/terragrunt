package discovery_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expansionCatalogUnit is the source every expanded unit in these tests is generated from.
// Instances differ only by the terragrunt.values.hcl written alongside them, so their
// directory hashes differ.
const expansionCatalogUnit = `inputs = {
  role = values.role
}
`

// setupExpansionRepo creates a Git repository holding a unit catalog. The stacks
// documentation recommends gitignoring the generated tree, so these repositories do.
func setupExpansionRepo(t *testing.T) (string, *git.GitRunner) {
	t.Helper()

	tmpDir, runner := setupGitRepo(t)

	writeExpansionFile(t, filepath.Join(tmpDir, ".gitignore"), ".terragrunt-stack\n")
	writeExpansionFile(
		t,
		filepath.Join(tmpDir, "catalog", "units", "app", "terragrunt.hcl"),
		expansionCatalogUnit,
	)

	return tmpDir, runner
}

func writeExpansionFile(t *testing.T, path, contents string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}

// parseErrorPolicy selects how a discovery treats a configuration that fails to parse.
// find and list suppress those errors. run does not.
type parseErrorPolicy int

const (
	failOnParseErrors parseErrorPolicy = iota
	suppressParseErrors
)

// discoverExpansionChanges generates stacks into the worktrees for [HEAD~1...HEAD] and runs
// discovery over them. Any extra filters join the Git expression in the same union a repeated
// --filter would build.
func discoverExpansionChanges(
	t *testing.T,
	tmpDir string,
	cmd string,
	extraFilters filter.Filters,
	policy parseErrorPolicy,
) (component.Components, *worktrees.WorktreePair) {
	t.Helper()

	l := logger.CreateLogger()
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}

	w, err := worktrees.NewWorktrees(
		t.Context(),
		l,
		venvtest.NewOSWithEmptyEnv(),
		worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: gitExpressions},
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, w.Cleanup(context.WithoutCancel(t.Context()), l, vfs.NewOSFS()))
	})

	filters := make(filter.Filters, 0, len(gitExpressions)+len(extraFilters))
	for _, gitExpr := range gitExpressions {
		filters = append(filters, filter.NewFilter(gitExpr, gitExpr.String()))
	}

	filters = append(filters, extraFilters...)

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir
	opts.Filters = filters
	opts.Experiments = experiment.NewExperiments()
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.FilterFlag))
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.BlockIteration))

	require.NoError(
		t,
		generate.NewGenerator().GenerateStacks(t.Context(), l, venvtest.NewOSWithEmptyEnv(), opts, w),
	)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir, Cmd: cmd}).
		WithWorktrees(w).
		WithFilters(filters)

	if policy == suppressParseErrors {
		d = d.WithSuppressParseErrors()
	}

	components, err := d.Discover(t.Context(), l, venvtest.NewOSWithEmptyEnv(), opts)
	require.NoError(t, err)

	pair, ok := w.WorktreePairs["[HEAD~1...HEAD]"]
	require.True(t, ok)

	return components, pair
}

func requireComponent(
	t *testing.T,
	components component.Components,
	path string,
) component.Component {
	t.Helper()

	for _, c := range components {
		if c.Path() == path {
			return c
		}
	}

	require.Failf(t, "component not discovered", "%s not among %v", path, components.Paths())

	return nil
}

func assertDestroyIntent(t *testing.T, c component.Component, ref string) {
	t.Helper()

	dc := c.DiscoveryContext()
	require.NotNil(t, dc)
	assert.Equal(t, ref, dc.Ref)
	assert.Contains(t, dc.Args, "-destroy")
}

func assertApplyIntent(t *testing.T, c component.Component, ref string) {
	t.Helper()

	dc := c.DiscoveryContext()
	require.NotNil(t, dc)
	assert.Equal(t, ref, dc.Ref)
	assert.NotContains(t, dc.Args, "-destroy")
}

// forEachStack renders a stack file expanding one unit per element of the set.
func forEachStack(elements string) string {
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

// TestWorktreePhase_Integration_ExpansionForEachShrinks pins the orphan cleanup path the
// stacks documentation points users at. An instance dropped from a for_each is discovered in
// the from worktree and carries the run intent that destroys its state.
func TestWorktreePhase_Integration_ExpansionForEachShrinks(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupExpansionRepo(t)
	stackFile := filepath.Join(tmpDir, "live", "terragrunt.stack.hcl")

	writeExpansionFile(t, stackFile, forEachStack(`["web", "api"]`))
	commitChanges(t, runner, "Expand shard over web and api")

	writeExpansionFile(t, stackFile, forEachStack(`["web"]`))
	commitChanges(t, runner, "Drop the api shard")

	components, pair := discoverExpansionChanges(t, tmpDir, "apply", nil, failOnParseErrors)

	orphan := requireComponent(
		t,
		components,
		filepath.Join(pair.FromWorktree.Path, "live", ".terragrunt-stack", "shard", "api"),
	)
	assertDestroyIntent(t, orphan, "HEAD~1")

	assert.NotContains(
		t,
		components.Paths(),
		filepath.Join(pair.ToWorktree.Path, "live", ".terragrunt-stack", "shard", "web"),
		"the surviving instance is unchanged between refs, so it stays out of the run",
	)
}

// TestWorktreePhase_Integration_ExpansionForEachGrows pins a growing for_each. The new
// instance is discovered in the to worktree and runs the command as typed.
func TestWorktreePhase_Integration_ExpansionForEachGrows(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupExpansionRepo(t)
	stackFile := filepath.Join(tmpDir, "live", "terragrunt.stack.hcl")

	writeExpansionFile(t, stackFile, forEachStack(`["web"]`))
	commitChanges(t, runner, "Expand shard over web")

	writeExpansionFile(t, stackFile, forEachStack(`["web", "api"]`))
	commitChanges(t, runner, "Add the api shard")

	components, pair := discoverExpansionChanges(t, tmpDir, "apply", nil, failOnParseErrors)

	added := requireComponent(
		t,
		components,
		filepath.Join(pair.ToWorktree.Path, "live", ".terragrunt-stack", "shard", "api"),
	)
	assertApplyIntent(t, added, "HEAD")

	assert.NotContains(
		t,
		components.Paths(),
		filepath.Join(pair.FromWorktree.Path, "live", ".terragrunt-stack", "shard", "api"),
	)
}

// TestWorktreePhase_Integration_ExpansionCountShrinks pins how a count expansion behaves when
// its iteration set loses an element. Instances are addressed by index, so dropping anything
// but the last element shifts every later instance onto the values of its successor. The
// shifted directories still pair up and are reported as changed. Only the trailing index is
// left unpaired, as the orphan.
func TestWorktreePhase_Integration_ExpansionCountShrinks(t *testing.T) {
	t.Parallel()

	countStack := func(elements string) string {
		return `locals {
  shards = ` + elements + `
}

unit "shard" {
  expansion {
    count = length(local.shards)
  }

  source = "${get_repo_root()}/catalog/units/app"
  path   = "shard/${count.index}"

  values = {
    role = local.shards[count.index]
  }
}
`
	}

	testCases := []struct {
		name             string
		to               string
		expectedChanged  []string
		expectedOrphaned []string
	}{
		{
			name:             "trailing element dropped",
			to:               `["alpha", "bravo"]`,
			expectedChanged:  nil,
			expectedOrphaned: []string{"2"},
		},
		{
			name:             "middle element dropped",
			to:               `["alpha", "charlie"]`,
			expectedChanged:  []string{"1"},
			expectedOrphaned: []string{"2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir, runner := setupExpansionRepo(t)
			stackFile := filepath.Join(tmpDir, "live", "terragrunt.stack.hcl")

			writeExpansionFile(t, stackFile, countStack(`["alpha", "bravo", "charlie"]`))
			commitChanges(t, runner, "Expand shard over three elements")

			writeExpansionFile(t, stackFile, countStack(tc.to))
			commitChanges(t, runner, "Shrink the shard expansion")

			components, pair := discoverExpansionChanges(t, tmpDir, "apply", nil, failOnParseErrors)

			expected := make([]string, 0, len(tc.expectedChanged)+len(tc.expectedOrphaned)+1)
			expected = append(expected, filepath.Join(pair.ToWorktree.Path, "live"))

			for _, index := range tc.expectedChanged {
				changed := filepath.Join(
					pair.ToWorktree.Path, "live", ".terragrunt-stack", "shard", index,
				)
				assertApplyIntent(t, requireComponent(t, components, changed), "HEAD")
				expected = append(expected, changed)
			}

			for _, index := range tc.expectedOrphaned {
				orphan := filepath.Join(
					pair.FromWorktree.Path, "live", ".terragrunt-stack", "shard", index,
				)
				assertDestroyIntent(t, requireComponent(t, components, orphan), "HEAD~1")
				expected = append(expected, orphan)
			}

			assert.ElementsMatch(t, expected, components.Paths())
		})
	}
}

// TestWorktreePhase_Integration_ExpansionUnitDisabled pins how a Git filter reads
// enabled = false. The address is meant to survive the toggle, but the generated directory
// does not, so the unit turns up only on the from side and is scheduled for destruction.
func TestWorktreePhase_Integration_ExpansionUnitDisabled(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupExpansionRepo(t)
	stackFile := filepath.Join(tmpDir, "live", "terragrunt.stack.hcl")

	enabledStack := func(enabled string) string {
		return `unit "legacy" {
  enabled = ` + enabled + `

  source = "${get_repo_root()}/catalog/units/app"
  path   = "legacy"

  values = {
    role = "legacy"
  }
}
`
	}

	writeExpansionFile(t, stackFile, enabledStack("true"))
	commitChanges(t, runner, "Add the legacy unit")

	writeExpansionFile(t, stackFile, enabledStack("false"))
	commitChanges(t, runner, "Disable the legacy unit")

	components, pair := discoverExpansionChanges(t, tmpDir, "apply", nil, failOnParseErrors)

	disabled := requireComponent(
		t,
		components,
		filepath.Join(pair.FromWorktree.Path, "live", ".terragrunt-stack", "legacy"),
	)
	assertDestroyIntent(t, disabled, "HEAD~1")
}

// TestWorktreePhase_Integration_ExpansionStackDisabled pins that disabling a stack block
// takes the units nested underneath it along. The generated stack directory and the units it
// generated in turn all come back from the from side, each scheduled for destruction.
func TestWorktreePhase_Integration_ExpansionStackDisabled(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupExpansionRepo(t)

	writeExpansionFile(
		t,
		filepath.Join(tmpDir, "catalog", "stacks", "team", "terragrunt.stack.hcl"),
		`unit "member" {
  source = "${get_repo_root()}/catalog/units/app"
  path   = "member"

  values = {
    role = "member"
  }
}
`,
	)

	stackFile := filepath.Join(tmpDir, "live", "terragrunt.stack.hcl")

	enabledStack := func(enabled string) string {
		return `stack "team" {
  enabled = ` + enabled + `

  source = "${get_repo_root()}/catalog/stacks/team"
  path   = "team"
}
`
	}

	writeExpansionFile(t, stackFile, enabledStack("true"))
	commitChanges(t, runner, "Add the team stack")

	writeExpansionFile(t, stackFile, enabledStack("false"))
	commitChanges(t, runner, "Disable the team stack")

	components, pair := discoverExpansionChanges(t, tmpDir, "apply", nil, failOnParseErrors)

	generated := filepath.Join(pair.FromWorktree.Path, "live", ".terragrunt-stack", "team")
	assertDestroyIntent(t, requireComponent(t, components, generated), "HEAD~1")
	assertDestroyIntent(
		t,
		requireComponent(t, components, filepath.Join(generated, ".terragrunt-stack", "member")),
		"HEAD~1",
	)
}

// TestWorktreePhase_Integration_ExpansionReadingAffectedOrphan pins the second route to an
// orphan. The stack file is untouched, but the file it reads to build its iteration set
// changed, which is enough for reading-affected detection to walk the stack.
func TestWorktreePhase_Integration_ExpansionReadingAffectedOrphan(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupExpansionRepo(t)
	shardsFile := filepath.Join(tmpDir, "shards.hcl")

	writeExpansionFile(
		t,
		filepath.Join(tmpDir, "live", "terragrunt.stack.hcl"),
		`locals {
  shards = read_terragrunt_config("${get_repo_root()}/shards.hcl").locals.shards
}

unit "shard" {
  expansion {
    for_each = toset(local.shards)
  }

  source = "${get_repo_root()}/catalog/units/app"
  path   = "shard/${each.key}"

  values = {
    role = each.key
  }
}
`,
	)

	writeExpansionFile(t, shardsFile, `locals {
  shards = ["web", "api"]
}
`)
	commitChanges(t, runner, "Expand shard over the shards file")

	writeExpansionFile(t, shardsFile, `locals {
  shards = ["web"]
}
`)
	commitChanges(t, runner, "Drop the api shard from the shards file")

	components, pair := discoverExpansionChanges(t, tmpDir, "apply", nil, failOnParseErrors)

	orphan := requireComponent(
		t,
		components,
		filepath.Join(pair.FromWorktree.Path, "live", ".terragrunt-stack", "shard", "api"),
	)
	assertDestroyIntent(t, orphan, "HEAD~1")
}

// TestWorktreePhase_Integration_ExpansionParseErrorsSuppressedInChangedStack covers a unit
// inside a changed stack whose configuration does not parse. Walking the stack runs a
// discovery per side, and those have to tolerate parse errors exactly as the parent does. A
// stricter sub-discovery fails the whole worktree phase, and a command that suppresses parse
// errors swallows that failure, so the orphan never reaches the user.
func TestWorktreePhase_Integration_ExpansionParseErrorsSuppressedInChangedStack(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupExpansionRepo(t)

	writeExpansionFile(
		t,
		filepath.Join(tmpDir, "catalog", "units", "malformed", "terragrunt.hcl"),
		"inputs = {\n",
	)

	stackFile := filepath.Join(tmpDir, "live", "terragrunt.stack.hcl")
	stackWithMalformedUnit := func(elements string) string {
		return forEachStack(elements) + `
unit "malformed" {
  source = "${get_repo_root()}/catalog/units/malformed"
  path   = "malformed"
}
`
	}

	writeExpansionFile(t, stackFile, stackWithMalformedUnit(`["web", "api"]`))
	commitChanges(t, runner, "Expand shard over web and api")

	writeExpansionFile(t, stackFile, stackWithMalformedUnit(`["web"]`))
	commitChanges(t, runner, "Drop the api shard")

	// A negated attribute filter subtracts from the Git expression's union rather than
	// replacing it, and matching on source forces discovery to parse every candidate. That is
	// what brings the malformed unit into the walk.
	sourceExpr, err := filter.NewAttributeExpression(filter.AttributeSource, "**/never-matches")
	require.NoError(t, err)

	negated := filter.NewPrefixExpression("!", sourceExpr)
	extraFilters := filter.Filters{filter.NewFilter(negated, negated.String())}

	components, pair := discoverExpansionChanges(t, tmpDir, "", extraFilters, suppressParseErrors)

	orphan := requireComponent(
		t,
		components,
		filepath.Join(pair.FromWorktree.Path, "live", ".terragrunt-stack", "shard", "api"),
	)
	assert.Equal(t, "HEAD~1", orphan.DiscoveryContext().Ref)
}
