package generate

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/panicreport"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Coordinator shares generation results for one command.
type Coordinator struct {
	fs    vfs.FS
	opts  *options.TerragruntOptions
	mu    sync.Mutex
	jobs  map[string]*generationJob
	edges map[string][]string
}

type generationJob struct {
	done chan struct{}
	err  error
}

type generationOwner struct {
	coordinator *Coordinator
	path        string
}

type generationOwnerKey struct{}

func NewCoordinator(fsys vfs.FS, opts *options.TerragruntOptions) *Coordinator {
	return &Coordinator{
		fs: fsys, opts: opts.Clone(),
		jobs:  make(map[string]*generationJob),
		edges: make(map[string][]string),
	}
}

// Generate shares one result per eligible stack file, including failures.
// The callback must finish queued work before returning and use the supplied
// context for recursive calls so cycles can be detected.
func (c *Coordinator) Generate(ctx context.Context, stackFile string, generate func(context.Context) error) (result error) {
	if c.opts.NoStackGenerate || !c.opts.Experiments.Evaluate(experiment.DependencyStackGeneration) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := util.CanonicalResolvedPath(c.fs, stackFile, c.opts.WorkingDir)
	if err != nil {
		return err
	}
	allowed, err := discovery.StackDependencyAllowed(c.fs, discovery.StackGenerateOptions{
		WorkingDir:        c.opts.WorkingDir,
		DiscoveryBoundary: c.opts.DiscoveryBoundary,
		Filters:           c.opts.Filters,
	}, filepath.Dir(path))
	if err != nil || !allowed {
		return err
	}

	c.mu.Lock()
	owner, _ := ctx.Value(generationOwnerKey{}).(generationOwner)
	if owner.coordinator == c {
		if route := c.route(path, owner.path, make(map[string]bool)); route != nil {
			c.mu.Unlock()
			return fmt.Errorf("stack dependency cycle: %s", strings.Join(append([]string{owner.path}, route...), " -> "))
		}
		c.edges[owner.path] = append(c.edges[owner.path], path)
	}
	if job, exists := c.jobs[path]; exists {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-job.done:
			return job.err
		}
	}
	job := &generationJob{done: make(chan struct{})}
	c.jobs[path] = job
	c.mu.Unlock()

	ctx = context.WithValue(ctx, generationOwnerKey{}, generationOwner{coordinator: c, path: path})
	defer func() {
		if recovered := recover(); recovered != nil {
			result = panicreport.NewRecoveredError(recovered, debug.Stack())
		}
		c.mu.Lock()
		job.err = result
		close(job.done)
		c.mu.Unlock()
	}()
	return c.generate(ctx, path, generate)
}

func (c *Coordinator) generate(ctx context.Context, path string, generate func(context.Context) error) error {
	if c.opts.SourceUpdate && !c.opts.TerraformCliArgs.IsDestroy(c.opts.TofuCommand) {
		// Clean only this stack's generated directory, never sibling stacks.
		target := filepath.Join(filepath.Dir(path), config.StackDir)
		if err := c.fs.RemoveAll(target); err != nil {
			return fmt.Errorf("failed to clean stack directory %q: %w", target, err)
		}
	}
	return generate(ctx)
}

// route requires c.mu to be held.
func (c *Coordinator) route(from, to string, visited map[string]bool) []string {
	if from == to {
		return []string{from}
	}
	if visited[from] {
		return nil
	}
	visited[from] = true
	for _, next := range c.edges[from] {
		if tail := c.route(next, to, visited); tail != nil {
			return append([]string{from}, tail...)
		}
	}
	return nil
}
