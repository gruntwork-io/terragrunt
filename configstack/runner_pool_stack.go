package configstack

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// RunnerPoolStack implements the Stack interface using discovery, queue, and worker pool for run --all/--graph
// This is a new stack implementation that leverages the new discovery and queue packages for abstract handling.
type RunnerPoolStack struct {
	discovered            discovery.DiscoveredConfigs
	queue                 *queue.Queue
	terragruntOptions     *options.TerragruntOptions
	parserOptions         []hclparse.Option
	childTerragruntConfig *config.TerragruntConfig
	outputMu              sync.Mutex
}

// NewRunnerPoolStack creates a new RunnerPoolStack.
func NewRunnerPoolStack(discovered discovery.DiscoveredConfigs, q *queue.Queue, terragruntOptions *options.TerragruntOptions, opts ...Option) *RunnerPoolStack {
	return &RunnerPoolStack{
		discovered:        discovered,
		queue:             q,
		terragruntOptions: terragruntOptions,
		parserOptions:     nil, // can be set via SetParseOptions
	}
}

// String renders this stack as a human-readable string
func (stack *RunnerPoolStack) String() string {
	modules := []string{}
	for _, entry := range stack.queue.Entries {
		modules = append(modules, "  => "+entry.Config.Path)
	}
	sort.Strings(modules)
	return fmt.Sprintf("RunnerPoolStack at %s:\n%s", stack.terragruntOptions.WorkingDir, strings.Join(modules, "\n"))
}

// LogModuleDeployOrder logs the modules in the order they will be deployed
func (stack *RunnerPoolStack) LogModuleDeployOrder(l log.Logger, terraformCommand string) error {
	outStr := fmt.Sprintf("The stack at %s will be processed in the following order for command %s:\n", stack.terragruntOptions.WorkingDir, terraformCommand)
	for i, entry := range stack.queue.Entries {
		outStr += fmt.Sprintf("%d. %s\n", i+1, entry.Config.Path)
	}
	l.Info(outStr)
	return nil
}

// JSONModuleDeployOrder returns the module deploy order as JSON
func (stack *RunnerPoolStack) JSONModuleDeployOrder(terraformCommand string) (string, error) {
	order := make([]string, len(stack.queue.Entries))
	for i, entry := range stack.queue.Entries {
		order[i] = entry.Config.Path
	}
	j, err := json.MarshalIndent(order, "", "  ")
	if err != nil {
		return "", err
	}
	return string(j), nil
}

// Graph is a stub for now
func (stack *RunnerPoolStack) Graph(l log.Logger, opts *options.TerragruntOptions) {
	// TODO: Implement graphviz output if needed
	l.Warnf("Graph output not implemented for RunnerPoolStack")
}

// runnerStatus represents the status of a unit in the runner pool
// (not exported, internal to this file)
type runnerStatus int

const (
	statusReady runnerStatus = iota
	statusPending
	statusRunning
	statusSucceeded
	statusFailed
	statusAncestorFailed
	statusFailFast
)

// runnerUnit represents a unit in the runner pool with dependency and status tracking
// (not exported, internal to this file)
type runnerUnit struct {
	entry      *queue.Entry
	blockedBy  map[string]struct{} // paths of units this is blocked by
	dependents map[string]struct{} // paths of units that depend on this
	status     runnerStatus
	err        error
}

// runnerPoolManager manages the dynamic scheduling and status of units
// (not exported, internal to this file)
type runnerPoolManager struct {
	units         map[string]*runnerUnit // path -> unit
	failFast      bool
	failEarly     bool
	remaining     int
	pool          *worker.Pool
	log           log.Logger
	terragruntOpt *options.TerragruntOptions
}

// newRunnerPoolManager builds the dependency graph and initializes statuses
func newRunnerPoolManager(entries []*queue.Entry, pool *worker.Pool, l log.Logger, terragruntOpt *options.TerragruntOptions, failFast, failEarly bool) *runnerPoolManager {
	units := make(map[string]*runnerUnit)
	// First pass: create all units
	for _, entry := range entries {
		units[entry.Config.Path] = &runnerUnit{
			entry:      entry,
			blockedBy:  make(map[string]struct{}),
			dependents: make(map[string]struct{}),
			status:     statusReady, // will update below
		}
	}
	// Second pass: fill dependencies and dependents
	for _, unit := range units {
		for _, dep := range unit.entry.Config.Dependencies {
			if depUnit, ok := units[dep.Path]; ok {
				unit.blockedBy[dep.Path] = struct{}{}
				depUnit.dependents[unit.entry.Config.Path] = struct{}{}
			}
		}
		if len(unit.blockedBy) > 0 {
			unit.status = statusPending
		}
	}
	return &runnerPoolManager{
		units:         units,
		failFast:      failFast,
		failEarly:     failEarly,
		remaining:     len(units),
		pool:          pool,
		log:           l,
		terragruntOpt: terragruntOpt,
	}
}

// scheduleReadyUnits adds all ready units to the pool up to concurrency limit
func (m *runnerPoolManager) scheduleReadyUnits() {
	for _, unit := range m.units {
		if unit.status == statusReady {
			unit.status = statusRunning
			m.pool.Submit(m.makeTask(unit))
		}
	}
}

// makeTask returns a worker.Task for the given unit
func (m *runnerPoolManager) makeTask(unit *runnerUnit) worker.Task {
	return func() error {
		m.log.Infof("Running: %s", unit.entry.Config.Path)
		// TODO: Replace with actual Terragrunt execution logic
		// Simulate success for now
		err := m.runUnit(unit)
		m.onUnitComplete(unit, err)
		return err
	}
}

// runUnit is where actual Terragrunt execution would go
func (m *runnerPoolManager) runUnit(unit *runnerUnit) error {
	// TODO: Integrate with Terragrunt execution
	return nil // Simulate success
}

// onUnitComplete updates statuses and propagates as needed
func (m *runnerPoolManager) onUnitComplete(unit *runnerUnit, err error) {
	if err == nil {
		unit.status = statusSucceeded
		m.log.Infof("Succeeded: %s", unit.entry.Config.Path)
		// Unblock dependents
		for depPath := range unit.dependents {
			depUnit := m.units[depPath]
			delete(depUnit.blockedBy, unit.entry.Config.Path)
			if len(depUnit.blockedBy) == 0 && depUnit.status == statusPending {
				depUnit.status = statusReady
			}
		}
	} else {
		unit.status = statusFailed
		unit.err = err
		m.log.Errorf("Failed: %s: %v", unit.entry.Config.Path, err)
		// Propagate ancestor failed
		for depPath := range unit.dependents {
			m.propagateAncestorFailed(depPath)
		}
		if m.failFast {
			for _, u := range m.units {
				if u.status == statusReady || u.status == statusPending {
					u.status = statusFailFast
					m.log.Warnf("Fail fast: %s", u.entry.Config.Path)
				}
			}
		}
	}
	m.remaining--
}

// propagateAncestorFailed recursively marks dependents as ancestor failed
func (m *runnerPoolManager) propagateAncestorFailed(path string) {
	if u, ok := m.units[path]; ok && u.status != statusAncestorFailed && u.status != statusFailed {
		u.status = statusAncestorFailed
		m.log.Warnf("Ancestor failed: %s", u.entry.Config.Path)
		for depPath := range u.dependents {
			m.propagateAncestorFailed(depPath)
		}
	}
}

// allDone returns true if all units are finished or evicted
func (m *runnerPoolManager) allDone() bool {
	for _, u := range m.units {
		if u.status == statusReady || u.status == statusPending || u.status == statusRunning {
			return false
		}
	}
	return true
}

// Run executes the stack using the improved runner pool model
func (stack *RunnerPoolStack) Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	failFast := false  // TODO: wire from flag
	failEarly := false // TODO: wire from flag
	pool := worker.NewWorkerPool(opts.Parallelism)
	pool.Start()

	mgr := newRunnerPoolManager(stack.queue.Entries, pool, l, opts, failFast, failEarly)

	for {
		mgr.scheduleReadyUnits()
		if mgr.allDone() {
			break
		}
		// Sleep or yield to allow tasks to complete
		// In a real implementation, use channels/condvars for efficiency
	}

	return pool.GracefulStop()
}

// GetModuleRunGraph returns the run order as a single group (for now)
func (stack *RunnerPoolStack) GetModuleRunGraph(terraformCommand string) ([]TerraformModules, error) {
	// TODO: Optionally group by concurrency level
	group := make(TerraformModules, len(stack.queue.Entries))
	for i, entry := range stack.queue.Entries {
		group[i] = &TerraformModule{Path: entry.Config.Path}
	}
	return []TerraformModules{group}, nil
}

// ListStackDependentModules returns a map of module dependencies
func (stack *RunnerPoolStack) ListStackDependentModules() map[string][]string {
	deps := make(map[string][]string)
	for _, entry := range stack.queue.Entries {
		for _, dep := range entry.Config.Dependencies {
			deps[dep.Path] = append(deps[dep.Path], entry.Config.Path)
		}
	}
	return deps
}

// Modules returns the discovered modules as TerraformModules
func (stack *RunnerPoolStack) Modules() TerraformModules {
	modules := make(TerraformModules, len(stack.queue.Entries))
	for i, entry := range stack.queue.Entries {
		modules[i] = &TerraformModule{Path: entry.Config.Path}
	}
	return modules
}

// FindModuleByPath finds a module by its path
func (stack *RunnerPoolStack) FindModuleByPath(path string) *TerraformModule {
	for _, entry := range stack.queue.Entries {
		if entry.Config.Path == path {
			return &TerraformModule{Path: path}
		}
	}
	return nil
}

// SetTerragruntConfig sets the child Terragrunt config
func (stack *RunnerPoolStack) SetTerragruntConfig(config *config.TerragruntConfig) {
	stack.childTerragruntConfig = config
}

// GetTerragruntConfig returns the child Terragrunt config
func (stack *RunnerPoolStack) GetTerragruntConfig() *config.TerragruntConfig {
	return stack.childTerragruntConfig
}

// SetParseOptions sets the parser options
func (stack *RunnerPoolStack) SetParseOptions(parserOptions []hclparse.Option) {
	stack.parserOptions = parserOptions
}

// GetParseOptions returns the parser options
func (stack *RunnerPoolStack) GetParseOptions() []hclparse.Option {
	return stack.parserOptions
}

// Lock locks the stack for concurrency control
func (stack *RunnerPoolStack) Lock() {
	stack.outputMu.Lock()
}

// Unlock unlocks the stack for concurrency control
func (stack *RunnerPoolStack) Unlock() {
	stack.outputMu.Unlock()
}
