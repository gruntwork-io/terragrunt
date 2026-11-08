package generate

import (
	"context"
	"errors"
	"path/filepath"
	"runtime/debug"
	"sync"

	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
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
	pending sync.WaitGroup
	mu      sync.Mutex
	errs    []error
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

func generateCoordinatedLevel(ctx context.Context, l log.Logger, v *venv.Venv, s *rewrite.Store, opts *options.TerragruntOptions, nodes []*StackNode, generatedFiles map[string]bool, coordinator *Coordinator) error {
	pool := worker.NewWorkerPool(opts.Parallelism)
	defer pool.Stop()
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
			return coordinator.generateSelected(ctx, node.FilePath, func(ctx context.Context) (result error) {
				defer s.InvalidateUnder(filepath.Dir(node.FilePath))
				tasks := &stackTasks{pool: pool}
				// Even a parse failure or panic can follow submitted work.
				defer func() { result = errors.Join(result, tasks.Wait()) }()
				pctx := configbridge.NewParsingContext(opts)
				scopedLogger, scopedPctx, err := pctx.WithConfigPath(l, node.FilePath)
				if err != nil {
					return err
				}
				return config.GenerateStackFile(ctx, scopedLogger, v, scopedPctx, tasks, node.FilePath)
			})
		})
	}
	return errors.Join(group.Wait(), pool.Wait())
}
