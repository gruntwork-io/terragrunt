package redesign

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// tempDirFormat mirrors the legacy catalog service's temp-path scheme so
// repeated runs reuse the same clone on disk.
const tempDirFormat = "catalog-%s"

// LoadURL clones repoURL via module.NewRepo, walks it with DiscoverComponents,
// resolves the latest release tag once, and emits a *ComponentEntry for each
// discovered component on componentCh.
//
// This is the redesign-owned replacement for catalog.CatalogService.LoadStreamingURL.
// It is intentionally duplicated to keep the redesign path isolated from the
// legacy catalog service while still reusing module.NewRepo for generic git/
// clone plumbing.
func LoadURL(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	repoURL string,
	componentCh chan<- *ComponentEntry,
) error {
	if repoURL == "" {
		l.Warnf("Empty repository URL encountered, skipping.")
		return nil
	}

	walkWithSymlinks := opts.Experiments.Evaluate(experiment.Symlinks)
	allowCAS := opts.Experiments.Evaluate(experiment.CAS)
	slowReporting := opts.Experiments.Evaluate(experiment.SlowTaskReporting)

	encodedRepoURL := util.EncodeBase64Sha1(repoURL)
	tempPath := filepath.Join(os.TempDir(), fmt.Sprintf(tempDirFormat, encodedRepoURL))

	l.Debugf("Processing repository %s in temporary path %s", repoURL, tempPath)

	repo, err := module.NewRepo(ctx, l, v.FS, &module.RepoOpts{
		CloneURL:         repoURL,
		Path:             tempPath,
		WalkWithSymlinks: walkWithSymlinks,
		AllowCAS:         allowCAS,
		SlowReporting:    slowReporting,
		RootWorkingDir:   opts.RootWorkingDir,
	})
	if err != nil {
		return errors.Errorf("failed to initialize repository %s: %w", repoURL, err)
	}

	discovery := NewComponentDiscovery().WithFS(v.FS).WithExtraIgnoreFile(opts.CatalogIgnoreFile)
	if walkWithSymlinks {
		discovery = discovery.WithWalkWithSymlinks()
	}

	components, err := discovery.Discover(repo)
	if err != nil {
		return errors.Errorf("failed to discover components in repository %s: %w", repoURL, err)
	}

	if len(components) == 0 {
		l.Debugf("No components found in repository %q", repoURL)
		return nil
	}

	l.Debugf("Found %d component(s) in repository %q", len(components), repoURL)

	// Resolve the latest release tag once per repo. All components from the
	// same repo share the Repo, so the tag is set for everyone.
	repo.ResolveLatestTag(ctx, l, v.Exec)

	source := ExtractRepoURL(repo.SourceURL())

	for _, c := range components {
		entry := NewComponentEntry(c).WithSource(source)

		if repo.LatestTag != "" {
			entry = entry.WithVersion(repo.LatestTag)
		}

		select {
		case componentCh <- entry:
		case <-ctx.Done():
			return nil
		}
	}

	return nil
}
