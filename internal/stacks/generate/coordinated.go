package generate

import (
	"context"
	"errors"
	"path/filepath"
	"runtime/debug"
	"sync"

	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/panicreport"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"golang.org/x/sync/errgroup"
)

// stackTasks keeps one stack's result separate while sharing the worker limit.
type stackTasks struct {
	pool    *worker.Pool
	errs    []error
	pending sync.WaitGroup
	mu      sync.Mutex
}

func (s *stackTasks) Submit(task worker.Task) {
	s.pending.Add(1)
	s.pool.Submit(func() (result error) {
		defer s.pending.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				result = panicreport.NewRecoveredError(recovered, debug.Stack())
			}

			if result != nil {
				s.mu.Lock()
				s.errs = append(s.errs, result)
				s.mu.Unlock()
			}

			result = nil
		}()

		return task()
	})
}

func (s *stackTasks) Wait() error {
	s.pending.Wait()
	return errors.Join(s.errs...)
}

type generationPoolKey struct{}

func generateCoordinatedLevel(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	nodes []*StackNode,
	generatedFiles map[string]bool,
	coordinator *Coordinator,
	scope stackScope,
) error {
	pool, shared := ctx.Value(generationPoolKey{}).(*worker.Pool)
	if !shared {
		pool = worker.NewWorkerPool(opts.Parallelism)

		ctx = context.WithValue(ctx, generationPoolKey{}, pool)
		defer pool.Stop()
	}

	var group errgroup.Group
	group.SetLimit(max(opts.Parallelism, 1))

	for _, node := range nodes {
		if generatedFiles[node.FilePath] {
			continue
		}

		generatedFiles[node.FilePath] = true
		if !vfs.Exists(v.FS, node.FilePath) {
			continue
		}

		group.Go(func() error {
			generate := coordinator.generateSelected
			if scope == dependencyStacks {
				generate = coordinator.Generate
			}

			return generate(ctx, node.FilePath, func(ctx context.Context) error {
				pctx := configbridge.NewParsingContext(opts)

				scopedLogger, scopedPctx, err := pctx.WithConfigPath(l, node.FilePath)
				if err != nil {
					return err
				}

				stack, err := generateStackFileAndWait(ctx, scopedLogger, v, s, scopedPctx, pool, node.FilePath)
				if err != nil {
					return err
				}

				return generateUnitDependencies(ctx, scopedLogger, v, opts, node.FilePath, stack.Units)
			})
		})
	}

	err := group.Wait()
	if shared {
		return err
	}

	return errors.Join(err, pool.Wait())
}

func generateUnitDependencies(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	stackFile string,
	units []*config.Unit,
) error {
	for _, unit := range units {
		if !unit.IsEnabled() {
			continue
		}

		path := filepath.Join(unit.GeneratedPath(filepath.Dir(stackFile)), config.DefaultTerragruntConfigPath)
		if !vfs.Exists(v.FS, path) {
			continue
		}

		if err := discovery.GenerateUnitDependencies(ctx, l, v, opts, path); err != nil {
			return err
		}
	}

	return nil
}

func generateStackFileAndWait(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	pctx *config.ParsingContext,
	pool *worker.Pool,
	path string,
) (stack *config.StackConfig, err error) {
	tasks := &stackTasks{pool: pool}
	// A parser panic must also wait for any work already submitted.
	defer func() {
		err = errors.Join(err, tasks.Wait())

		s.InvalidateUnder(filepath.Dir(path))
	}()

	return config.GenerateStackFile(ctx, l, v, pctx, tasks, path)
}
