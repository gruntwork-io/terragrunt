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
			name:       "List with JSON format",
			workingDir: "fixtures/list/basic",
			args:       []string{"list", "--json"},
			expectedOutput: `[
  {
    "path": "a-unit",
    "type": "unit"
  },
  {
    "path": "b-unit",
    "type": "unit"
  }
]
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
			name:       "List with dependencies in JSON format",
			workingDir: "fixtures/list",
			args:       []string{"list", "--json", "--dependencies"},
			expected: `[
  {
    "path": "basic",
    "children": [
      {
        "path": "a-unit",
        "type": "unit"
      },
      {
        "path": "b-unit",
        "type": "unit"
      }
    ]
  },
  {
    "path": "tree",
    "children": [
      {
        "path": "L1",
        "type": "unit",
        "children": [
          {
            "path": "L2",
            "type": "unit",
            "children": [
              {
                "path": "L3",
                "type": "unit"
              },
              {
                "path": "L3-a",
                "type": "unit",
                "children": [
                  {
                    "path": "L4",
                    "type": "unit"
                  }
                ]
              }
            ]
          }
        ]
      },
      {
        "path": "child1",
        "type": "unit"
      },
      {
        "path": "child2",
        "type": "unit"
      },
      {
        "path": "grandchild1",
        "type": "unit"
      },
      {
        "path": "grandchild2",
        "type": "unit"
      }
    ]
  }
]
`,
		},
		{
			name:       "List with dependencies in tree format",
			workingDir: "fixtures/list",
			args:       []string{"list", "--tree", "--dag"},
			expected: `.
├── basic/a-unit
├── basic/b-unit
├── tree/L1/L2/L3
├── tree/L1/L2/L3-a/L4
├── tree/grandchild1
│   ╰── tree/child1
╰── tree/grandchild2
    ╰── tree/child2
`,
		},
		{
			name:       "List with dependencies in long format",
			workingDir: "fixtures/list",
			args:       []string{"list", "--long", "--dependencies"},
			expected: `Type  Path                Dependencies
unit  basic/a-unit
unit  basic/b-unit
unit  tree/L1/L2/L3
unit  tree/L1/L2/L3-a/L4
unit  tree/child1         tree/grandchild1
unit  tree/child2         tree/grandchild2
unit  tree/grandchild1
unit  tree/grandchild2
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
