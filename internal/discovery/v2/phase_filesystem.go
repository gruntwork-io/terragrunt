package v2

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"golang.org/x/sync/errgroup"
)

// FilesystemPhase walks directories to discover Terragrunt configurations.
type FilesystemPhase struct {
	// numWorkers is the number of concurrent workers.
	numWorkers int
}

// NewFilesystemPhase creates a new FilesystemPhase.
func NewFilesystemPhase(numWorkers int) *FilesystemPhase {
	if numWorkers <= 0 {
		numWorkers = defaultDiscoveryWorkers
	}

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
func (p *FilesystemPhase) Run(ctx context.Context, input *PhaseInput) PhaseOutput {
	discovered := make(chan DiscoveryResult, p.numWorkers*channelBufferMultiplier)
	candidates := make(chan DiscoveryResult, p.numWorkers*channelBufferMultiplier)
	errors := make(chan error, p.numWorkers)
	done := make(chan struct{})

	go func() {
		defer close(discovered)
		defer close(candidates)
		defer close(errors)
		defer close(done)

		p.runDiscovery(ctx, input, discovered, candidates, errors)
	}()

	return PhaseOutput{
		Discovered: discovered,
		Candidates: candidates,
		Done:       done,
		Errors:     errors,
	}
}

// runDiscovery performs the actual filesystem discovery.
func (p *FilesystemPhase) runDiscovery(
	ctx context.Context,
	input *PhaseInput,
	discovered chan<- DiscoveryResult,
	candidates chan<- DiscoveryResult,
	errors chan<- error,
) {
	discovery := input.Discovery
	if discovery == nil {
		errors <- NewClassificationError("", "discovery configuration is nil")
		return
	}

	discoveryContext := discovery.discoveryContext
	if discoveryContext == nil || discoveryContext.WorkingDir == "" {
		errors <- NewClassificationError("", "discovery context or working directory is nil")
		return
	}

	filenames := discovery.configFilenames
	if len(filenames) == 0 {
		filenames = DefaultConfigFilenames
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.numWorkers)

	filePaths := make(chan string, p.numWorkers*channelBufferMultiplier)

	g.Go(func() error {
		defer close(filePaths)

		walkFn := filepath.WalkDir
		if input.Opts != nil && input.Opts.Experiments.Evaluate(experiment.Symlinks) {
			walkFn = util.WalkDirWithSymlinks
		}

		return walkFn(discoveryContext.WorkingDir, func(path string, d fs.DirEntry, err error) error {
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

			select {
			case filePaths <- path:
			case <-ctx.Done():
				return ctx.Err()
			}

			return nil
		})
	})

	g.Go(func() error {
		for path := range filePaths {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			result := p.processFile(input, path, filenames)
			if result == nil {
				continue
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				switch result.Status {
				case StatusDiscovered:
					select {
					case discovered <- *result:
					case <-ctx.Done():
						return ctx.Err()
					}
				case StatusCandidate:
					select {
					case candidates <- *result:
					case <-ctx.Done():
						return ctx.Err()
					}
				case StatusExcluded:
					// Excluded components are not sent to any channel
				}
			}
		}

		return nil
	})

	if err := g.Wait(); err != nil {
		select {
		case errors <- err:
		default:
		}
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
		Status:    StatusDiscovered,
		Reason:    CandidacyReasonNone,
		Phase:     PhaseFilesystem,
	}
}
