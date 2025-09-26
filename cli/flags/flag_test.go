package flags_test

import (
	"bytes"
	"flag"
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockDestValue[T any](val T) *T {
	return &val
}

func newLogger() (log.Logger, *bytes.Buffer) {
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})
	output := new(bytes.Buffer)
	logger := log.New(log.WithOutput(output), log.WithLevel(log.InfoLevel), log.WithFormatter(formatter))

	return logger, output
}

func TestFlag_TakesValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flag     cli.Flag
		expected bool
	}{
		{
			&cli.BoolFlag{Name: "name", Destination: mockDestValue(false)},
			true,
		},
		{
			&cli.BoolFlag{Name: "name", Destination: mockDestValue(true)},
			false,
		},
		{
			&cli.BoolFlag{Name: "name", Negative: true, Destination: mockDestValue(true)},
			true,
		},
		{
			&cli.BoolFlag{Name: "name", Negative: true, Destination: mockDestValue(false)},
			false,
		},
		{
			&cli.GenericFlag[string]{Name: "name", Destination: mockDestValue("value")},
			true,
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testFlag := flags.NewFlag(tc.flag)

			err := testFlag.Apply(new(flag.FlagSet))
			require.NoError(t, err)

			assert.Equal(t, tc.expected, testFlag.TakesValue())
		})
	}
}

func TestFlag_Evaluate(t *testing.T) {
	t.Parallel()

	mockRegControls := func(flagNameControl, envVarControl strict.Control) bool {
		return true
	}

	deprecatedFlagWarning := func() string {
		return controls.NewDeprecatedFlagName(&cli.BoolFlag{}, &cli.BoolFlag{}, "").WarningFmt
	}

	deprecatedEnvVarWarning := func() string {
		return controls.NewDeprecatedEnvVar(&cli.BoolFlag{}, &cli.BoolFlag{}, "").WarningFmt
	}

	type testCaseFlag struct {
		flag   *flags.Flag
		arg    string
		envVar string
	}

	testCases := []struct {
		flags          []testCaseFlag
		expectedOutput []string
	}{

		{
			[]testCaseFlag{
				{
					flags.NewFlag(
						&cli.BoolFlag{Name: "new-flag-name"},
						flags.WithDeprecatedName("old-flag-name", mockRegControls),
					),
					"old-flag-name",
					"",
				},
				{
					flags.NewFlag(
						&cli.BoolFlag{Name: "new-env-var-name", EnvVars: []string{"NEW_ENV_VAR_NAME"}},
						flags.WithDeprecatedName("old-env-var-name", mockRegControls),
					),
					"",
					"OLD_ENV_VAR_NAME",
				},
			},
			[]string{
				fmt.Sprintf(deprecatedFlagWarning(), "old-flag-name", "new-flag-name"),
				fmt.Sprintf(deprecatedEnvVarWarning(), "OLD_ENV_VAR_NAME", "NEW_ENV_VAR_NAME=true"),
			},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			logger, output := newLogger()
			ctx := t.Context()
			ctx = log.ContextWithLogger(ctx, logger)

			for _, testFlag := range tc.flags {
				err := testFlag.flag.Apply(new(flag.FlagSet))
				require.NoError(t, err)

				if testFlag.arg != "" {
					err := testFlag.flag.Value().Getter(testFlag.arg).Set("1")
					require.NoError(t, err)
				}

				if testFlag.envVar != "" {
					err := testFlag.flag.Value().Getter(testFlag.envVar).EnvSet("1")
					require.NoError(t, err)
				}

				err = testFlag.flag.RunAction(cli.NewAppContext(ctx, nil, nil))
				require.NoError(t, err)
			}

			outputLines := strings.Split(strings.TrimSpace(output.String()), "\n")
			assert.Equal(t, tc.expectedOutput, outputLines)
		})
	}
}
