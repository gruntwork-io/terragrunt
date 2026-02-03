package discovery

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/util"
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
func (p *FilesystemPhase) Run(ctx context.Context, l log.Logger, input *PhaseInput) PhaseOutput {
	collector := NewResultCollector()

	p.runDiscovery(ctx, l, input, collector)

	discovered, candidates, errs := collector.Results()

	return PhaseOutput{
		Discovered: discovered,
		Candidates: candidates,
		Errors:     errs,
	}
}

// runDiscovery performs the actual filesystem discovery.
func (p *FilesystemPhase) runDiscovery(
	ctx context.Context,
	l log.Logger,
	input *PhaseInput,
	collector *ResultCollector,
) {
	discovery := input.Discovery
	if discovery == nil {
		collector.AddError(NewClassificationError("", "discovery configuration is nil"))
		return
	}

	discoveryContext := discovery.discoveryContext
	if discoveryContext == nil || discoveryContext.WorkingDir == "" {
		collector.AddError(NewClassificationError("", "discovery context or working directory is nil"))
		return
	}

	filenames := discovery.configFilenames
	if len(filenames) == 0 {
		filenames = DefaultConfigFilenames
	}

	walkFn := filepath.WalkDir
	if input.Opts != nil && input.Opts.Experiments.Evaluate(experiment.Symlinks) {
		walkFn = util.WalkDirWithSymlinks
	}

	err := walkFn(discoveryContext.WorkingDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			return p.skipDirIfIgnorable(discovery, path)
		}

		result := p.processFile(l, input, path, filenames)
		if result == nil {
			return nil
		}

		switch result.Status {
		case StatusDiscovered:
			collector.AddDiscovered(*result)
		case StatusCandidate:
			collector.AddCandidate(*result)
		case StatusExcluded:
			// Excluded components are not added
		}

		return nil
	})
	if err != nil {
		collector.AddError(err)
	}
}

// skipDirIfIgnorable determines if a directory should be skipped during traversal.
func (p *FilesystemPhase) skipDirIfIgnorable(discovery *Discovery, path string) error {
	if err := skipDirIfIgnorable(path); err != nil {
		return err
	}

	if discovery.noHidden {
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") && base != "." && base != ".." {
			return filepath.SkipDir
		}
	}

	return nil
}

// processFile processes a single file to determine if it's a Terragrunt configuration
// and classifies it as discovered, candidate, or excluded.
func (p *FilesystemPhase) processFile(
	l log.Logger,
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
		status, reason, graphIdx := input.Classifier.Classify(l, c, ctx)

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
		Status:    StatusDiscovered,
		Reason:    CandidacyReasonNone,
		Phase:     PhaseFilesystem,
	}
}
