// Package catalog provides the core functionality for the Terragrunt catalog command.
// It handles the logic for fetching and processing module information from remote repositories.
//
// This logic is intentionally isolated from the CLI package, as that package is focused on
// spinning up the Terminal User Interface (TUI), and forwarding user input to the catalog service.
//
// This should result in an implementation that is easier to test, and more maintainable.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// NewRepoFunc defines the signature for a function that creates a new repository.
// This allows for mocking in tests.
type NewRepoFunc func(ctx context.Context, l log.Logger, fsys vfs.FS, opts *module.RepoOpts) (*module.Repo, error)

// ModuleFunc is called for each module discovered during streaming load.
type ModuleFunc func(mod *module.Module)

const (
	// tempDirFormat is used to create unique temporary directory names for catalog repositories.
	// It uses a hexadecimal representation of a SHA1 hash of the repo URL.
	tempDirFormat = "catalog-%s" // Changed from catalog%x to catalog-%s for clarity with Sprintf.
)

// CatalogService defines the interface for the catalog service.
// It's responsible for fetching and processing module information.
type CatalogService interface {
	// Load retrieves all modules from the configured repositories.
	// It stores discovered modules internally.
	Load(ctx context.Context, l log.Logger) error

	// Modules returns the discovered modules.
	Modules() module.Modules

	// Scaffold scaffolds a module.
	Scaffold(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, module *module.Module) error

	// WithNewRepoFunc allows overriding the default function used to create repository instances.
	// This is primarily useful for testing.
	WithNewRepoFunc(fn NewRepoFunc) CatalogService

	// WithRepoURL allows overriding the repository URL.
	// This is primarily useful for testing.
	WithRepoURL(repoURL string) CatalogService

	// WithRepoURLs allows setting multiple repository URLs directly.
	// When set, these URLs take precedence over both WithRepoURL and catalog config.
	WithRepoURLs(urls []string) CatalogService

	// LoadStreamingURL clones a single repository and streams its modules
	// via onModule. Modules are accumulated internally so Modules()
	// returns the complete set across all LoadStreamingURL calls.
	LoadStreamingURL(ctx context.Context, l log.Logger, repoURL string, onModule ModuleFunc) error
}

// catalogServiceImpl is the concrete implementation of CatalogService.
// It holds the necessary options and configuration to perform its tasks.
type catalogServiceImpl struct {
	opts     *options.TerragruntOptions
	newRepo  NewRepoFunc
	repoURL  string
	repoURLs []string
	modules  module.Modules
	mu       sync.Mutex
}

// NewCatalogService creates a new instance of catalogServiceImpl with default settings.
// It requires TerragruntOptions and an optional initial repository URL.
// Configuration methods like WithNewRepoFunc can be chained to customize the service.
func NewCatalogService(opts *options.TerragruntOptions) *catalogServiceImpl {
	return &catalogServiceImpl{
		opts:    opts,
		newRepo: module.NewRepo,
	}
}

// WithNewRepoFunc allows overriding the default function used to create repository instances.
// This is primarily useful for testing.
func (s *catalogServiceImpl) WithNewRepoFunc(fn NewRepoFunc) CatalogService {
	s.newRepo = fn

	return s
}

// WithRepoURL allows overriding the repository URL.
// This is primarily useful for testing.
func (s *catalogServiceImpl) WithRepoURL(repoURL string) CatalogService {
	s.repoURL = repoURL

	return s
}

// WithRepoURLs sets multiple repository URLs directly.
// When set, these URLs take precedence over both WithRepoURL and catalog config.
func (s *catalogServiceImpl) WithRepoURLs(urls []string) CatalogService {
	s.repoURLs = urls

	return s
}

// Load implements the CatalogService interface.
// It contains the core logic for cloning/updating repositories and finding Terragrunt modules within them.
func (s *catalogServiceImpl) Load(ctx context.Context, l log.Logger) error {
	var repoURLs []string

	switch {
	case len(s.repoURLs) > 0:
		repoURLs = s.repoURLs
	case s.repoURL != "":
		repoURLs = []string{s.repoURL}
	default:
		_, pctx := configbridge.NewParsingContext(ctx, l, s.opts)

		catalogCfg, err := config.ReadCatalogConfig(ctx, l, pctx)
		if err != nil {
			return fmt.Errorf("failed to read catalog configuration: %w", err)
		}

		if catalogCfg != nil && len(catalogCfg.URLs) > 0 {
			repoURLs = catalogCfg.URLs
		} else {
			return errors.New("no catalog URLs provided")
		}
	}

	// Remove duplicates
	repoURLs = util.RemoveDuplicates(repoURLs)
	if len(repoURLs) == 0 || (len(repoURLs) == 1 && repoURLs[0] == "") {
		return errors.New("no valid repository URLs specified after configuration and flag processing")
	}

	var allModules module.Modules

	// Evaluate experimental features for symlinks and content-addressable storage.
	walkWithSymlinks := s.opts.Experiments.Evaluate(experiment.Symlinks)
	allowCAS := s.opts.Experiments.Evaluate(experiment.CAS)
	slowReporting := s.opts.Experiments.Evaluate(experiment.SlowTaskReporting)

	var errs []error

	for _, currentRepoURL := range repoURLs {
		if currentRepoURL == "" {
			l.Warnf("Empty repository URL encountered, skipping.")
			continue
		}

		tempPath := catalogTempPath(currentRepoURL)

		l.Debugf("Processing repository %s in temporary path %s", currentRepoURL, tempPath)

		// Initialize the repository. This might involve cloning or updating.
		// Use the newRepo function stored in the service instance.
		fsys := vfs.NewOSFS()

		repo, err := s.newRepo(ctx, l, fsys, &module.RepoOpts{
			CloneURL:         currentRepoURL,
			Path:             tempPath,
			WalkWithSymlinks: walkWithSymlinks,
			AllowCAS:         allowCAS,
			CASCloneDepth:    s.opts.CASCloneDepth,
			SlowReporting:    slowReporting,
			RootWorkingDir:   s.opts.RootWorkingDir,
		})
		if err != nil {
			l.Errorf("Failed to initialize repository %s: %v", currentRepoURL, err)

			errs = append(errs, err)

			continue
		}

		// Find modules within the initialized repository.
		repoModules, err := repo.FindModules(ctx, l, fsys)
		if err != nil {
			l.Errorf("Failed to find modules in repository %s: %v", currentRepoURL, err)

			errs = append(errs, err)

			continue
		}

		l.Debugf("Found %d module(s) in repository %q", len(repoModules), currentRepoURL)
		allModules = append(allModules, repoModules...)
	}

	s.mu.Lock()
	s.modules = allModules
	s.mu.Unlock()

	if len(errs) > 0 {
		return fmt.Errorf("failed to find modules in some repositories: %v", errs)
	}

	if len(allModules) == 0 {
		return errors.New("no modules found in any of the configured repositories")
	}

	return nil
}

// LoadStreamingURL clones a single repository and streams its modules
// via onModule. Modules are accumulated internally so Modules()
// returns the complete set across all LoadStreamingURL calls.
func (s *catalogServiceImpl) LoadStreamingURL(ctx context.Context, l log.Logger, repoURL string, onModule ModuleFunc) error {
	if repoURL == "" {
		l.Warnf("Empty repository URL encountered, skipping.")
		return nil
	}

	walkWithSymlinks := s.opts.Experiments.Evaluate(experiment.Symlinks)
	allowCAS := s.opts.Experiments.Evaluate(experiment.CAS)
	slowReporting := s.opts.Experiments.Evaluate(experiment.SlowTaskReporting)

	tempPath := catalogTempPath(repoURL)

	l.Debugf("Processing repository %s in temporary path %s", repoURL, tempPath)

	fsys := vfs.NewOSFS()

	repo, err := s.newRepo(ctx, l, fsys, &module.RepoOpts{
		CloneURL:         repoURL,
		Path:             tempPath,
		WalkWithSymlinks: walkWithSymlinks,
		AllowCAS:         allowCAS,
		SlowReporting:    slowReporting,
		RootWorkingDir:   s.opts.RootWorkingDir,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize repository %s: %w", repoURL, err)
	}

	repoModules, err := repo.FindModules(ctx, l, fsys)
	if err != nil {
		return fmt.Errorf("failed to find modules in repository %s: %w", repoURL, err)
	}

	l.Debugf("Found %d module(s) in repository %q", len(repoModules), repoURL)

	s.mu.Lock()
	s.modules = append(s.modules, repoModules...)
	s.mu.Unlock()

	for _, mod := range repoModules {
		onModule(mod)
	}

	return nil
}

func (s *catalogServiceImpl) Modules() module.Modules {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.modules)
}

func (s *catalogServiceImpl) Scaffold(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, module *module.Module) error {
	l.Debugf("Scaffolding module: %q", module.TerraformSourcePath())

	// TODO: thread venv from the CLI entrypoint through catalog service
	// so this leaf participates in the root virtualized environment.
	return scaffold.Run(ctx, l, venv.OSVenv(), opts, module.TerraformSourcePath(), "")
}

// catalogTempPath returns the local cache directory for repoURL's clone. It
// resolves os.TempDir() through any symlinks so the subdirectories module
// discovery derives via filepath.Rel stay inside the clone. macOS reports
// os.TempDir() as a /var/folders symlink to /private/var/folders; leaving it
// unresolved makes Rel emit a "../" traversal that go-getter rejects when
// scaffolding.
func catalogTempPath(repoURL string) string {
	encodedRepoURL := util.EncodeBase64Sha1(repoURL)
	return filepath.Join(util.ResolvePath(os.TempDir()), fmt.Sprintf(tempDirFormat, encodedRepoURL))
}
