package test_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureListBasic = "fixtures/list/basic"
	testFixtureListDag   = "fixtures/list/dag"
)

func TestListCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                      string
		workingDir                string
		expectedOutput            string
		args                      []string
		unnecessaryExperimentFlag bool
	}{
		{
			name:           "Basic list with default format",
			workingDir:     testFixtureListBasic,
			args:           []string{"list"},
			expectedOutput: "a-unit  b-unit  \n",
		},
		{
			name:       "List with long format",
			workingDir: testFixtureListBasic,
			args:       []string{"list", "--long"},
			expectedOutput: `Type  Path
unit  a-unit
unit  b-unit
`,
		},
		{
			name:       "List with tree format",
			workingDir: testFixtureListBasic,
			args:       []string{"list", "--tree"},
			expectedOutput: `.
├── a-unit
╰── b-unit
`,
		},
		{
			name:           "Basic list with default format",
			workingDir:     testFixtureListBasic,
			args:           []string{"list"},
			expectedOutput: "a-unit  b-unit  \n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, tc.workingDir)

			args := []string{"terragrunt", "--no-color"}
			if tc.unnecessaryExperimentFlag {
				args = append(args, "--experiment", "cli-redesign")
			}

			args = append(args, tc.args...)
			args = append(args, "--working-dir", tc.workingDir)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, strings.Join(args, " "))

			require.NoError(t, err)

			if tc.unnecessaryExperimentFlag {
				require.Contains(t, stderr, "The following experiment(s) are already completed: cli-redesign. Please remove any completed experiments, as setting them no longer does anything. For a list of all ongoing experiments, and the outcomes of previous experiments, see https://terragrunt.gruntwork.io/docs/reference/experiments")
			} else {
				require.Empty(t, stderr)
			}

			assert.Equal(t, tc.expectedOutput, stdout)
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
			workingDir: testFixtureListDag,
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
			workingDir: testFixtureListDag,
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, tc.workingDir)

			args := []string{"terragrunt", "--no-color"}
			args = append(args, tc.args...)
			args = append(args, "--working-dir", tc.workingDir)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, strings.Join(args, " "))
			require.NoError(t, err)
			require.Empty(t, stderr)

			assert.Equal(t, tc.expected, stdout)
		})
	}
}

func TestListCommandWithExclude(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		expectedOutput string
		args           []string
	}{
		{
			name:           "List with queue-construct-as plan",
			args:           []string{"list", "--queue-construct-as", "plan"},
			expectedOutput: "unit2  unit3  \n",
		},
		{
			name:           "List with queue-construct-as apply",
			args:           []string{"list", "--queue-construct-as", "apply"},
			expectedOutput: "unit1  unit3  \n",
		},
		{
			name: "List with queue-construct-as plan in long format",
			args: []string{"list", "--queue-construct-as", "plan", "--long"},
			expectedOutput: `Type  Path
unit  unit2
unit  unit3
`,
		},
		{
			name: "List with queue-construct-as apply in tree format",
			args: []string{"list", "--queue-construct-as", "apply", "--tree"},
			expectedOutput: `.
├── unit1
╰── unit3
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureFindExclude)

			args := []string{"terragrunt", "--no-color"}
			args = append(args, tc.args...)
			args = append(args, "--working-dir", testFixtureFindExclude)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, strings.Join(args, " "))
			require.NoError(t, err)
			require.Empty(t, stderr)
			assert.Equal(t, tc.expectedOutput, stdout)
		})
	}
}
