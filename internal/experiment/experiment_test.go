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

func TestOptOutAuthIsCompleted(t *testing.T) {
	t.Parallel()

	exps := experiment.NewExperiments()
	got := exps.Find(experiment.OptOutAuth)
	require.NotNil(t, got, "opt-out-auth experiment must be registered in NewExperiments()")
	assert.Equal(t, experiment.StatusCompleted, got.Status, "opt-out-auth must be completed")
	assert.True(t, got.Evaluate(), "opt-out-auth must be enabled by default")
}

func TestOptionalHooksIsOngoing(t *testing.T) {
	t.Parallel()

	exps := experiment.NewExperiments()
	got := exps.Find(experiment.OptionalHooks)
	require.NotNil(t, got, "optional-hooks experiment must be registered in NewExperiments()")
	assert.Equal(t, experiment.StatusOngoing, got.Status, "optional-hooks must be ongoing")
	assert.False(t, got.Evaluate(), "optional-hooks must be disabled by default")
}

func TestProfilingIsOngoing(t *testing.T) {
	t.Parallel()

	exps := experiment.NewExperiments()
	got := exps.Find(experiment.Profiling)
	require.NotNil(t, got, "profiling experiment must be registered in NewExperiments()")
	assert.Equal(t, experiment.StatusOngoing, got.Status, "profiling must be ongoing")
	assert.False(t, got.Evaluate(), "profiling must be disabled by default")
}

func TestVersionAttributeIsOngoing(t *testing.T) {
	t.Parallel()

	exps := experiment.NewExperiments()
	got := exps.Find(experiment.VersionAttribute)
	require.NotNil(t, got, "version-attribute experiment must be registered in NewExperiments()")
	assert.Equal(t, experiment.StatusOngoing, got.Status, "version-attribute must be ongoing")
	assert.False(t, got.Evaluate(), "version-attribute must be disabled by default")

	require.NoError(t, exps.EnableExperiment(experiment.VersionAttribute))
	assert.True(t, got.Evaluate(), "version-attribute must be enabled once explicitly requested")
}

func TestEvaluate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		experiment  string
		experiments experiment.Experiments
		want        bool
	}{
		{
			name: "ongoing disabled",
			experiments: experiment.Experiments{
				{
					Name: testOngoingA,
				},
			},
			experiment: testOngoingA,
			want:       false,
		},
		{
			name: "ongoing enabled",
			experiments: experiment.Experiments{
				{
					Name:    testOngoingA,
					Enabled: true,
				},
			},
			experiment: testOngoingA,
			want:       true,
		},
		{
			name: "completed evaluates as permanently enabled",
			experiments: experiment.Experiments{
				{
					Name:   testCompletedA,
					Status: experiment.StatusCompleted,
				},
			},
			experiment: testCompletedA,
			want:       true,
		},
		{
			name: "unknown experiment",
			experiments: experiment.Experiments{
				{
					Name: testOngoingA,
				},
			},
			experiment: "unknown",
			want:       false,
		},
		{
			name:        "promoted filter-flag experiment",
			experiments: experiment.NewExperiments(),
			experiment:  experiment.FilterFlag,
			want:        true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.experiments.Evaluate(tc.experiment))
		})
	}
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
			expectedWarning: "The following experiment(s) are already completed: " + testCompletedA +
				". Please remove any completed experiments, as setting them no longer does anything." +
				" For a list of all ongoing experiments, and the outcomes of previous experiments," +
				" see https://docs.terragrunt.com/reference/experiments",
			expectedError: nil,
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
			expectedWarning: "The following experiment(s) are already completed: " + testCompletedA +
				". Please remove any completed experiments, as setting them no longer does anything." +
				" For a list of all ongoing experiments, and the outcomes of previous experiments," +
				" see https://docs.terragrunt.com/reference/experiments",
			expectedError: experiment.NewInvalidExperimentNameError([]string{testOngoingA}),
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
