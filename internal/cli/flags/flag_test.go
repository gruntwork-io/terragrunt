package flags_test

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLogger() (log.Logger, *bytes.Buffer) {
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})
	output := new(bytes.Buffer)
	logger := log.New(
		log.WithOutput(output),
		log.WithLevel(log.InfoLevel),
		log.WithFormatter(formatter),
	)

	return logger, output
}

func TestFlag_TakesValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flag     clihelper.Flag
		expected bool
	}{
		{
			&clihelper.BoolFlag{Name: "name", Destination: new(false)},
			true,
		},
		{
			&clihelper.BoolFlag{Name: "name", Destination: new(true)},
			false,
		},
		{
			&clihelper.BoolFlag{Name: "name", Negative: true, Destination: new(true)},
			true,
		},
		{
			&clihelper.BoolFlag{Name: "name", Negative: true, Destination: new(false)},
			false,
		},
		{
			&clihelper.GenericFlag[string]{Name: "name", Destination: new("value")},
			true,
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			testFlag := flags.NewFlag(tc.flag)

			err := testFlag.Apply(new(flag.FlagSet), map[string]string{})
			require.NoError(t, err)

			assert.Equal(t, tc.expected, testFlag.TakesValue())
		})
	}
}

// newDeprecatedAliasFlag mirrors the shape of `--no-auto-init`: a negative bool flag
// whose deprecated alias is a separate flag carrying the opposite sense.
func newDeprecatedAliasFlag(dest *bool) *flags.Flag {
	return flags.NewFlag(
		&clihelper.BoolFlag{
			Name:        "no-auto-init",
			Negative:    true,
			Destination: dest,
		},
		flags.WithDeprecatedFlag(&clihelper.BoolFlag{
			EnvVars: []string{"TERRAGRUNT_AUTO_INIT"},
		}, nil, strict.Controls{}),
	)
}

// TestFlag_ValueCarriesDeprecatedAlias pins that a value given only by a deprecated
// alias reaches the flag, and that reading the flag repeatedly does not change it.
func TestFlag_ValueCarriesDeprecatedAlias(t *testing.T) {
	t.Parallel()

	dest := new(true)
	testFlag := newDeprecatedAliasFlag(dest)

	require.NoError(t, testFlag.Parse(nil, map[string]string{"TERRAGRUNT_AUTO_INIT": "false"}))

	assert.Equal(t, false, testFlag.Value().Get())
	assert.Equal(t, false, testFlag.Value().Get(), "reading the flag again must not change its value")
	assert.False(t, *dest)
}

// TestFlag_ValueKeepsExplicitOverDeprecatedAlias pins that a value given under the
// flag's current name wins over the same setting given by a deprecated alias.
func TestFlag_ValueKeepsExplicitOverDeprecatedAlias(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		env      map[string]string
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "flag turns auto-init off, alias turns it on",
			args:     []string{"--no-auto-init"},
			env:      map[string]string{"TERRAGRUNT_AUTO_INIT": "true"},
			expected: false,
		},
		{
			name:     "flag turns auto-init on, alias turns it off",
			args:     []string{"--no-auto-init=false"},
			env:      map[string]string{"TERRAGRUNT_AUTO_INIT": "false"},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dest := new(true)
			testFlag := newDeprecatedAliasFlag(dest)

			require.NoError(t, testFlag.Parse(tc.args, tc.env))

			assert.Equal(t, tc.expected, testFlag.Value().Get())
			assert.Equal(t, tc.expected, *dest)
		})
	}
}

func TestFlag_Evaluate(t *testing.T) {
	t.Parallel()

	// A non-nil (even empty) Controls is enough to trigger registration in
	// DeprecatedFlag.SetStrictControls; the umbrella parents simply aren't
	// present to add subcontrols under.
	mockStrictControls := strict.Controls{}

	deprecatedFlagWarning := func() string {
		return controls.NewDeprecatedFlagName(
			&clihelper.BoolFlag{},
			&clihelper.BoolFlag{},
			"",
		).WarningFmt
	}

	deprecatedEnvVarWarning := func() string {
		return controls.NewDeprecatedEnvVar(
			&clihelper.BoolFlag{},
			&clihelper.BoolFlag{},
			"",
		).WarningFmt
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
						&clihelper.BoolFlag{Name: "new-flag-name"},
						flags.WithDeprecatedName("old-flag-name", mockStrictControls),
					),
					"old-flag-name",
					"",
				},
				{
					flags.NewFlag(
						&clihelper.BoolFlag{
							Name:    "new-env-var-name",
							EnvVars: []string{"NEW_ENV_VAR_NAME"},
						},
						flags.WithDeprecatedName("old-env-var-name", mockStrictControls),
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
				err := testFlag.flag.Apply(new(flag.FlagSet), map[string]string{})
				require.NoError(t, err)

				if testFlag.arg != "" {
					err := testFlag.flag.Value().Getter(testFlag.arg).Set("1")
					require.NoError(t, err)
				}

				if testFlag.envVar != "" {
					err := testFlag.flag.Value().Getter(testFlag.envVar).EnvSet("1")
					require.NoError(t, err)
				}

				err = testFlag.flag.RunAction(ctx, clihelper.NewAppContext(nil, nil))
				require.NoError(t, err)
			}

			outputLines := strings.Split(strings.TrimSpace(output.String()), "\n")
			assert.Equal(t, tc.expectedOutput, outputLines)
		})
	}
}

func TestFlag_Parse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		expected    string
		args        clihelper.Args
		expectedErr bool
	}{
		{
			name:     "accepted value",
			args:     clihelper.Args{"--level", "debug"},
			expected: "debug",
		},
		{
			name:        "value rejected by the flag setter",
			args:        clihelper.Args{"--level", "bogus"},
			expectedErr: true,
		},
		{
			name:        "value missing",
			args:        clihelper.Args{"--level"},
			expectedErr: true,
		},
		{
			name: "flag belonging to another parser",
			args: clihelper.Args{"--some-other-flag"},
		},
		{
			name:     "value after a flag belonging to another parser",
			args:     clihelper.Args{"--some-other-flag", "--level", "debug"},
			expected: "debug",
		},
		{
			name:        "rejected value after a flag belonging to another parser",
			args:        clihelper.Args{"--some-other-flag", "--level", "bogus"},
			expectedErr: true,
		},
		{
			name:     "value after --help",
			args:     clihelper.Args{"--help", "--level", "debug"},
			expected: "debug",
		},
		{
			name: "-h alone",
			args: clihelper.Args{"-h"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got string

			testFlag := flags.NewFlag(&clihelper.GenericFlag[string]{
				Name: "level",
				Setter: func(val string) error {
					if val == "bogus" {
						return errors.New("unsupported level")
					}

					got = val

					return nil
				},
			})

			err := testFlag.Parse(tc.args, map[string]string{})

			if tc.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expected, got)
		})
	}
}
