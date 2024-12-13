package experiment_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/stretchr/testify/assert"
)

func TestValidateExperiments(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name            string
		experiments     experiment.Experiments
		experimentNames []string
		expectedWarning string
		expectedError   error
	}{
		{
			name:            "no experiments",
			experiments:     experiment.NewExperiments(),
			experimentNames: []string{},
			expectedWarning: "",
			expectedError:   nil,
		},
		{
			name:            "valid experiment",
			experiments:     experiment.NewExperiments(),
			experimentNames: []string{experiment.Symlinks},
			expectedWarning: "",
			expectedError:   nil,
		},
		{
			name:            "invalid experiment",
			experiments:     experiment.NewExperiments(),
			experimentNames: []string{"invalid"},
			expectedWarning: "",
			expectedError: experiment.InvalidExperimentsError{
				ExperimentNames: []string{"invalid"},
			},
		},
		{
			name: "completed experiment",
			experiments: experiment.Experiments{
				experiment.Symlinks: experiment.Experiment{
					Name:   experiment.Symlinks,
					Status: experiment.StatusCompleted,
				},
			},
			experimentNames: []string{experiment.Symlinks},
			expectedWarning: "The following experiment(s) are already completed: symlinks. Please remove this experiment flag, as it no longer does anything. For a list of all ongoing experiments, and the outcomes of previous experiments, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode",
			expectedError:   nil,
		},
		{
			name: "invalid and completed experiment",
			experiments: experiment.Experiments{
				experiment.Symlinks: experiment.Experiment{
					Name:   experiment.Symlinks,
					Status: experiment.StatusCompleted,
				},
			},
			experimentNames: []string{"invalid", experiment.Symlinks},
			expectedWarning: "The following experiment(s) are already completed: symlinks. Please remove this experiment flag, as it no longer does anything. For a list of all ongoing experiments, and the outcomes of previous experiments, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode",
			expectedError: experiment.InvalidExperimentsError{
				ExperimentNames: []string{"invalid"},
			},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			warning, err := tt.experiments.ValidateExperimentNames(tt.experimentNames)

			assert.Equal(t, tt.expectedWarning, warning)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}
