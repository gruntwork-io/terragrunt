package tui

import (
	"context"
	"fmt"
	"os"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const tempDirPattern = "catalog-*"

// CreateCatalogTempPath creates a fresh clone root under the resolved temp dir.
// Resolving os.TempDir keeps filepath.Rel results inside the clone on systems
// where the temp dir itself is reported through a symlink.
func CreateCatalogTempPath(fsys vfs.FS) (string, error) {
	return vfs.MkdirTemp(fsys, util.ResolvePath(os.TempDir()), tempDirPattern)
}

// LoadURL clones repoURL via module.NewRepo, walks it with a
// [ComponentDiscovery], resolves the latest release tag once, and emits a
// *ComponentEntry for each discovered component on componentCh. Each load uses
// a fresh temp directory and module.NewRepo for the generic git/clone plumbing.
func LoadURL(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	opts *options.TerragruntOptions,
	tempDirs *TempDirTracker,
	repoURL string,
	componentCh chan<- *ComponentEntry,
) error {
	if repoURL == "" {
		l.Warnf("Empty repository URL encountered, skipping.")
		return nil
	}

	walkWithSymlinks := opts.Experiments.Evaluate(experiment.Symlinks)
	allowCAS := !opts.NoCAS
	slowReporting := opts.Experiments.Evaluate(experiment.SlowTaskReporting)

	tempPath, err := CreateCatalogTempPath(v.FS)
	if err != nil {
		return fmt.Errorf("failed to create catalog temporary directory for %s: %w", repoURL, err)
	}

	tempDirs.Track(tempPath)

	keepDir := false

	defer func() {
		if keepDir {
			return
		}

		if err := v.FS.RemoveAll(tempPath); err != nil {
			l.Debugf("Failed to remove unused catalog temporary directory %q: %v", tempPath, err)
		}
	}()

	l.Debugf("Processing repository %s in temporary path %s", repoURL, tempPath)

	repo, err := module.NewRepo(ctx, l, v.FS, &module.RepoOpts{
		CloneURL:         repoURL,
		Path:             tempPath,
		WalkWithSymlinks: walkWithSymlinks,
		AllowCAS:         allowCAS,
		CASCloneDepth:    opts.CASCloneDepth,
		SlowReporting:    slowReporting,
		RootWorkingDir:   opts.RootWorkingDir,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize repository %s: %w", repoURL, err)
	}

	discovery := NewComponentDiscovery().WithFS(v.FS).WithExtraIgnoreFile(opts.CatalogIgnoreFile)
	if walkWithSymlinks {
		discovery = discovery.WithWalkWithSymlinks()
	}

	components, err := discovery.Discover(repo)
	if err != nil {
		return fmt.Errorf("failed to discover components in repository %s: %w", repoURL, err)
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

	keepDir = true

	return nil
}
