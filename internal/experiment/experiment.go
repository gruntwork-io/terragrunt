// Package experiment provides utilities used by Terragrunt to support an "experiment" mode.
// By default experiment mode is disabled, but when enabled, experimental features can be enabled.
// These features are not yet stable and may change in the future.
//
// Note that any behavior outlined here should be documented in /docs/_docs/04_reference/experiment-mode.md
//
// That is how users will know what to expect when they enable experiment mode, and how to customize it.
package experiment

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// NewExperiments returns a new Experiments map with all experiments disabled.
//
// Bottom values for each experiment are the defaults, so only the names of experiments need to be set.
func NewExperiments() Experiments {
	return Experiments{
		Symlinks: Experiment{
			Name: Symlinks,
		},
		Stacks: Experiment{
			Name: Stacks,
		},
	}
}

// Experiment represents an experiment that can be enabled.
// When the experiment is enabled, Terragrunt will behave in a way that uses some experimental functionality.
type Experiment struct {
	// Enabled determines if the experiment is enabled.
	Enabled bool
	// Name is the name of the experiment.
	Name string
	// Status is the status of the experiment.
	Status int
}

func (e Experiment) String() string {
	return e.Name
}

const (
	// Symlinks is the experiment that allows symlinks to be used in Terragrunt configurations.
	Symlinks = "symlinks"
	// Stacks is the experiment that allows stacks to be used in Terragrunt.
	Stacks = "stacks"
)

const (
	// StatusOngoing is the status of an experiment that is ongoing.
	StatusOngoing = iota
	// StatusCompleted is the status of an experiment that is completed.
	StatusCompleted
)

type Experiments map[string]Experiment

// ValidateExperimentNames validates the given slice of experiment names are valid.
func (e *Experiments) ValidateExperimentNames(experimentNames []string) (string, error) {
	completedExperiments := []string{}
	invalidExperiments := []string{}

	for _, name := range experimentNames {
		experiment, ok := (*e)[name]
		if !ok {
			invalidExperiments = append(invalidExperiments, name)
			continue
		}

		if experiment.Status == StatusCompleted {
			completedExperiments = append(completedExperiments, name)
		}
	}

	var warning string
	if len(completedExperiments) > 0 {
		warning = CompletedExperimentsWarning{
			ExperimentNames: completedExperiments,
		}.String()
	}

	var err error
	if len(invalidExperiments) > 0 {
		err = errors.New(InvalidExperimentsError{
			ExperimentNames: invalidExperiments,
		})
	}

	return warning, err
}

// EnableExperiments enables the given experiments.
func (e *Experiments) EnableExperiments(experimentNames []string) error {
	invalidExperiments := []string{}

	for _, name := range experimentNames {
		experiment, ok := (*e)[name]
		if !ok {
			invalidExperiments = append(invalidExperiments, name)
			continue
		}

		experiment.Enabled = true
		(*e)[name] = experiment
	}

	if len(invalidExperiments) > 0 {
		return errors.New(InvalidExperimentsError{
			ExperimentNames: invalidExperiments,
		})
	}

	return nil
}

// CompletedExperimentsWarning is a warning that is returned when completed experiments are requested.
type CompletedExperimentsWarning struct {
	ExperimentNames []string
}

func (e CompletedExperimentsWarning) String() string {
	return "The following experiment(s) are already completed: " + strings.Join(e.ExperimentNames, ", ") + ". Please remove any completed experiments, as setting them no longer does anything. For a list of all ongoing experiments, and the outcomes of previous experiments, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode"
}

// InvalidExperimentsError is an error that is returned when an invalid experiments are requested.
type InvalidExperimentsError struct {
	ExperimentNames []string
}

func (e InvalidExperimentsError) Error() string {
	return "The following experiment(s) are invalid: " + strings.Join(e.ExperimentNames, ", ") + ". For a list of all valid experiments, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode"
}

// Evaluate returns true if either the experiment is enabled, or experiment mode is enabled.
func (e Experiment) Evaluate(experimentMode bool) bool {
	if experimentMode {
		return true
	}

	return e.Enabled
}
