package test_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
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
		cmd := fmt.Sprintf("terragrunt %s --non-interactive --log-level trace --working-dir %s", strings.Join(tc.command, " "), testFixtureExtraArgsPath)

		var (
			stdout bytes.Buffer
			stderr bytes.Buffer
		)
		// Call helpers.RunTerragruntCommand directly because this command contains failures (which causes helpers.RunTerragruntRedirectOutput to abort) but we don't care.
		if err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr); err == nil {
			t.Fatalf("Failed to properly fail command: %v.", cmd)
		}

		output := stdout.String()
		errOutput := stderr.String()
		assert.True(t, strings.Contains(errOutput, tc.expected) || strings.Contains(output, tc.expected))
	}
}
