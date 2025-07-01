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
	"fmt"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
)

// NewRepoFunc defines the signature for a function that creates a new repository.
// This allows for mocking in tests.
type NewRepoFunc func(ctx context.Context, l log.Logger, cloneURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error)

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
}

// catalogServiceImpl is the concrete implementation of CatalogService.
// It holds the necessary options and configuration to perform its tasks.
type catalogServiceImpl struct {
	opts    *options.TerragruntOptions
	newRepo NewRepoFunc
	repoURL string
	modules module.Modules
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

// Load implements the CatalogService interface.
// It contains the core logic for cloning/updating repositories and finding Terragrunt modules within them.
func (s *catalogServiceImpl) Load(ctx context.Context, l log.Logger) error {
	repoURLs := []string{s.repoURL}

	// If no specific repoURL was provided to the service, try to read from catalog config.
	if s.repoURL == "" {
		catalogCfg, err := config.ReadCatalogConfig(ctx, l, s.opts)
		if err != nil {
			return errors.Errorf("failed to read catalog configuration: %w", err)
		}

		if catalogCfg != nil && len(catalogCfg.URLs) > 0 {
			repoURLs = catalogCfg.URLs
		} else {
			return errors.Errorf("no catalog URLs provided")
		}
	}

	// Remove duplicates
	repoURLs = util.RemoveDuplicatesFromList(repoURLs)
	if len(repoURLs) == 0 || (len(repoURLs) == 1 && repoURLs[0] == "") {
		return errors.Errorf("no valid repository URLs specified after configuration and flag processing")
	}

	var allModules module.Modules

	// Evaluate experimental features for symlinks and content-addressable storage.
	walkWithSymlinks := s.opts.Experiments.Evaluate(experiment.Symlinks)
	allowCAS := s.opts.Experiments.Evaluate(experiment.CAS)

	var errs []error

	for _, currentRepoURL := range repoURLs {
		if currentRepoURL == "" {
			l.Warnf("Empty repository URL encountered, skipping.")
			continue
		}

		// Create a unique path in the system's temporary directory for this repository.
		// The path is based on a SHA1 hash of the repository URL to ensure uniqueness and idempotency.
		encodedRepoURL := util.EncodeBase64Sha1(currentRepoURL)
		tempPath := filepath.Join(os.TempDir(), fmt.Sprintf(tempDirFormat, encodedRepoURL))

		l.Debugf("Processing repository %s in temporary path %s", currentRepoURL, tempPath)

		// Initialize the repository. This might involve cloning or updating.
		// Use the newRepo function stored in the service instance.
		repo, err := s.newRepo(ctx, l, currentRepoURL, tempPath, walkWithSymlinks, allowCAS)
		if err != nil {
			l.Errorf("Failed to initialize repository %s: %v", currentRepoURL, err)

			errs = append(errs, err)

			continue
		}

		// Find modules within the initialized repository.
		repoModules, err := repo.FindModules(ctx)
		if err != nil {
			l.Errorf("Failed to find modules in repository %s: %v", currentRepoURL, err)

			errs = append(errs, err)

			continue
		}

		l.Infof("Found %d module(s) in repository %q", len(repoModules), currentRepoURL)
		allModules = append(allModules, repoModules...)
	}

	s.modules = allModules

	if len(errs) > 0 {
		return errors.Errorf("failed to find modules in some repositories: %v", errs)
	}

	if len(allModules) == 0 {
		return errors.Errorf("no modules found in any of the configured repositories")
	}

	return nil
}

func (s *catalogServiceImpl) Modules() module.Modules {
	return s.modules
}

func (s *catalogServiceImpl) Scaffold(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, module *module.Module) error {
	l.Infof("Scaffolding module: %q", module.TerraformSourcePath())

	return scaffold.Run(ctx, l, opts, module.TerraformSourcePath(), "")
}
