package generate

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"

	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/panicreport"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Coordinator shares generation results for one command.
type Coordinator struct {
	store *rewrite.Store
	fs    vfs.FS
	opts  *options.TerragruntOptions
	jobs  map[string]*generationJob
	edges map[string][]string
	mu    sync.Mutex
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

// NewCoordinator shares generation results and the parse store for one command.
func NewCoordinator(fsys vfs.FS, s *rewrite.Store, opts *options.TerragruntOptions) *Coordinator {
	return &Coordinator{
		fs: fsys, store: s, opts: opts.Clone(),
		jobs:  make(map[string]*generationJob),
		edges: make(map[string][]string),
	}
}

type coordinatorKey struct{}

// WithCoordinator retains the original command's options across generation passes.
func WithCoordinator(
	ctx context.Context,
	fsys vfs.FS,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
) context.Context {
	if _, ok := ctx.Value(coordinatorKey{}).(*Coordinator); ok || opts.NoStackGenerate ||
		!opts.Experiments.Evaluate(experiment.DependencyStackGeneration) {
		return ctx
	}

	coordinator := NewCoordinator(fsys, s, opts)
	ctx = context.WithValue(ctx, coordinatorKey{}, coordinator)

	ctx = discovery.WithStackDiscovery(ctx, coordinator.discoverDependencyStacks)

	return config.WithStackGenerator(ctx, coordinator.generateDependencyStack)
}

func (c *Coordinator) generateDependencyStack(ctx context.Context, l log.Logger, v *venv.Venv, dir string) error {
	allowed, err := c.allowed(dir)
	if err != nil || !allowed {
		return err
	}

	dependencyOpts := c.opts.Clone()
	dependencyOpts.WorkingDir = dir
	dependencyOpts.TerragruntConfigPath = filepath.Join(dir, config.DefaultTerragruntConfigPath)
	dependencyOpts.TerragruntStackConfigPath = filepath.Join(dir, config.DefaultStackFile)
	// The walk stays under this dependency; each job also checks the original command's limits.
	dependencyOpts.DiscoveryBoundary = dir
	dependencyOpts.Filters = nil

	return NewGenerator().generateStacks(ctx, l, v, c.store, dependencyOpts, nil, dependencyStacks)
}

// Generate shares one result per eligible stack file, including failures.
// The callback must finish queued work before returning and use the supplied
// context for recursive calls so cycles can be detected.
func (c *Coordinator) Generate(
	ctx context.Context,
	stackFile string,
	generate func(context.Context) error,
) (result error) {
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

	allowed, err := c.allowed(filepath.Dir(path))
	if err != nil || !allowed {
		return err
	}

	return c.generateSelected(ctx, path, generate)
}

// Discovery has already checked these paths against generation limits, including
// limits mirrored into comparison worktrees.
func (c *Coordinator) generateSelected(
	ctx context.Context,
	path string,
	generate func(context.Context) error,
) (result error) {
	if err := ctx.Err(); err != nil {
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
		if err := c.clean(path); err != nil {
			return err
		}
	}

	return generate(ctx)
}

// Keep the overlap check and deletion atomic with respect to job registration.
func (c *Coordinator) clean(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	target := filepath.Join(filepath.Dir(path), config.StackDir)
	for registered := range c.jobs {
		if vfs.Within(c.fs, target, registered) {
			return fmt.Errorf(
				"cannot clean stack %q: nested stack %q was already requested; generate parent stacks first",
				path,
				registered,
			)
		}
	}

	if err := c.fs.RemoveAll(target); err != nil {
		return fmt.Errorf("failed to clean stack directory %q: %w", target, err)
	}

	return nil
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

func (c *Coordinator) allowed(dir string) (bool, error) {
	return discovery.StackDependencyAllowed(
		c.fs,
		discovery.StackGenerateOptions{
			WorkingDir:        c.opts.WorkingDir,
			DiscoveryBoundary: c.opts.DiscoveryBoundary,
			Filters:           c.opts.Filters,
		},
		dir,
	)
}
