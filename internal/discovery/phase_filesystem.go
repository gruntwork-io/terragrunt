package discovery

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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

	visit := func(path string, d fs.DirEntry, err error) error {
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
	}

	roots, reasons := input.Classifier.WalkRoots(discoveryContext.WorkingDir)

	l.Debugf("Discovery: pruned walk from %d root(s): %s", len(roots), strings.Join(reasons, "; "))

	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(
			attribute.Int("walk_roots", len(roots)),
			attribute.StringSlice("walk_reasons", reasons),
		)
	}

	walkRoot, err := walkRootUnder(v.FS, discoveryContext.WorkingDir, discovery.walkRoot)
	if err != nil {
		return nil, err
	}

	walk := &prunedWalk{
		visit:       visit,
		workingDir:  discoveryContext.WorkingDir,
		walkRoot:    walkRoot,
		numWorkers:  p.numWorkers,
		followLinks: input.Opts.Experiments.Evaluate(experiment.Symlinks),
	}

	return results, walk.run(ctx, v.FS, roots)
}

// walkRootUnder spells walkRoot below workingDir, since the pruned walk
// compares paths as text. An empty walkRoot means the whole working directory.
// [NewForStackGenerate] sets walkRoot only when it sits within workingDir, but
// walkRoot has its symlinks resolved and workingDir may not.
func walkRootUnder(fsys vfs.FS, workingDir, walkRoot string) (string, error) {
	if walkRoot == "" {
		return workingDir, nil
	}

	rel, err := filepath.Rel(vfs.ResolveForCompare(fsys, workingDir), vfs.ResolveForCompare(fsys, walkRoot))
	if err != nil {
		return "", err
	}

	return filepath.Join(workingDir, rel), nil
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
