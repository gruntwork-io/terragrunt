package discovery_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// TestGraphPhase_UpstreamCandidateContextOwnership pins which component ends up
// with which discovery context when the upstream dependent walk canonicalises a
// candidate.
//
// The walk assigns the target's context to the throwaway component it minted,
// then canonicalises. Canonicalising keeps whichever component the shared set
// already holds, so the assignment reaches the shared set only when the walk's
// own component is the one adopted. A unit the filesystem walk already found
// therefore keeps the origin it was discovered under, while a unit above the
// working directory, which only the upstream walk reaches, carries the graph
// origin copied from the target.
//
// Assigning to the component canonicalisation returns instead would rewrite the
// first case, and doing it after canonicalisation would publish a component
// before its working directory is set.
func TestGraphPhase_UpstreamCandidateContextOwnership(t *testing.T) {
	t.Parallel()

	workingDir := repoPath("svc")

	v := memRepoRootVenv(t, corpusRepoRoot)

	writeFixture(t, v, map[string]string{
		repoPath("svc", "vpc", "terragrunt.hcl"):   "",
		repoPath("svc", "near", "terragrunt.hcl"):  unitHCL("../vpc"),
		repoPath("svc", "noise", "terragrunt.hcl"): "",
		repoPath("above", "terragrunt.hcl"):        unitHCL("../svc/vpc"),
	})

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = workingDir
	opts.RootWorkingDir = workingDir

	l := logger.CreateLogger()

	// The second query keeps a unit the walk processes as a candidate but never
	// selects as a dependent in the result set, so its context stays observable.
	filters, err := filter.ParseFilterQueries(l, []string{
		"...{" + repoPath("svc", "vpc") + "}",
		"./noise",
	})
	require.NoError(t, err)

	components, err := discovery.NewDiscovery(workingDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: workingDir}).
		WithGitRoot(corpusRepoRoot).
		WithFilters(filters).
		Discover(t.Context(), l, v, opts)
	require.NoError(t, err)

	origins := make(map[string]component.Origin, len(components))
	for _, c := range components {
		origins[c.Path()] = c.Origin()
	}

	assert.Equal(t, map[string]component.Origin{
		repoPath("svc", "vpc"):   component.OriginPathDiscovery,
		repoPath("svc", "near"):  component.OriginPathDiscovery,
		repoPath("svc", "noise"): component.OriginPathDiscovery,
		repoPath("above"):        component.OriginGraphDiscovery,
	}, origins)
}

// TestGraphPhase_UpstreamCandidateBailKeepsExternalMark pins that a candidate
// the upstream dependent walk abandons leaves no trace in the shared component
// set.
//
// The walk parses whatever the set already holds for a candidate's path, so it
// looks the component up instead of publishing its own. Publishing first would
// leave an abandoned candidate in the set carrying the target's working
// directory, and a non-empty working directory is what tells the dependency
// resolver that a component has already been classified. A dependency later
// reaching that same path would then skip the external mark, which backs the
// external filter attribute and the runner's queue construction.
//
// The fixture puts the abandoned unit one directory above the working directory
// and the unit depending on it one directory above that, so the walk reaches the
// abandoned unit in an earlier frame than the dependent that would otherwise
// classify it.
func TestGraphPhase_UpstreamCandidateBailKeepsExternalMark(t *testing.T) {
	t.Parallel()

	// The dependency block itself parses, so the walk gets past the parse and
	// abandons the candidate on the unresolvable config_path instead.
	const unresolvableDependencyHCL = "dependency \"oops\" {\n  config_path =\n}\n"

	workingDir := repoPath("env", "staging")

	v := memRepoRootVenv(t, corpusRepoRoot)

	writeFixture(t, v, map[string]string{
		repoPath("env", "staging", "vpc", "terragrunt.hcl"): "",
		repoPath("env", "abandoned", "terragrunt.hcl"):      unresolvableDependencyHCL,
		repoPath("consumer", "terragrunt.hcl"): unitHCL(
			"../env/abandoned",
			"../env/staging/vpc",
		),
	})

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = workingDir
	opts.RootWorkingDir = workingDir

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, []string{
		"...{" + repoPath("env", "staging", "vpc") + "}",
	})
	require.NoError(t, err)

	components, err := discovery.NewDiscovery(workingDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: workingDir}).
		WithGitRoot(corpusRepoRoot).
		WithFilters(filters).
		WithSuppressParseErrors().
		Discover(t.Context(), l, v, opts)
	require.NoError(t, err)

	paths := components.Paths()
	slices.Sort(paths)

	require.Equal(t, []string{repoPath("consumer"), repoPath("env", "staging", "vpc")}, paths)

	var consumer component.Component

	for _, c := range components {
		if c.Path() == repoPath("consumer") {
			consumer = c
		}
	}

	require.NotNil(t, consumer)

	external := make(map[string]bool, len(consumer.Dependencies()))
	for _, dep := range consumer.Dependencies() {
		external[dep.Path()] = dep.External()
	}

	assert.Equal(t, map[string]bool{
		repoPath("env", "abandoned"):      true,
		repoPath("env", "staging", "vpc"): false,
	}, external)
}
