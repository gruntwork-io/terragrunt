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
			workingDir: "fixtures/list/dag",
			args:       []string{"list", "--json", "--dependencies"},
			expected: `[
  {
    "path": "live",
    "children": [
      {
        "path": "dev",
        "type": "unit",
        "children": [
          {
            "path": "live/dev/db",
            "type": "unit",
            "dependencies": [
              {
                "path": "live/dev/vpc",
                "type": "unit"
              }
            ]
          },
          {
            "path": "live/dev/ec2",
            "type": "unit",
            "dependencies": [
              {
                "path": "live/dev/vpc",
                "type": "unit"
              },
              {
                "path": "live/dev/db",
                "type": "unit",
                "dependencies": [
                  {
                    "path": "live/dev/vpc",
                    "type": "unit"
                  }
                ]
              }
            ]
          },
          {
            "path": "live/dev/vpc",
            "type": "unit"
          }
        ]
      },
      {
        "path": "prod",
        "type": "unit",
        "children": [
          {
            "path": "live/prod/db",
            "type": "unit",
            "dependencies": [
              {
                "path": "live/prod/vpc",
                "type": "unit"
              }
            ]
          },
          {
            "path": "live/prod/ec2",
            "type": "unit",
            "dependencies": [
              {
                "path": "live/prod/vpc",
                "type": "unit"
              },
              {
                "path": "live/prod/db",
                "type": "unit",
                "dependencies": [
                  {
                    "path": "live/prod/vpc",
                    "type": "unit"
                  }
                ]
              }
            ]
          },
          {
            "path": "live/prod/vpc",
            "type": "unit"
          }
        ]
      },
      {
        "path": "stage",
        "type": "unit",
        "children": [
          {
            "path": "live/stage/db",
            "type": "unit",
            "dependencies": [
              {
                "path": "live/stage/vpc",
                "type": "unit"
              }
            ]
          },
          {
            "path": "live/stage/ec2",
            "type": "unit",
            "dependencies": [
              {
                "path": "live/stage/vpc",
                "type": "unit"
              },
              {
                "path": "live/stage/db",
                "type": "unit",
                "dependencies": [
                  {
                    "path": "live/stage/vpc",
                    "type": "unit"
                  }
                ]
              }
            ]
          },
          {
            "path": "live/stage/vpc",
            "type": "unit"
          }
        ]
      }
    ]
  }
]
`,
		},
		{
			name:       "List with dependencies in tree format",
			workingDir: "fixtures/list/dag",
			args:       []string{"list", "--tree", "--dag"},
			expected: `.
├── live/dev/vpc
│   ├── live/dev/db
│   │   ╰── live/dev/ec2
│   ╰── live/dev/ec2
├── live/prod/vpc
│   ├── live/prod/db
│   │   ╰── live/prod/ec2
│   ╰── live/prod/ec2
╰── live/stage/vpc
    ├── live/stage/db
    │   ╰── live/stage/ec2
    ╰── live/stage/ec2
`,
		},
		{
			name:       "List with dependencies in long format",
			workingDir: "fixtures/list/dag",
			args:       []string{"list", "--long", "--dependencies"},
			expected: `Type  Path            Dependencies
unit  live/dev/db     live/dev/vpc
unit  live/dev/ec2    live/dev/vpc, live/dev/db
unit  live/dev/vpc
unit  live/prod/db    live/prod/vpc
unit  live/prod/ec2   live/prod/vpc, live/prod/db
unit  live/prod/vpc
unit  live/stage/db   live/stage/vpc
unit  live/stage/ec2  live/stage/vpc, live/stage/db
unit  live/stage/vpc
`,
		},
		{
			name:       "List with DAG dependencies in JSON format",
			workingDir: "fixtures/list/dag/live",
			args:       []string{"list", "--json", "--dependencies"},
			expected: `[
  {
    "path": "dev",
    "children": [
      {
        "path": "dev/db",
        "type": "unit",
        "dependencies": [
          {
            "path": "dev/vpc",
            "type": "unit"
          }
        ]
      },
      {
        "path": "dev/ec2",
        "type": "unit",
        "dependencies": [
          {
            "path": "dev/vpc",
            "type": "unit"
          },
          {
            "path": "dev/db",
            "type": "unit",
            "dependencies": [
              {
                "path": "dev/vpc",
                "type": "unit"
              }
            ]
          }
        ]
      },
      {
        "path": "dev/vpc",
        "type": "unit"
      }
    ]
  },
  {
    "path": "prod",
    "children": [
      {
        "path": "prod/db",
        "type": "unit",
        "dependencies": [
          {
            "path": "prod/vpc",
            "type": "unit"
          }
        ]
      },
      {
        "path": "prod/ec2",
        "type": "unit",
        "dependencies": [
          {
            "path": "prod/vpc",
            "type": "unit"
          },
          {
            "path": "prod/db",
            "type": "unit",
            "dependencies": [
              {
                "path": "prod/vpc",
                "type": "unit"
              }
            ]
          }
        ]
      },
      {
        "path": "prod/vpc",
        "type": "unit"
      }
    ]
  },
  {
    "path": "stage",
    "children": [
      {
        "path": "stage/db",
        "type": "unit",
        "dependencies": [
          {
            "path": "stage/vpc",
            "type": "unit"
          }
        ]
      },
      {
        "path": "stage/ec2",
        "type": "unit",
        "dependencies": [
          {
            "path": "stage/vpc",
            "type": "unit"
          },
          {
            "path": "stage/db",
            "type": "unit",
            "dependencies": [
              {
                "path": "stage/vpc",
                "type": "unit"
              }
            ]
          }
        ]
      },
      {
        "path": "stage/vpc",
        "type": "unit"
      }
    ]
  }
]
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
