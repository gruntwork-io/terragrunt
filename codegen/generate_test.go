package codegen

import (
	"bytes"
	"testing"

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
