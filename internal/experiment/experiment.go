// Package experiment provides utilities used by Terragrunt to support an "experiment" mode.
// By default experiment mode is disabled, but when enabled, experimental features can be enabled.
// These features are not yet stable and may change in the future.
//
// Note that any behavior outlined here should be documented in /docs/_docs/04_reference/experiment-mode.md
//
// That is how users will know what to expect when they enable experiment mode, and how to customize it.
package experiment

import (
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/exp/slices"
)

const (
	// Symlinks is the experiment that allows symlinks to be used in Terragrunt configurations.
	Symlinks = "symlinks"
	// CLIRedesign is an experiment that allows users to use new commands related to the CLI redesign.
	CLIRedesign = "cli-redesign"
	// Stacks is the experiment that allows stacks to be used in Terragrunt.
	Stacks = "stacks"
	// SkipDependenciesInputs is the experiment that allows to prevent reading dependencies inputs and get performance boost.
	SkipDependenciesInputs = "skip-dependencies-inputs"
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
			Name: CLIRedesign,
		},
		{
			Name: Stacks,
		},
		{
			Name: SkipDependenciesInputs,
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

// FindByStatus returns experiments that have the given `Status`.
func (exps Experiments) FindByStatus(status byte) Experiments {
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
	for _, experiment := range exps.FindByStatus(StatusOngoing) {
		experiment.Enabled = true
	}
}

// EnableExperiment validates that the specified experiment name is valid and enables this experiment.
func (exps Experiments) EnableExperiment(name string) error {
	if experiment := exps.Find(name); experiment != nil {
		experiment.Enabled = true

		return nil
	}

	return NewInvalidExperimentNameError(exps.FindByStatus(StatusOngoing).Names())
}

// NotifyCompletedExperiments logs the experiment names that are Enabled and have completed Status.
func (exps Experiments) NotifyCompletedExperiments(logger log.Logger) {
	var completed Experiments

	for _, experiment := range exps.FindByStatus(StatusCompleted) {
		if experiment.Enabled {
			completed = append(completed, experiment)
		}
	}

	if len(completed) == 0 {
		return
	}

	logger.Warnf(NewCompletedExperimentsError(completed.Names()).Error())
}

// Evaluate returns true if the experiment is found and enabled otherwise returns false.
func (exps Experiments) Evaluate(name string) bool {
	if experiment := exps.FindByStatus(StatusOngoing).Find(name); experiment != nil {
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
func (exps Experiment) Evaluate() bool {
	return exps.Enabled
}
