package discovery

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// FilesystemPhase walks directories to discover Terragrunt configurations.
type FilesystemPhase struct {
	// numWorkers is the number of concurrent workers.
	numWorkers int
}

// NewFilesystemPhase creates a new FilesystemPhase.
func NewFilesystemPhase(numWorkers int) *FilesystemPhase {
	numWorkers = max(numWorkers, defaultDiscoveryWorkers)

	return &FilesystemPhase{
		numWorkers: numWorkers,
	}
}

// Name returns the human-readable name of the phase.
func (p *FilesystemPhase) Name() string {
	return "filesystem"
}

// Kind returns the PhaseKind identifier.
func (p *FilesystemPhase) Kind() PhaseKind {
	return PhaseFilesystem
}

// Run executes the filesystem discovery phase.
func (p *FilesystemPhase) Run(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	input *PhaseInput,
) (*PhaseResults, error) {
	results := NewPhaseResults()

	discovery := input.Discovery
	if discovery == nil {
		return nil, NewClassificationError("", "discovery configuration is nil")
	}

	discoveryContext := discovery.discoveryContext
	if discoveryContext == nil || discoveryContext.WorkingDir == "" {
		return nil, NewClassificationError("", "discovery context or working directory is nil")
	}

	filenames := discovery.configFilenames
	if len(filenames) == 0 {
		filenames = DefaultConfigFilenames
	}

	walkFn := walkDirFunc(v, input.Opts)

	walkStarts := discovery.walkRoots
	if len(walkStarts) == 0 {
		walkStarts = []string{discoveryContext.WorkingDir}
	}

	for _, walkStart := range walkStarts {
		if err := p.walk(ctx, walkFn, walkStart, input, discovery, filenames, results); err != nil {
			return results, err
		}
	}

	return results, nil
}

// walk classifies every configuration file under walkStart into results.
func (p *FilesystemPhase) walk(
	ctx context.Context,
	walkFn func(string, fs.WalkDirFunc) error,
	walkStart string,
	input *PhaseInput,
	discovery *Discovery,
	filenames []string,
	results *PhaseResults,
) error {
	return walkFn(walkStart, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			return p.skipDirIfIgnorable(discovery, d.Name())
		}

		result := p.processFile(input, path, filenames)
		if result == nil {
			return nil
		}

		switch result.Status {
		case filter.StatusReadyForFilter:
			results.AddDiscovered(*result)
		case filter.StatusCandidate:
			results.AddCandidate(*result)
		case filter.StatusExcluded:
			// Excluded components are not added
		}

		return nil
	})
}

// skipDirIfIgnorable determines if a directory should be skipped during traversal.
func (p *FilesystemPhase) skipDirIfIgnorable(discovery *Discovery, dir string) error {
	if err := util.SkipDirIfIgnorable(dir); err != nil {
		return err
	}

	if discovery.noHidden {
		if strings.HasPrefix(dir, ".") && dir != "." && dir != ".." {
			return filepath.SkipDir
		}
	}

	return nil
}

// processFile processes a single file to determine if it's a Terragrunt configuration
// and classifies it as discovered, candidate, or excluded.
func (p *FilesystemPhase) processFile(
	input *PhaseInput,
	path string,
	filenames []string,
) *DiscoveryResult {
	discovery := input.Discovery

	c := createComponentFromPath(path, filenames, discovery.discoveryContext)
	if c == nil {
		return nil
	}

	if input.Classifier != nil {
		ctx := filter.ClassificationContext{}
		status, reason, graphIdx := input.Classifier.Classify(c, ctx)

		return &DiscoveryResult{
			Component:            c,
			Status:               status,
			Reason:               reason,
			Phase:                PhaseFilesystem,
			GraphExpressionIndex: graphIdx,
		}
	}

	return &DiscoveryResult{
		Component: c,
		Status:    filter.StatusReadyForFilter,
		Reason:    filter.CandidacyReasonNone,
		Phase:     PhaseFilesystem,
	}
}
