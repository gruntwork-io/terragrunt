// Package experiment provides utilities used by Terragrunt to support an "experiment" mode.
// By default, experiment mode is disabled, but when enabled, experimental features can be enabled.
// These features are not yet stable and may change in the future.
//
// Note that any behavior outlined here should be documented in /docs/_docs/04_reference/experiments.md
//
// That is how users will know what to expect when they enable experiment mode, and how to customize it.
package experiment

import (
	"slices"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// Symlinks is the experiment that allows symlinks to be used in Terragrunt configurations.
	Symlinks = "symlinks"
	// CLIRedesign is an experiment that allows users to use new commands related to the CLI redesign.
	CLIRedesign = "cli-redesign"
	// Stacks is the experiment that allows stacks to be used in Terragrunt.
	Stacks = "stacks"
	// CAS is the experiment that enables using the CAS package for git operations
	// in the catalog command, which provides better performance through content-addressable storage.
	CAS = "cas"
	// Report is the experiment that enables the new run report.
	Report = "report"
	// RunnerPool is the experiment that allows using a pool of runners for parallel execution.
	RunnerPool = "runner-pool"
	// AutoProviderCacheDir is the experiment that automatically enables central
	// provider caching by setting TF_PLUGIN_CACHE_DIR.
	//
	// Only works with OpenTofu version >= 1.10.
	AutoProviderCacheDir = "auto-provider-cache-dir"
	// FilterFlag is the experiment that enables usage of the filter flag for filtering components
	FilterFlag = "filter-flag"
)

const (
	// StatusOngoing is the status of an experiment that is ongoing.
	StatusOngoing byte = iota
	// StatusCompleted is the status of an experiment that is completed.
	StatusCompleted
)

type Experiments []*Experiment

// NewExperiments returns a new Experiments map with all experiments disabled.
//
// Bottom values for each experiment are the defaults, so only the names of experiments need to be set.
func NewExperiments() Experiments {
	return Experiments{
		{
			Name: Symlinks,
		},
		{
			Name:   CLIRedesign,
			Status: StatusCompleted,
		},
		{
			Name:   Stacks,
			Status: StatusCompleted,
		},
		{
			Name: CAS,
		},
		{
			Name:   Report,
			Status: StatusCompleted,
		},
		{
			Name:   RunnerPool,
			Status: StatusCompleted,
		},
		{
			Name:   AutoProviderCacheDir,
			Status: StatusCompleted,
		},
		{
			Name: FilterFlag,
		},
	}
}

// Names returns all experiment names.
func (exps Experiments) Names() []string {
	names := []string{}

	for _, exp := range exps {
		names = append(names, exp.Name)
	}

	slices.Sort(names)

	return names
}

// FilterByStatus returns experiments filtered by the given `status`.
func (exps Experiments) FilterByStatus(status byte) Experiments {
	var found Experiments

	for _, experiment := range exps {
		if experiment.Status == status {
			found = append(found, experiment)
		}
	}

	return found
}

// Find searches and returns the experiment by the given `name`.
func (exps Experiments) Find(name string) *Experiment {
	for _, experiment := range exps {
		if experiment.Name == name {
			return experiment
		}
	}

	return nil
}

// ExperimentMode enables the experiment mode.
func (exps Experiments) ExperimentMode() {
	for _, e := range exps {
		if e.Status == StatusOngoing {
			e.Enabled = true
		}
	}
}

// EnableExperiment validates that the specified experiment name is valid and enables this experiment.
func (exps Experiments) EnableExperiment(name string) error {
	for _, e := range exps {
		if e.Name == name {
			e.Enabled = true
			return nil
		}
	}

	return NewInvalidExperimentNameError(exps.FilterByStatus(StatusOngoing).Names())
}

// NotifyCompletedExperiments logs the experiment names that are Enabled and have completed Status.
func (exps Experiments) NotifyCompletedExperiments(logger log.Logger) {
	var completed Experiments

	for _, experiment := range exps.FilterByStatus(StatusCompleted) {
		if experiment.Enabled {
			completed = append(completed, experiment)
		}
	}

	if len(completed) == 0 {
		return
	}

	logger.Warnf(NewCompletedExperimentsWarning(completed.Names()).String())
}

// Evaluate returns true if the experiment is found and enabled otherwise returns false.
func (exps Experiments) Evaluate(name string) bool {
	if experiment := exps.FilterByStatus(StatusOngoing).Find(name); experiment != nil {
		return experiment.Evaluate()
	}

	return false
}

// Experiment represents an experiment that can be enabled.
// When the experiment is enabled, Terragrunt will behave in a way that uses some experimental functionality.
type Experiment struct {
	// Name is the name of the experiment.
	Name string
	// Enabled determines if the experiment is enabled.
	Enabled bool
	// Status is the status of the experiment.
	Status byte
}

func (exps Experiment) String() string {
	return exps.Name
}

// Evaluate returns true the experiment is enabled.
//
// If the experiment is completed, consider it permanently enabled.
func (exps Experiment) Evaluate() bool {
	return exps.Enabled || exps.Status == StatusCompleted
}
