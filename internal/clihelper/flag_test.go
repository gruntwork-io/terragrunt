package clihelper_test

import (
	libflag "flag"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFlagEnvVar = "TG_TEST_FLAG_ENV"

func TestFlagIgnoresProcessEnv(t *testing.T) {
	t.Setenv(testFlagEnvVar, "from-process")

	flag := &clihelper.GenericFlag[string]{Name: "foo", EnvVars: []string{testFlagEnvVar}}

	set := libflag.NewFlagSet("test-cmd", libflag.ContinueOnError)
	set.SetOutput(io.Discard)

	require.NoError(t, flag.Apply(set, map[string]string{}))

	assert.False(t, flag.Value().IsEnvSet())
	assert.Empty(t, flag.Value().Get())
}

func TestFlagPrefersSuppliedEnvOverProcessEnv(t *testing.T) {
	t.Setenv(testFlagEnvVar, "from-process")

	flag := &clihelper.GenericFlag[string]{Name: "foo", EnvVars: []string{testFlagEnvVar}}

	set := libflag.NewFlagSet("test-cmd", libflag.ContinueOnError)
	set.SetOutput(io.Discard)

	require.NoError(t, flag.Apply(set, map[string]string{testFlagEnvVar: "from-venv"}))

	assert.Equal(t, "from-venv", flag.Value().Get())
}

func TestApplyFlagPanicsOnUnsetEnv(t *testing.T) {
	t.Parallel()

	flag := &clihelper.GenericFlag[string]{Name: "foo", EnvVars: []string{testFlagEnvVar}}

	set := libflag.NewFlagSet("test-cmd", libflag.ContinueOnError)
	set.SetOutput(io.Discard)

	assert.PanicsWithError(t, clihelper.ErrEnvUnset.Error(), func() {
		require.NoError(t, flag.Apply(set, nil))
	})
}
