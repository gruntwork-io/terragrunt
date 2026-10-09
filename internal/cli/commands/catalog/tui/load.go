package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/redact"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// CreateCatalogTempPath creates a fresh clone root under the resolved temp dir.
// Resolving the temp dir keeps filepath.Rel results inside the clone on systems
// where the temp dir itself is reported through a symlink.
func CreateCatalogTempPath(v *venv.Venv, repoURL string) (string, error) {
	v.RequireFS()
	v.RequireTempDir()

	prefix := "catalog-" + util.EncodeBase64Sha1(repoURL) + "-"

	return vfs.MkdirTemp(v.FS, vfs.ResolveForCompare(v.FS, v.Platform.TempDir()), prefix)
}

// DisplayURL returns repoURL as it is shown to the user: a URL without the
// credentials it carries, and a local path as it is written.
func DisplayURL(repoURL string) string {
	if !strings.Contains(repoURL, "://") {
		return repoURL
	}

	return redact.NewURL(repoURL).String()
}

// LoadURL clones repoURL via module.NewRepo, walks it with a
// [ComponentDiscovery], resolves the latest release tag once, and emits a
// *ComponentEntry for each discovered component on componentCh. Each load uses
// a fresh temp directory and module.NewRepo for the generic git/clone plumbing.
func LoadURL(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
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

	shownURL := DisplayURL(repoURL)

	tempPath, err := CreateCatalogTempPath(v, repoURL)
	if err != nil {
		return fmt.Errorf("failed to create catalog temporary directory for %s: %w", shownURL, err)
	}

	keepDir := false

	preserveTempDir := func() {
		if keepDir {
			return
		}

		keepDir = true

		tempDirs.Track(tempPath)
	}

	defer func() {
		if keepDir {
			return
		}

		if err := v.FS.RemoveAll(tempPath); err != nil {
			l.Warnf("Failed to remove catalog temporary directory %q: %v", tempPath, err)
		}
	}()

	l.Debugf("Processing repository %s in temporary path %s", shownURL, tempPath)

	repo, err := module.NewRepo(ctx, l, v, &module.RepoOpts{
		CloneURL:         repoURL,
		Path:             tempPath,
		WalkWithSymlinks: walkWithSymlinks,
		AllowCAS:         allowCAS,
		CASCloneDepth:    opts.CASCloneDepth,
		CASProbeTTL:      opts.CASProbeTTL,
		CASOffline:       opts.CASOffline,
		CASRefresh:       opts.CASRefresh,
		CASProbeCache:    opts.Experiments.Evaluate(experiment.OfflineCAS),
		RootWorkingDir:   opts.RootWorkingDir,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize repository %s: %w", shownURL, err)
	}

	discovery := NewComponentDiscovery().WithExtraIgnoreFile(opts.CatalogIgnoreFile)
	if walkWithSymlinks {
		discovery = discovery.WithWalkWithSymlinks()
	}

	components, err := discovery.Discover(v.FS, repo)
	if err != nil {
		return fmt.Errorf("failed to discover components in repository %s: %w", shownURL, err)
	}

	if len(components) == 0 {
		l.Debugf("No components found in repository %q", shownURL)
		return nil
	}

	l.Debugf("Found %d component(s) in repository %q", len(components), shownURL)

	// Resolve the latest release tag once per repo. All components from the
	// same repo share the Repo, so the tag is set for everyone.
	repo.ResolveLatestTag(ctx, l, v)

	source := ExtractRepoURL(repo.SourceURL())

	for _, c := range components {
		entry := NewComponentEntry(c).WithSource(source)

		if repo.LatestTag != "" {
			entry = entry.WithVersion(repo.LatestTag)
		}

		select {
		case componentCh <- entry:
			preserveTempDir()
		case <-ctx.Done():
			return nil
		}
	}

	return nil
}
