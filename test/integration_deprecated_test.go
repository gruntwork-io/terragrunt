package test_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This tests terragrunt properly passes through terraform commands with sub commands
// and any number of specified args
func TestDeprecatedDefaultCommand_TerraformSubcommandCliArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expected string
		command  []string
	}{
		{
			command:  []string{"force-unlock"},
			expected: wrappedBinary() + " force-unlock",
		},
		{
			command:  []string{"force-unlock", "foo"},
			expected: wrappedBinary() + " force-unlock foo",
		},
		{
			command:  []string{"force-unlock", "foo", "bar", "baz"},
			expected: wrappedBinary() + " force-unlock foo bar baz",
		},
		{
			command:  []string{"force-unlock", "foo", "bar", "baz", "foobar"},
			expected: wrappedBinary() + " force-unlock foo bar baz foobar",
		},
	}

	for _, tc := range testCases {
		tofuCmd := strings.Join(tc.command, " ")

		t.Run(tofuCmd, func(t *testing.T) {
			t.Parallel()

			cmd := fmt.Sprintf(
				"terragrunt --log-level debug --non-interactive --working-dir %s -- %s",
				testFixtureExtraArgsPath,
				strings.Join(tc.command, " "),
			)

			// Call helpers.RunTerragruntCommand directly because this command
			// contains failures (which causes helpers.RunTerragruntRedirectOutput to abort) but we don't care.
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.Error(t, err)

			assert.True(t, strings.Contains(stderr, tc.expected) || strings.Contains(stdout, tc.expected))
		})
	}
}
