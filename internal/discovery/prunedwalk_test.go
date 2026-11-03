package discovery_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// TestPrunedWalkListsLeavesConcurrentlyWithRacing runs a pruned walk whose
// wildcard matches multiple component directories, so their listings run on
// multiple goroutines at once.
func TestPrunedWalkListsLeavesConcurrentlyWithRacing(t *testing.T) {
	t.Parallel()

	files := make(map[string]string, 2)
	for i := range 2 {
		files[repoPath("apps", fmt.Sprintf("u%02d", i), "terragrunt.hcl")] = ""
	}

	v := memRepoRootVenv(t, corpusRepoRoot)
	writeFixture(t, v, files)

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, []string{"./apps/*"})
	require.NoError(t, err)

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = corpusRepoRoot

	_, err = discovery.NewDiscovery(corpusRepoRoot).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: corpusRepoRoot}).
		WithFilters(filters).
		Discover(t.Context(), l, v, opts)
	require.NoError(t, err)
}

// TestPrunedWalkStopsOnCancellation pins that a pruned walk honors a cancelled
// context.
func TestPrunedWalkStopsOnCancellation(t *testing.T) {
	t.Parallel()

	v := memRepoRootVenv(t, corpusRepoRoot)
	writeFixture(t, v, map[string]string{
		repoPath("apps", "a", "terragrunt.hcl"): "",
	})

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = corpusRepoRoot

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, []string{"./apps/**"})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = discovery.NewDiscovery(corpusRepoRoot).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: corpusRepoRoot}).
		WithFilters(filters).
		Discover(ctx, l, v, opts)
	require.ErrorIs(t, err, context.Canceled)
}
