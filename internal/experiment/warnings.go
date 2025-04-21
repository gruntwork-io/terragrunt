package experiment

import (
	"strings"
)

// CompletedExperimentsWarning is a warning that is returned when completed experiments are requested.
type CompletedExperimentsWarning struct {
	experimentsNames []string
}

func NewCompletedExperimentsWarning(experimentsNames []string) *CompletedExperimentsWarning {
	return &CompletedExperimentsWarning{
		experimentsNames: experimentsNames,
	}
}

func (w CompletedExperimentsWarning) String() string {
	return "The following experiment(s) are already completed: " + strings.Join(w.experimentsNames, ", ") + ". Please remove any completed experiments, as setting them no longer does anything. For a list of all ongoing experiments, and the outcomes of previous experiments, see https://terragrunt.gruntwork.io/docs/reference/experiments"
}
