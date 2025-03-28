package experiment_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testOngoingA   = "test-ongoing-a"
	testOngoingB   = "test-ongoing-b"
	testCompletedA = "test-completed-a"
)

func newTestLogger() (log.Logger, *bytes.Buffer) {
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})
	output := new(bytes.Buffer)
	logger := log.New(log.WithOutput(output), log.WithLevel(log.InfoLevel), log.WithFormatter(formatter))

	return logger, output
}

func TestValidateExperiments(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedError   error
		name            string
		expectedWarning string
		experiments     experiment.Experiments
		experimentNames []string
	}{
		{
			name: "no experiments",
			experiments: experiment.Experiments{
				{
					Name:   testOngoingA,
					Status: experiment.StatusCompleted,
				},
				{
					Name: experiment.CLIRedesign,
				},
			},
			experimentNames: []string{},
			expectedWarning: "",
			expectedError:   nil,
		},
		{
			name: "valid experiment",
			experiments: experiment.Experiments{
				{
					Name: testOngoingA,
				},
				{
					Name: testOngoingB,
				},
			},
			experimentNames: []string{testOngoingA},
			expectedWarning: "",
			expectedError:   nil,
		},
		{
			name: "invalid experiment",
			experiments: experiment.Experiments{
				{
					Name:   testCompletedA,
					Status: experiment.StatusCompleted,
				},
				{
					Name: testOngoingA,
				},
			},
			experimentNames: []string{"invalid"},
			expectedWarning: "",
			expectedError:   experiment.NewInvalidExperimentNameError([]string{testOngoingA}),
		},
		{
			name: "completed experiment",
			experiments: experiment.Experiments{
				{
					Name:   testCompletedA,
					Status: experiment.StatusCompleted,
				},
			},
			experimentNames: []string{testCompletedA},
			expectedWarning: "The following experiment(s) are already completed: " + testCompletedA + ". Please remove any completed experiments, as setting them no longer does anything. For a list of all ongoing experiments, and the outcomes of previous experiments, see https://terragrunt.gruntwork.io/docs/reference/experiments",
			expectedError:   nil,
		},
		{
			name: "invalid and completed experiment",
			experiments: experiment.Experiments{
				{
					Name:   testCompletedA,
					Status: experiment.StatusCompleted,
				},
				{
					Name: testOngoingA,
				},
			},
			experimentNames: []string{"invalid", testCompletedA},
			expectedWarning: "The following experiment(s) are already completed: " + testCompletedA + ". Please remove any completed experiments, as setting them no longer does anything. For a list of all ongoing experiments, and the outcomes of previous experiments, see https://terragrunt.gruntwork.io/docs/reference/experiments",
			expectedError:   experiment.NewInvalidExperimentNameError([]string{testOngoingA}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, name := range tc.experimentNames {
				if err := tc.experiments.EnableExperiment(name); err != nil {
					require.EqualError(t, err, tc.expectedError.Error())
				} else {
					require.NoError(t, err)
				}
			}

			logger, output := newTestLogger()

			tc.experiments.NotifyCompletedExperiments(logger)

			if tc.expectedWarning == "" {
				assert.Empty(t, output.String())

				return
			}

			assert.Contains(t, strings.TrimSpace(output.String()), tc.expectedWarning)
		})
	}
}
