package test_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		workingDir     string
		expectedOutput string
		args           []string
	}{
		{
			name:           "Basic list with default format",
			workingDir:     "fixtures/list/basic",
			args:           []string{"list"},
			expectedOutput: "a-unit  b-unit  \n",
		},
		{
			name:       "List with long format",
			workingDir: "fixtures/list/basic",
			args:       []string{"list", "--long"},
			expectedOutput: `Type  Path
unit  a-unit
unit  b-unit
`,
		},
		{
			name:       "List with tree format",
			workingDir: "fixtures/list/basic",
			args:       []string{"list", "--tree"},
			expectedOutput: `.
├── a-unit
╰── b-unit
`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testCase.workingDir)

			args := []string{"terragrunt", "--no-color", "--experiment", "cli-redesign"}
			args = append(args, testCase.args...)
			args = append(args, "--working-dir", testCase.workingDir)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, strings.Join(args, " "))

			require.NoError(t, err)
			require.Empty(t, stderr)
			assert.Equal(t, testCase.expectedOutput, stdout)
		})
	}
}

func TestListCommandWithDependencies(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		workingDir string
		expected   string
		args       []string
	}{
		{
			name:       "List with dependencies in tree format",
			workingDir: "fixtures/list/dag",
			args:       []string{"list", "--tree", "--dag"},
			expected: `.
├── stacks/live/dev
├── stacks/live/prod
├── units/live/dev/vpc
│   ├── units/live/dev/db
│   │   ╰── units/live/dev/ec2
│   ╰── units/live/dev/ec2
╰── units/live/prod/vpc
    ├── units/live/prod/db
    │   ╰── units/live/prod/ec2
    ╰── units/live/prod/ec2
`,
		},
		{
			name:       "List with dependencies in long format",
			workingDir: "fixtures/list/dag",
			args:       []string{"list", "--long", "--dependencies"},
			expected: `Type  Path                 Dependencies
stack stacks/live/dev
stack stacks/live/prod
unit  units/live/dev/db    units/live/dev/vpc
unit  units/live/dev/ec2   units/live/dev/db, units/live/dev/vpc
unit  units/live/dev/vpc
unit  units/live/prod/db   units/live/prod/vpc
unit  units/live/prod/ec2  units/live/prod/db, units/live/prod/vpc
unit  units/live/prod/vpc
`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testCase.workingDir)

			args := []string{"terragrunt", "--no-color", "--experiment", "cli-redesign"}
			args = append(args, testCase.args...)
			args = append(args, "--working-dir", testCase.workingDir)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, strings.Join(args, " "))
			require.NoError(t, err)
			require.Empty(t, stderr)

			assert.Equal(t, testCase.expected, stdout)
		})
	}
}
