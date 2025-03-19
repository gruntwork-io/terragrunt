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
		args           []string
		expectedOutput string
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
		testCase := testCase
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
		args       []string
		expected   string
	}{
		{
			name:       "List with dependencies in tree format",
			workingDir: "fixtures/list/dag",
			args:       []string{"list", "--tree", "--dag"},
			expected: `.
├── live/dev/vpc
│   ├── live/dev/db
│   │   ╰── live/dev/ec2
│   ╰── live/dev/ec2
╰── live/prod/vpc
    ├── live/prod/db
    │   ╰── live/prod/ec2
    ╰── live/prod/ec2
`,
		},
		{
			name:       "List with dependencies in long format",
			workingDir: "fixtures/list/dag",
			args:       []string{"list", "--long", "--dependencies"},
			expected: `Type  Path           Dependencies
unit  live/dev/db    live/dev/vpc
unit  live/dev/ec2   live/dev/db, live/dev/vpc
unit  live/dev/vpc
unit  live/prod/db   live/prod/vpc
unit  live/prod/ec2  live/prod/db, live/prod/vpc
unit  live/prod/vpc
`,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
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
