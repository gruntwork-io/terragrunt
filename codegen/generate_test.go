package codegen

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

func TestRemoteStateConfigToTerraformCode(t *testing.T) {
	expectedOrdered := []byte(`terraform {
  backend "ordered" {
    a = 1
    b = 2
    c = 3
  }
}
`)
	expectedEmpty := []byte(`terraform {
  backend "empty" {
  }
}
`)

	testCases := []struct {
		name     string
		backend  string
		config   map[string]interface{}
		expected []byte
	}{
		{
			"remote-state-config-unsorted-keys",
			"ordered",
			map[string]interface{}{
				"b": 2,
				"a": 1,
				"c": 3,
			},
			expectedOrdered,
		},
		{
			"remote-state-config-empty",
			"empty",
			map[string]interface{}{},
			expectedEmpty,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			output, err := RemoteStateConfigToTerraformCode(testCase.backend, testCase.config)
			// validates the first output.
			require.True(t, bytes.Contains(output, []byte(testCase.backend)))
			require.Equal(t, testCase.expected, output)
			require.Nil(t, err)

			// runs the function a few of times again. All the outputs must be
			// equal to the first output.
			for i := 0; i < 20; i++ {
				actual, _ := RemoteStateConfigToTerraformCode(testCase.backend, testCase.config)
				require.Equal(t, output, actual)
			}
		})
	}
}

func TestGenerateDisabling(t *testing.T) {
	testDir := t.TempDir()

	testCases := []struct {
		name     string
		disabled bool
		path     string
		contents string
		ifExists GenerateConfigExists
	}{
		{
			"generate-disabled-true",
			true,
			fmt.Sprintf("%s/%s", testDir, "disabled_true"),
			"this file should not be generated",
			ExistsError,
		},
		{
			"generate-disabled-false",
			false,
			fmt.Sprintf("%s/%s", testDir, "disabled_false"),
			"this file should be generated",
			ExistsError,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			config := GenerateConfig{
				Path:             testCase.path,
				IfExists:         testCase.ifExists,
				CommentPrefix:    "",
				DisableSignature: false,
				Contents:         testCase.contents,
				Disable:          testCase.disabled,
			}

			opts, err := options.NewTerragruntOptionsForTest("mock-path-for-test.hcl")
			require.Nil(t, err)
			require.NotNil(t, opts)

			err = WriteToFile(opts, "", config)
			require.Nil(t, err)

			if testCase.disabled {
				require.True(t, util.FileNotExists(testCase.path))
			} else {
				require.True(t, util.FileExists(testCase.path))
			}
		})
	}
}
