package clihelper_test

import (
	"context"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	urfaveCli "github.com/urfave/cli/v2"
)

// newFlagDefaults returns a FlagDefaultsFunc that answers with the given values for the
// given flag name at the given command path, and records the paths it was asked about.
func newFlagDefaults(
	cmdPath [][]string,
	flagName string,
	values []string,
	seen *[][][]string,
) clihelper.FlagDefaultsFunc {
	return func(actualPath [][]string, flag clihelper.Flag) ([]string, bool) {
		*seen = append(*seen, actualPath)

		if !assert.ObjectsAreEqual(cmdPath, actualPath) {
			return nil, false
		}

		for _, name := range flag.Names() {
			if name == flagName {
				return values, true
			}
		}

		return nil, false
	}
}

func TestApplyFlagDefaults(t *testing.T) {
	t.Parallel()

	var (
		destination string
		seen        [][][]string
	)

	app := &clihelper.App{
		App:          &urfaveCli.App{Writer: io.Discard},
		FlagDefaults: newFlagDefaults(nil, "foo", []string{"from-rc"}, &seen),
	}

	cmd := clihelper.Command{
		Name:   "terragrunt",
		IsRoot: true,
		Flags: clihelper.Flags{
			&clihelper.GenericFlag[string]{Name: "foo", Destination: &destination},
		},
	}

	require.NoError(t, cmd.Run(t.Context(), clihelper.NewAppContext(app, nil), nil))
	assert.Equal(t, "from-rc", destination)
	assert.Equal(t, [][][]string{nil}, seen, "the root command owns the global flags")

	value := cmd.Flags.Get("foo").Value()
	assert.True(t, value.IsSet())
	assert.True(t, value.IsDefaultSet())
	assert.False(t, value.IsArgSet(), "a default must not look like a command line argument")
	assert.False(t, value.IsEnvSet(), "a default must not look like an environment variable")
}

func TestApplyFlagDefaultsIsOverriddenByArg(t *testing.T) {
	t.Parallel()

	var (
		destination string
		seen        [][][]string
	)

	app := &clihelper.App{
		App:          &urfaveCli.App{Writer: io.Discard},
		FlagDefaults: newFlagDefaults(nil, "foo", []string{"from-rc"}, &seen),
	}

	cmd := clihelper.Command{
		Name:   "terragrunt",
		IsRoot: true,
		Flags: clihelper.Flags{
			&clihelper.GenericFlag[string]{Name: "foo", Destination: &destination},
		},
	}

	args := []string{"--foo", "from-arg"}
	require.NoError(t, cmd.Run(t.Context(), clihelper.NewAppContext(app, args), args))
	assert.Equal(t, "from-arg", destination)
}

//nolint:paralleltest // sets an environment variable the flag reads.
func TestApplyFlagDefaultsIsOverriddenByEnvVar(t *testing.T) {
	t.Setenv("TG_CLIHELPER_TEST_FOO", "from-env")

	var (
		destination string
		seen        [][][]string
	)

	app := &clihelper.App{
		App:          &urfaveCli.App{Writer: io.Discard},
		FlagDefaults: newFlagDefaults(nil, "foo", []string{"from-rc"}, &seen),
	}

	cmd := clihelper.Command{
		Name:   "terragrunt",
		IsRoot: true,
		Flags: clihelper.Flags{
			&clihelper.GenericFlag[string]{
				Name:        "foo",
				EnvVars:     []string{"TG_CLIHELPER_TEST_FOO"},
				Destination: &destination,
			},
		},
	}

	require.NoError(t, cmd.Run(t.Context(), clihelper.NewAppContext(app, nil), nil))
	assert.Equal(t, "from-env", destination)
}

func TestApplyFlagDefaultsRunsTheFlagAction(t *testing.T) {
	t.Parallel()

	var (
		actionValue string
		seen        [][][]string
	)

	app := &clihelper.App{
		App:          &urfaveCli.App{Writer: io.Discard},
		FlagDefaults: newFlagDefaults(nil, "foo", []string{"from-rc"}, &seen),
	}

	cmd := clihelper.Command{
		Name:   "terragrunt",
		IsRoot: true,
		Flags: clihelper.Flags{
			&clihelper.GenericFlag[string]{
				Name: "foo",
				Action: func(_ context.Context, _ *clihelper.Context, value string) error {
					actionValue = value

					return nil
				},
			},
		},
	}

	require.NoError(t, cmd.Run(t.Context(), clihelper.NewAppContext(app, nil), nil))
	assert.Equal(t, "from-rc", actionValue, "a value from the defaults marks the flag as set")
}

func TestApplyFlagDefaultsForSubcommand(t *testing.T) {
	t.Parallel()

	var (
		rootValue string
		subValue  []string
		seen      [][][]string
	)

	app := &clihelper.App{
		App: &urfaveCli.App{Writer: io.Discard},
		FlagDefaults: newFlagDefaults(
			[][]string{{"hcl"}, {"fmt"}},
			"bar",
			[]string{"one", "two"},
			&seen,
		),
	}

	cmd := clihelper.Command{
		Name:   "terragrunt",
		IsRoot: true,
		Flags: clihelper.Flags{
			&clihelper.GenericFlag[string]{Name: "foo", Destination: &rootValue},
		},
		Subcommands: clihelper.Commands{
			&clihelper.Command{
				Name: "hcl",
				Subcommands: clihelper.Commands{
					&clihelper.Command{
						Name: "fmt",
						Flags: clihelper.Flags{
							&clihelper.SliceFlag[string]{Name: "bar", Destination: &subValue},
						},
					},
				},
			},
		},
	}

	args := []string{"hcl", "fmt"}
	require.NoError(t, cmd.Run(t.Context(), clihelper.NewAppContext(app, args), args))

	assert.Empty(t, rootValue, "a subcommand default must not reach a global flag")
	assert.Equal(t, []string{"one", "two"}, subValue)
	// The intermediate `hcl` command declares no flags, so nothing is looked up for it.
	assert.Equal(t, [][][]string{nil, {{"hcl"}, {"fmt"}}}, seen)
}

func TestApplyFlagDefaultsInvalidValue(t *testing.T) {
	t.Parallel()

	var seen [][][]string

	app := &clihelper.App{
		App:          &urfaveCli.App{Writer: io.Discard},
		FlagDefaults: newFlagDefaults(nil, "foo", []string{"not-a-bool"}, &seen),
	}

	cmd := clihelper.Command{
		Name:   "terragrunt",
		IsRoot: true,
		Flags:  clihelper.Flags{&clihelper.BoolFlag{Name: "foo"}},
	}

	err := cmd.Run(t.Context(), clihelper.NewAppContext(app, nil), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid value "not-a-bool" for flag foo`)
}

func TestApplyFlagDefaultsWithoutSource(t *testing.T) {
	t.Parallel()

	var destination string

	app := &clihelper.App{App: &urfaveCli.App{Writer: io.Discard}}

	cmd := clihelper.Command{
		Name:   "terragrunt",
		IsRoot: true,
		Flags: clihelper.Flags{
			&clihelper.GenericFlag[string]{Name: "foo", Destination: &destination},
		},
	}

	require.NoError(t, cmd.Run(t.Context(), clihelper.NewAppContext(app, nil), nil))
	assert.Empty(t, destination)
}
